package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newElasticMQ starts a throwaway ElasticMQ (SQS-compatible) container and
// returns its endpoint. ElasticMQ is the standard fully-open SQS emulator (no
// license), speaking the same SQS API surface — SendMessage, long-poll
// ReceiveMessage, DeleteMessage, ChangeMessageVisibility — the code runs against
// Amazon SQS in production, per the no-mocks rule.
func newElasticMQ(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping ElasticMQ container test in -short mode")
	}
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Image:        "softwaremill/elasticmq-native:latest",
		ExposedPorts: []string{"9324/tcp"},
		WaitingFor: wait.ForListeningPort("9324/tcp").
			WithStartupTimeout(60 * time.Second),
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "9324/tcp")
	require.NoError(t, err)
	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func rawSQSClient(t *testing.T, endpoint string) *sqs.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	require.NoError(t, err)
	return sqs.NewFromConfig(cfg, func(o *sqs.Options) { o.BaseEndpoint = aws.String(endpoint) })
}

func testSQSOptions(endpoint string) SQSOptions {
	return SQSOptions{
		Region:          "us-east-1",
		Endpoint:        endpoint,
		AccessKeyID:     "test",
		SecretAccessKey: "test",
	}
}

// The core contract the SQSQueue owns: a job enqueues, dequeues with its body
// intact, nack returns it for redelivery, and ack removes it for good.
func TestSQSRoundTripAckNack(t *testing.T) {
	endpoint := newElasticMQ(t)
	ctx := t.Context()

	raw := rawSQSClient(t, endpoint)
	_, err := raw.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String("jobs")})
	require.NoError(t, err)

	q, err := NewSQSQueue(ctx, testSQSOptions(endpoint), "jobs")
	require.NoError(t, err)

	require.NoError(t, q.Enqueue(ctx, "job-1"))

	// First delivery, then nack — the job must come back.
	id, _, nack, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-1", id)
	nack()

	// Second delivery — same job, now ack it.
	id2, ack2, _, err := q.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, "job-1", id2)
	ack2()

	// After ack the queue is empty (short-poll, so the test doesn't block on the
	// 20s long-poll the production Dequeue uses).
	out, err := raw.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(q.queueURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     1,
	})
	require.NoError(t, err)
	assert.Empty(t, out.Messages, "an acked message must not be redelivered")
}

// A missing queue is a configuration error surfaced at construction, not a
// silent nil that fails later on the first enqueue.
func TestNewSQSQueueMissingQueue(t *testing.T) {
	endpoint := newElasticMQ(t)
	_, err := NewSQSQueue(t.Context(), testSQSOptions(endpoint), "does-not-exist")
	assert.Error(t, err)
}

// The env contract selects the backend and carries the endpoint override — no
// container needed.
func TestSQSConfiguredFromEnv(t *testing.T) {
	t.Setenv("BOWRAIN_QUEUE_BACKEND", "sqs")
	t.Setenv("AWS_REGION", "eu-north-1")
	t.Setenv("SQS_ENDPOINT", "http://localstack:4566")
	t.Setenv("BOWRAIN_SQS_QUEUE_PREFIX", "bowrain-")

	assert.True(t, SQSConfigured())
	opts := SQSOptionsFromEnv()
	assert.Equal(t, "eu-north-1", opts.Region)
	assert.Equal(t, "http://localstack:4566", opts.Endpoint)
	assert.Equal(t, "bowrain-", opts.QueuePrefix)
}

func TestSQSNotConfiguredByDefault(t *testing.T) {
	t.Setenv("BOWRAIN_QUEUE_BACKEND", "")
	assert.False(t, SQSConfigured())
}
