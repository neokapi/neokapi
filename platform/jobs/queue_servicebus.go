package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// ServiceBusQueue implements Queue using Azure Service Bus.
type ServiceBusQueue struct {
	client   *azservicebus.Client
	sender   *azservicebus.Sender
	receiver *azservicebus.Receiver
	queue    string
}

// NewServiceBusQueue creates a ServiceBusQueue connected to the given queue name.
// connStr is the Azure Service Bus connection string.
func NewServiceBusQueue(connStr, queueName string) (*ServiceBusQueue, error) {
	client, err := azservicebus.NewClientFromConnectionString(connStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create service bus client: %w", err)
	}

	sender, err := client.NewSender(queueName, nil)
	if err != nil {
		client.Close(context.Background())
		return nil, fmt.Errorf("create sender for %q: %w", queueName, err)
	}

	receiver, err := client.NewReceiverForQueue(queueName, nil)
	if err != nil {
		sender.Close(context.Background())
		client.Close(context.Background())
		return nil, fmt.Errorf("create receiver for %q: %w", queueName, err)
	}

	return &ServiceBusQueue{
		client:   client,
		sender:   sender,
		receiver: receiver,
		queue:    queueName,
	}, nil
}

func (q *ServiceBusQueue) Enqueue(ctx context.Context, jobID string) error {
	msg := &azservicebus.Message{
		Body: []byte(jobID),
	}
	if err := q.sender.SendMessage(ctx, msg, nil); err != nil {
		return fmt.Errorf("enqueue job %s: %w", jobID, err)
	}
	return nil
}

func (q *ServiceBusQueue) Dequeue(ctx context.Context) (string, func(), func(), error) {
	messages, err := q.receiver.ReceiveMessages(ctx, 1, &azservicebus.ReceiveMessagesOptions{
		TimeAfterFirstMessage: 5 * time.Second,
	})
	if err != nil {
		return "", nil, nil, fmt.Errorf("receive message: %w", err)
	}
	if len(messages) == 0 {
		return "", nil, nil, fmt.Errorf("no messages available")
	}

	msg := messages[0]
	jobID := string(msg.Body)

	ack := func() {
		_ = q.receiver.CompleteMessage(context.Background(), msg, nil)
	}
	nack := func() {
		_ = q.receiver.AbandonMessage(context.Background(), msg, nil)
	}

	return jobID, ack, nack, nil
}

func (q *ServiceBusQueue) Healthy() bool {
	return true // Service Bus SDK has no connection-level ping; best-effort.
}

func (q *ServiceBusQueue) Close() error {
	ctx := context.Background()
	var firstErr error
	if err := q.receiver.Close(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := q.sender.Close(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := q.client.Close(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
