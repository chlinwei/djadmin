package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"autoadmin/internal/job"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	DefaultExchange    = "autoadmin.jobs"
	DefaultQueue       = "autoadmin.job.execute"
	DefaultRoutingKey  = "job.execute"
	DeadLetterExchange = "autoadmin.jobs.dead"
	DeadLetterQueue    = "autoadmin.job.dead"
)

type Client struct {
	connection *amqp.Connection
}

func Dial(url string) (*Client, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial RabbitMQ: %w", err)
	}
	return &Client{connection: connection}, nil
}

func (client *Client) Close() error {
	return client.connection.Close()
}

func (client *Client) DeclareTopology() error {
	channel, err := client.connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ topology channel: %w", err)
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(DeadLetterExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}
	if _, err := channel.QueueDeclare(DeadLetterQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	if err := channel.QueueBind(DeadLetterQueue, DefaultRoutingKey, DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue: %w", err)
	}
	if err := channel.ExchangeDeclare(DefaultExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare job exchange: %w", err)
	}
	arguments := amqp.Table{"x-dead-letter-exchange": DeadLetterExchange, "x-dead-letter-routing-key": DefaultRoutingKey}
	if _, err := channel.QueueDeclare(DefaultQueue, true, false, false, false, arguments); err != nil {
		return fmt.Errorf("declare job queue: %w", err)
	}
	if err := channel.QueueBind(DefaultQueue, DefaultRoutingKey, DefaultExchange, false, nil); err != nil {
		return fmt.Errorf("bind job queue: %w", err)
	}
	return nil
}

func (client *Client) Publish(ctx context.Context, message job.Message) error {
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode job message: %w", err)
	}
	channel, err := client.connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ publish channel: %w", err)
	}
	defer channel.Close()

	return channel.PublishWithContext(ctx, DefaultExchange, DefaultRoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    message.ExecutionID,
		Body:         body,
	})
}

type Handler interface {
	Handle(context.Context, job.Message) error
}

func (client *Client) Consume(ctx context.Context, consumer string, prefetch int, handler Handler) error {
	channel, err := client.connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ consumer channel: %w", err)
	}
	defer channel.Close()
	if err := channel.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("set RabbitMQ prefetch: %w", err)
	}
	deliveries, err := channel.ConsumeWithContext(ctx, DefaultQueue, consumer, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume job queue: %w", err)
	}
	for delivery := range deliveries {
		var message job.Message
		if err := json.Unmarshal(delivery.Body, &message); err != nil || message.SchemaVersion != job.SchemaVersion {
			_ = delivery.Nack(false, false)
			continue
		}
		if err := handler.Handle(ctx, message); err != nil {
			_ = delivery.Nack(false, false)
			continue
		}
		_ = delivery.Ack(false)
	}
	return ctx.Err()
}
