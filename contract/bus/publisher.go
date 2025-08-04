package bus

import "context"

// EventPublisher abstracts publishing integration events to a broker/bus.
// Library users provide an implementation that maps to Kafka/NATS/RabbitMQ etc.
type EventPublisher interface {
	PublishIntegration(ctx context.Context, evt IntegrationEvent, opts PublishOptions) error
}
