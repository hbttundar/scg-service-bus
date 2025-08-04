package bus

import "context"

// JobEnqueuer abstracts command/listener enqueue operations.
// Library users provide an implementation backed by their queue/broker.
type JobEnqueuer interface {
	EnqueueCommand(ctx context.Context, cmd Command, opts QueueOptions) error
	EnqueueListener(ctx context.Context, evt DomainEvent, handler string, opts QueueOptions) error
}
