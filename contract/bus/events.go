package bus

// DomainEvent represents in-process domain events (sync fan-out). Handlers may be queued.
type DomainEvent interface{}

// IntegrationEvent represents events destined to external brokers (async). Topic() may guide routing.
type IntegrationEvent interface{ Topic() string }
