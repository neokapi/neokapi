package jobs

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Job queue names. These are the base names; Terraform (prod) and the compose
// bootstrap (local, via LocalStack/ElasticMQ) provision queues with these names
// plus the optional BOWRAIN_SQS_QUEUE_PREFIX, each with a dead-letter queue and a
// redrive policy of maxReceiveCount=3 (sqsMaxReceiveCount).
const (
	SQSTranslationQueue = "translation-jobs"
	SQSExtractionQueue  = "extraction-jobs"
	SQSBrandScanQueue   = "brand-scan-jobs"
)

// sqsMaxReceiveCount mirrors the redrive policy's maxReceiveCount provisioned
// by Terraform (prod) and the compose bootstrap (local): after this many
// receives a message moves to the dead-letter queue. The DB attempt budget
// (defaultMaxJobAttempts) is aligned with it so both ceilings agree.
const sqsMaxReceiveCount = 3

// sqsVisibilityTimeout mirrors the provisioned queue visibility timeout: a
// received-but-unacked message reappears after this long if its worker dies
// mid-flight. The stale-job sweeper threshold sits comfortably above it so the
// sweeper never races the broker's own redelivery.
const sqsVisibilityTimeout = 5 * time.Minute

// sqsLongPollSeconds is the ReceiveMessage wait time. 20s is the SQS maximum and
// keeps empty-receive request volume (and cost) low.
const sqsLongPollSeconds = 20

// SQSQueue implements Queue using Amazon SQS (or an SQS-compatible service:
// LocalStack, ElasticMQ). One instance wraps one queue URL.
type SQSQueue struct {
	client   *sqs.Client
	queueURL string
}

// SQSOptions configures the SQS client shared across the job queues.
type SQSOptions struct {
	Region      string // AWS region, e.g. "eu-north-1"
	Endpoint    string // endpoint override for LocalStack/ElasticMQ; empty on AWS
	QueuePrefix string // optional name prefix applied to every queue

	// Static credentials for local dev / tests. Empty in production, where the
	// default chain resolves the ECS task role.
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// SQSConfigured reports whether the environment selects the SQS backend.
func SQSConfigured() bool {
	return os.Getenv("BOWRAIN_QUEUE_BACKEND") == "sqs"
}

// SQSOptionsFromEnv reads the shared SQS env contract (server and worker use the
// same names so they can't drift):
//
//	BOWRAIN_QUEUE_BACKEND=sqs   select the SQS backend
//	AWS_REGION                  AWS region (also read natively by the SDK)
//	SQS_ENDPOINT                endpoint override for LocalStack/ElasticMQ
//	BOWRAIN_SQS_QUEUE_PREFIX    optional queue-name prefix
//
// Credentials come from the standard AWS chain, never from here.
func SQSOptionsFromEnv() SQSOptions {
	return SQSOptions{
		Region:      os.Getenv("AWS_REGION"),
		Endpoint:    os.Getenv("SQS_ENDPOINT"),
		QueuePrefix: os.Getenv("BOWRAIN_SQS_QUEUE_PREFIX"),
	}
}

// NewSQSQueue resolves the URL of the prefixed queue name and returns a Queue
// bound to it. The queue must already exist (provisioned by Terraform or the
// compose bootstrap); a missing queue is a configuration error, surfaced here.
func NewSQSQueue(ctx context.Context, opts SQSOptions, queueName string) (*SQSQueue, error) {
	loadOpts := []func(*config.LoadOptions) error{}
	if opts.Region != "" {
		loadOpts = append(loadOpts, config.WithRegion(opts.Region))
	}
	if opts.AccessKeyID != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.SecretAccessKey, opts.SessionToken)))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("sqs: load aws config: %w", err)
	}

	client := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
		}
	})

	name := opts.QueuePrefix + queueName
	out, err := client.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{QueueName: aws.String(name)})
	if err != nil {
		return nil, fmt.Errorf("sqs: resolve queue %q: %w", name, err)
	}
	return &SQSQueue{client: client, queueURL: aws.ToString(out.QueueUrl)}, nil
}

// Enqueue sends the job ID as the message body.
func (q *SQSQueue) Enqueue(ctx context.Context, jobID string) error {
	_, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(q.queueURL),
		MessageBody: aws.String(jobID),
	})
	if err != nil {
		return fmt.Errorf("sqs: enqueue job %s: %w", jobID, err)
	}
	return nil
}

// Dequeue long-polls for one message. Empty polls are retried internally (SQS
// long-poll returns nothing when idle) so the caller only sees a message or a
// real error — no per-poll error churn. ack deletes the message; nack makes it
// immediately visible again for redelivery, which counts toward the queue's
// maxReceiveCount before it lands in the DLQ.
func (q *SQSQueue) Dequeue(ctx context.Context) (string, func(), func(), error) {
	for {
		select {
		case <-ctx.Done():
			return "", nil, nil, ctx.Err()
		default:
		}

		out, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(q.queueURL),
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     sqsLongPollSeconds,
		})
		if err != nil {
			if ctx.Err() != nil {
				return "", nil, nil, ctx.Err()
			}
			return "", nil, nil, fmt.Errorf("sqs: receive: %w", err)
		}
		if len(out.Messages) == 0 {
			continue // long-poll idle timeout; poll again
		}

		msg := out.Messages[0]
		jobID := aws.ToString(msg.Body)
		receipt := msg.ReceiptHandle

		// Settlement runs after the caller has processed the job, when the
		// receive ctx may already be done; detach it but keep the trace.
		settleCtx := context.WithoutCancel(ctx)
		ack := func() {
			_, _ = q.client.DeleteMessage(settleCtx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(q.queueURL),
				ReceiptHandle: receipt,
			})
		}
		nack := func() {
			_, _ = q.client.ChangeMessageVisibility(settleCtx, &sqs.ChangeMessageVisibilityInput{
				QueueUrl:          aws.String(q.queueURL),
				ReceiptHandle:     receipt,
				VisibilityTimeout: 0,
			})
		}
		return jobID, ack, nack, nil
	}
}

// Healthy is best-effort: the SQS client has no connection-level ping.
func (q *SQSQueue) Healthy() bool { return true }

// Close releases resources. The SQS client holds no long-lived connection.
func (q *SQSQueue) Close() error { return nil }

var _ Queue = (*SQSQueue)(nil)
