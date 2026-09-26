// Package s3remote keeps a project's shared context in an S3-compatible
// bucket: Amazon S3, or any service that speaks its API and honours a
// conditional write.
//
// Each object of the context layout (core/workspace) is one key under the
// recipe's prefix. A write is a PutObject with If-None-Match: *, so a key is
// created once and never replaced, which is all the layout asks of a store.
// Credentials come from the standard AWS environment, shared configuration and
// profile; the recipe names the bucket, the prefix and, for a service other
// than AWS, the endpoint.
//
// Importing the package registers the "s3" backend with the host.
package s3remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
)

func init() {
	host.RegisterContextRemote(project.ContextBackendS3, func(ctx context.Context, spec project.ContextBackend) (workspace.Remote, error) {
		return New(ctx, Options{Bucket: spec.Bucket, Prefix: spec.Prefix, Region: spec.Region, Endpoint: spec.Endpoint})
	})
}

// Options place the remote.
type Options struct {
	Bucket string
	// Prefix is prepended to every key; a trailing slash is added when
	// missing.
	Prefix string
	// Region defaults to the AWS configuration's, and to us-east-1 for an
	// endpoint that names none.
	Region string
	// Endpoint is the base URL of an S3-compatible service other than AWS.
	// Requests to it use path-style addressing.
	Endpoint string
	// AccessKeyID and SecretAccessKey override the AWS credential chain. Tests
	// set them; a recipe never does.
	AccessKeyID, SecretAccessKey string
}

// Remote is a context layout in a bucket.
type Remote struct {
	client *s3.Client
	bucket string
	prefix string
}

// New builds a remote from the AWS configuration and the options given.
func New(ctx context.Context, o Options) (*Remote, error) {
	if o.Bucket == "" {
		return nil, errors.New("s3: no bucket named")
	}
	var opts []func(*config.LoadOptions) error
	if o.Region != "" {
		opts = append(opts, config.WithRegion(o.Region))
	}
	if o.AccessKeyID != "" {
		opts = append(opts, config.WithCredentialsProvider(aws.CredentialsProviderFunc(
			func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: o.AccessKeyID, SecretAccessKey: o.SecretAccessKey}, nil
			})))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: s3: load the AWS configuration: %w", workspace.ErrRemoteUnreachable, err)
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	client := s3.NewFromConfig(cfg, func(so *s3.Options) {
		if o.Endpoint != "" {
			so.BaseEndpoint = aws.String(o.Endpoint)
			so.UsePathStyle = true
		}
	})
	prefix := o.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &Remote{client: client, bucket: o.Bucket, prefix: prefix}, nil
}

// Describe reports the bucket and prefix.
func (r *Remote) Describe() workspace.RemoteDescriptor {
	return workspace.RemoteDescriptor{Kind: "s3", Location: "s3://" + r.bucket + "/" + r.prefix}
}

// List reads the keys under one directory of the layout.
func (r *Remote) List(ctx context.Context, dir string) ([]string, error) {
	switch dir {
	case workspace.RemoteLogDir, workspace.RemoteBlobDir, workspace.RemoteCheckpointsDir:
	default:
		return nil, fmt.Errorf("s3: %q is not a directory of the context layout", dir)
	}
	var out []string
	pages := s3.NewListObjectsV2Paginator(r.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(r.bucket), Prefix: aws.String(r.prefix + dir),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, unreachable("list", dir, err)
		}
		for _, obj := range page.Contents {
			name := strings.TrimPrefix(aws.ToString(obj.Key), r.prefix)
			if workspace.ValidObjectName(name) {
				out = append(out, name)
			}
		}
	}
	return out, nil
}

// Get reads one object, refusing one larger than a blob may be.
func (r *Remote) Get(ctx context.Context, name string) ([]byte, error) {
	if !workspace.ValidObjectName(name) {
		return nil, fmt.Errorf("s3: %q is not a name in the context layout", name)
	}
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(r.bucket), Key: aws.String(r.prefix + name)})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) || statusOf(err) == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", workspace.ErrNoObject, name)
		}
		return nil, unreachable("read", name, err)
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(out.Body, workspace.MaxBlobSize+1))
	if err != nil {
		return nil, unreachable("read", name, err)
	}
	if len(data) > workspace.MaxBlobSize {
		return nil, fmt.Errorf("s3: %s is larger than any object of the context layout", name)
	}
	return data, nil
}

// Put creates each object with a conditional write.
func (r *Remote) Put(ctx context.Context, objs ...workspace.Object) error {
	for _, obj := range objs {
		if !workspace.ValidObjectName(obj.Name) {
			return fmt.Errorf("s3: %q is not a name in the context layout", obj.Name)
		}
	}
	for _, obj := range objs {
		_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(r.bucket),
			Key:           aws.String(r.prefix + obj.Name),
			Body:          bytes.NewReader(obj.Data),
			ContentLength: aws.Int64(int64(len(obj.Data))),
			IfNoneMatch:   aws.String("*"),
		})
		if err == nil {
			continue
		}
		switch statusOf(err) {
		case http.StatusPreconditionFailed, http.StatusConflict:
			held, gerr := r.Get(ctx, obj.Name)
			if gerr != nil {
				return gerr
			}
			if !bytes.Equal(held, obj.Data) {
				return fmt.Errorf("%w: %s", workspace.ErrObjectExists, obj.Name)
			}
		default:
			return unreachable("write", obj.Name, err)
		}
	}
	return nil
}

// Close holds nothing open.
func (r *Remote) Close() error { return nil }

// statusOf reads the HTTP status a failed request answered with, zero when
// it reached no server.
func statusOf(err error) int {
	var resp *smithyhttp.ResponseError
	if errors.As(err, &resp) {
		return resp.HTTPStatusCode()
	}
	return 0
}

// unreachable reports a failed request as the backend being out of reach,
// naming the API error code when the server gave one.
func unreachable(verb, name string, err error) error {
	var api smithy.APIError
	if errors.As(err, &api) {
		return fmt.Errorf("%w: s3 %s %s: %s: %s", workspace.ErrRemoteUnreachable, verb, name, api.ErrorCode(), api.ErrorMessage())
	}
	return fmt.Errorf("%w: s3 %s %s: %w", workspace.ErrRemoteUnreachable, verb, name, err)
}
