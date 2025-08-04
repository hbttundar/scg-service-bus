package rabbitmq

import (
	"context"
	"fmt"
	"time"

	berr "github.com/hbttundar/scg-service-bus/contract/errors"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Concrete AMQP connection-backed constructor and publisher wrapper.

const (
	integrationExchange   = "integration"
	integrationExchangeTy = "topic"
)

type Config struct {
	URL         string
	ConnTimeout time.Duration
}

type amqpConnPublisher struct{ ch *amqp.Channel }

func (p amqpConnPublisher) Publish(ctx context.Context, m PubMsg) error {
	var h amqp.Table
	if len(m.Headers) > 0 {
		h = amqp.Table{}
		for k, v := range m.Headers {
			h[k] = v
		}
	}

	return p.ch.PublishWithContext(
		ctx,
		m.Exchange,
		m.RoutingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			Headers:      h,
			ContentType:  "application/json",
			Body:         m.Body,
		},
	)
}

// NewWithAMQPConn dials RabbitMQ, ensures integration exchange, and returns Adapter and cleanup.
func NewWithAMQPConn(cfg Config) (*Adapter, func(), error) {
	if cfg.URL == "" {
		return nil, nil, fmt.Errorf("%w: rabbitmq url required", berr.ErrPublishFailed)
	}

	conn, err := amqp.DialConfig(cfg.URL, amqp.Config{
		Locale:     "en_US",
		Properties: amqp.Table{"product": "scg-service-bus"},
		Dial:       amqp.DefaultDial(cfg.ConnTimeout),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: rabbitmq dial: %w", berr.ErrPublishFailed, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close() //nolint:errcheck // best-effort close on error path; cannot return this error
		return nil, nil, fmt.Errorf("%w: rabbitmq open channel: %w", berr.ErrPublishFailed, err)
	}

	if err := ch.ExchangeDeclare(
		integrationExchange,
		integrationExchangeTy,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = ch.Close()   //nolint:errcheck // best-effort shutdown on error path; cannot return this error
		_ = conn.Close() //nolint:errcheck // best-effort shutdown on error path; cannot return this error

		return nil, nil, fmt.Errorf("%w: declare exchange %q: %w", berr.ErrPublishFailed, integrationExchange, err)
	}

	ad := New(amqpConnPublisher{ch: ch})
	cleanup := func() {
		_ = ch.Close()   //nolint:errcheck // best-effort shutdown in cleanup; cannot return error here
		_ = conn.Close() //nolint:errcheck // best-effort shutdown in cleanup; cannot return error here
	}

	return ad, cleanup, nil
}
