package bus

import "time"

// revive:disable:max-public-structs
// Queueable indicates that a command prefers to be enqueued for async processing.
// Implement on command types that should be queued by default.
type Queueable interface {
	QueueName() string
	Delay() time.Duration
}

// QueueableListener indicates that a domain event listener may be enqueued.
// If a JobEnqueuer is configured, such listeners will be enqueued instead of invoked synchronously.
type QueueableListener interface {
	QueueName() string
	Delay() time.Duration
}

// ShouldQueue mirrors Laravel's ShouldQueue marker for jobs/commands.
// It is an alias for Queueable to keep APIs familiar for Laravel users.
// Implementers may choose either name; the Bus treats them identically.
type ShouldQueue = Queueable

// ShouldQueueListener mirrors Laravel's listener queuing semantics.
// Alias of QueueableListener for familiarity.
type ShouldQueueListener = QueueableListener

// Retryable allows a command/listener to specify a maximum number of attempts (tries).
// Aligns with Laravel's tries property.
type Retryable interface {
	Tries() int
}

// RetryUntil allows specifying a cutoff time after which the job should no longer be retried.
// Mirrors Laravel's retryUntil().
type RetryUntil interface {
	RetryUntil() time.Time
}

// Backoffable exposes a backoff schedule between retries.
// Mirrors Laravel's backoff property/method.
type Backoffable interface {
	Backoff() []time.Duration
}

// Timeoutable allows specifying a timeout for job execution.
// Mirrors Laravel's $timeout.
type Timeoutable interface {
	Timeout() time.Duration
}

// AfterCommit indicates that enqueue/publish should occur only after the surrounding transaction commits.
// Mirrors Laravel's ShouldQueue with ShouldBeUniqueUntilProcessing/AfterCommit behavior.
// This is advisory for adapters/outbox.
type AfterCommit interface {
	AfterCommit() bool
}

// Unique indicates that a job/event should be unique for a period to enforce idempotency.
// Mirrors Laravel's ShouldBeUnique/ShouldBeUniqueUntilProcessing.
type Unique interface {
	UniqueKey() string
	UniqueTTL() time.Duration // zero means adapter default
}

// OnConnection allows selecting a named connection (transport instance) for queueing.
// Mirrors Laravel's onConnection().
type OnConnection interface {
	Connection() string
}

// revive:enable:max-public-structs
