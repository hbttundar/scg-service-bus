package bus

// Adapter is a convenience interface that combines queueing and publishing capabilities.
// Any adapter that implements both JobEnqueuer and EventPublisher can be passed to the Bus.
//
// This keeps the Bus decoupled from concrete transports while enabling simple injection
// of user-provided adapters (Kafka, NATS, RabbitMQ, in-memory, etc.).
type Adapter interface {
	JobEnqueuer
	EventPublisher
}
