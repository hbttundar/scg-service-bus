package bus

import "context"

// CommandHandler handles commands of type C.
// Implementations must be safe for concurrent use by multiple goroutines.
type CommandHandler[C Command] interface {
	Handle(ctx context.Context, c C) error
}

// QueryHandler handles queries of type Q and returns a result of type R.
// Implementations must be safe for concurrent use by multiple goroutines.
type QueryHandler[Q Query, R any] interface {
	Handle(ctx context.Context, q Q) (R, error)
}

// DomainEventHandler handles domain events of type E.
// Implementations may be invoked synchronously or enqueued based on QueueableListener.
type DomainEventHandler[E DomainEvent] interface {
	Handle(ctx context.Context, e E) error
}
