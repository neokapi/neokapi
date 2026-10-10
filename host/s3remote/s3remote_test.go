package s3remote

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
)

// defaultGatewayImage is the S3-compatible gateway the tests run against: a
// real server with real conditional-write semantics, never a mock of them.
// It is the Docker Hub image versity/versitygw mirrored into this
// organisation's registry by scripts/mirror-image.sh (the "Mirror images"
// workflow), so CI pulls it with the workflow token rather than against
// Docker Hub's unauthenticated pull limit. Bump the tag here after mirroring
// the new one. KAPI_S3_TEST_IMAGE overrides it for a local run.
const defaultGatewayImage = "ghcr.io/neokapi/versitygw:v1.8.0"

const (
	testAccess = "kapitest"
	testSecret = "kapitestsecret"
)

func gatewayImage() string {
	if img := os.Getenv("KAPI_S3_TEST_IMAGE"); img != "" {
		return img
	}
	return defaultGatewayImage
}

// pullGatewayImage fetches the image unless it is already present. A registry
// that refuses the pull for lack of credentials is an environment the test
// cannot run in, like a machine without Docker, so it skips and says how to
// log in; any other pull failure is reported.
func pullGatewayImage(t *testing.T, image string) {
	t.Helper()
	if exec.Command("docker", "image", "inspect", image).Run() == nil {
		return
	}
	pull := exec.Command("docker", "pull", "--quiet", image)
	var stderr strings.Builder
	pull.Stderr = &stderr
	if err := pull.Run(); err != nil {
		msg := stderr.String()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "denied") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "authentication required") {
			t.Skipf("cannot pull %s (%s); log in with `gh auth token | docker login ghcr.io -u <user> --password-stdin` or set KAPI_S3_TEST_IMAGE", image, strings.TrimSpace(msg))
		}
		require.NoError(t, err, "docker pull %s: %s", image, msg)
	}
}

// startGateway runs the gateway in Docker for the test and returns its
// endpoint. The test is skipped where Docker is not available.
func startGateway(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping the S3 gateway container in -short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker is not running")
	}
	image := gatewayImage()
	pullGatewayImage(t, image)
	// The container id is stdout alone: on a runner without the image cached,
	// stderr carries the pull progress, which is not part of the id.
	run := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1::7070", image,
		"--access", testAccess, "--secret", testSecret, "posix", "/tmp")
	var stderr strings.Builder
	run.Stderr = &stderr
	out, err := run.Output()
	require.NoError(t, err, "docker run: %s", stderr.String())
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })

	port, err := exec.Command("docker", "port", id, "7070/tcp").Output()
	require.NoError(t, err)
	addr := strings.TrimSpace(strings.Split(string(port), "\n")[0])
	deadline := time.Now().Add(60 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			break
		}
		require.True(t, time.Now().Before(deadline), "the gateway did not start: %v", err)
		time.Sleep(200 * time.Millisecond)
	}
	return "http://" + addr
}

func TestS3RemoteConformance(t *testing.T) {
	endpoint := startGateway(t)
	n := 0
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		n++
		bucket := fmt.Sprintf("kapi-context-%d", n)
		opts := Options{
			Bucket: bucket, Prefix: "project", Region: "us-east-1", Endpoint: endpoint,
			AccessKeyID: testAccess, SecretAccessKey: testSecret,
		}
		first, err := New(context.Background(), opts)
		require.NoError(t, err)
		var created error
		for range 50 {
			_, created = first.client.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(bucket)})
			if created == nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		require.NoError(t, created)
		if n == 1 {
			requireConditionalWrites(t, first.client, bucket)
		}
		return func(t *testing.T) workspace.Remote {
			r, err := New(context.Background(), opts)
			require.NoError(t, err)
			return r
		}
	})
}
