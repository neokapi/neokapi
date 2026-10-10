package s3remote

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

// requireConditionalWrites holds the gateway to the two preconditions the
// remote relies on before any conformance case runs: a create-only put
// (`If-None-Match: *`) against an existing key and a put guarded by a stale
// `If-Match` are both refused with 412. A server that stops enforcing either
// would otherwise pass the suite by overwriting, which is the one outcome
// these tests exist to rule out.
func requireConditionalWrites(t *testing.T, client *s3.Client, bucket string) {
	t.Helper()
	ctx := context.Background()
	key := aws.String("probe/conditional-writes")
	put := func(data string, in *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		in.Bucket = aws.String(bucket)
		in.Key = key
		in.Body = strings.NewReader(data)
		in.ContentLength = aws.Int64(int64(len(data)))
		return client.PutObject(ctx, in)
	}

	first, err := put("one", &s3.PutObjectInput{IfNoneMatch: aws.String("*")})
	require.NoError(t, err, "the first create-only put succeeds")

	_, err = put("two", &s3.PutObjectInput{IfNoneMatch: aws.String("*")})
	require.Error(t, err, "a create-only put against an existing key must be refused")
	require.Equal(t, http.StatusPreconditionFailed, statusOf(err), "If-None-Match: * on an existing key answers 412, got %v", err)

	_, err = put("two", &s3.PutObjectInput{IfMatch: aws.String(`"0000000000000000000000000000dead"`)})
	require.Error(t, err, "a put guarded by a stale If-Match must be refused")
	require.Equal(t, http.StatusPreconditionFailed, statusOf(err), "a stale If-Match answers 412, got %v", err)

	_, err = put("two", &s3.PutObjectInput{IfMatch: first.ETag})
	require.NoError(t, err, "a put guarded by the current ETag succeeds")

	got, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: key})
	require.NoError(t, err)
	defer func() { _ = got.Body.Close() }()
	data, err := io.ReadAll(got.Body)
	require.NoError(t, err)
	require.Equal(t, "two", string(data), "the guarded put landed and the refused ones did not")
}
