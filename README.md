# SCG Service Bus

A thin, idiomatic Go mediator aligned with Laravel 12 Bus semantics. In-process dispatch/ask and domain events, plus minimal adapters to enqueue jobs and publish integration events. No globals.

- Status: Experimental (APIs may evolve before 1.0)

## Install & Requirements
- Minimum Go: 1.24.5 (from go.mod)
- Module:

```bash
go get github.com/hbttundar/scg-service-bus
```

- Build tags:
  - Kafka franz-go helper requires build tag `franz` when you use adapters/kafka.NewWithKgo.

## Concepts
- Messages
  - Command: intent to change state (contract/bus.Command)
  - Query: read-only request, returns a response (contract/bus.Query)
  - DomainEvent: in-process fan-out; handlers may be queued (contract/bus.DomainEvent)
  - IntegrationEvent: async, destined to external brokers; must implement Topic() string
- Handlers (generics in contract/bus)
  - CommandHandler[C], QueryHandler[Q,R], DomainEventHandler[E]
- Queue semantics
  - Queueable (alias: ShouldQueue) on commands → Dispatch enqueues when a JobEnqueuer is configured
  - QueueableListener (alias: ShouldQueueListener) on domain event handlers → handler is enqueued if a JobEnqueuer exists
- Errors
  - Canonical errors live in contract/errors/errors.go and are wrapped with %w (fmt.Errorf) across the codebase

## Quickstart (sync, in-process)

```go
package main

import (
	"context"
	"fmt"

	"github.com/hbttundar/scg-service-bus/servicebus"
)

type CreateUser struct{ ID, Name string }

type GetUser struct{ ID string }

type User struct{ ID, Name string }

func main() {
	bus := servicebus.New(nil, nil, nil)

	// Bind using the provided "Of" helpers (untyped) or the generic helpers below.
	_ = bus.BindCommandOf(CreateUser{}, func(ctx context.Context, v any) error {
		c := v.(CreateUser)
		fmt.Println("created:", c.ID, c.Name)
		return nil
	})

	_ = bus.BindQueryOf(GetUser{}, func(ctx context.Context, v any) (any, error) {
		q := v.(GetUser)
		return User{ID: q.ID, Name: "Jane"}, nil
	})

	// Sync dispatch and ask
	_ = bus.DispatchSync(context.Background(), CreateUser{ID: "u-1", Name: "Jane"})

	res, _ := bus.Ask(context.Background(), GetUser{ID: "u-1"})
	u := res.(User)
	fmt.Println("user:", u.ID, u.Name)
}
```

Typed helpers (generics):

```go
// After binding with generics
// _ = servicebus.BindCommand[CreateUser](bus, CreateUserHandler{})
// _ = servicebus.BindQuery[GetUser, User](bus, GetUserHandler{})

u, err := servicebus.Ask[GetUser, User](ctx, bus, GetUser{ID: "u-1"})
_ = u
_ = err
```

Domain events:

```go
// Bind domain event handler
_ = bus.BindDomainEventOf(UserCreated{}, func(ctx context.Context, e any) error { return nil })

// Or type-safe
// _ = servicebus.BindDomainEvent[UserCreated](bus, SendWelcomeEmail{})

_ = bus.PublishDomain(ctx, UserCreated{ID: "u-1"})
```

## Async and queueing semantics
- Commands
  - bus.Dispatch(ctx, cmd) enqueues when cmd implements Queueable and a JobEnqueuer is configured. Otherwise, it executes synchronously (same as DispatchSync).
- Domain events
  - PublishDomain(ctx, e) fans out synchronously to all handlers by default.
  - If a handler implements QueueableListener and a JobEnqueuer is configured, that handler is enqueued instead of being called inline.
- Integration events
  - PublishIntegration(ctx, e, opts) always goes through the configured EventPublisher. If none is set, returns contract/errors.ErrAsyncNotConfigured.

## Adapters
All adapters satisfy contract/bus.Adapter (both JobEnqueuer and EventPublisher) or subsets thereof. Wire them into servicebus.New(enqueuer, publisher, logger).

- InMemory (adapters/inmemory)
  - Constructor: inmemory.New() -> *inmemory.Adapter
  - Use for tests/examples; stores enqueued commands/listeners and published events in memory.
  - Wiring:

```go
ad := inmemory.New()
bus := servicebus.New(ad, ad, nil)
```

- Kafka (adapters/kafka)
  - Constructors:
    - kafka.New(w Writer) where Writer has: Write(topic string, key, value []byte, headers map[string]string) error
    - kafka.NewWithKgo(cfg Config) (requires build tag franz)
  - Topic mapping:
    - EnqueueCommand → topic "jobs.<CommandType>" or "jobs.<QueueOptions.Queue>"
    - EnqueueListener → topic "listeners.<EventType>.<handlerName>" or "jobs.<QueueOptions.Queue>"
    - PublishIntegration → topic from e.Topic() or opts.TopicOverride; Key from opts.Key
  - Delay: sets header "x-delay" (seconds) when QueueOptions.DelaySeconds > 0. No native delay; infra/consumers must respect it.
  - Context: Writer has no context; adapter checks ctx before calling Writer and then performs a synchronous write (caller cancellation after that point is not observed).
  - Wiring examples:

```go
// Minimal custom writer example
package main

import (
	"context"
	"github.com/hbttundar/scg-service-bus/adapters/kafka"
	"github.com/hbttundar/scg-service-bus/servicebus"
)

type MyWriter struct{}

func (MyWriter) Write(topic string, key, value []byte, headers map[string]string) error {
	// send to Kafka using your client
	return nil
}

func main() {
	ad := kafka.New(MyWriter{})
	bus := servicebus.New(ad, ad, nil)
	_ = bus // use bus
}
```

```bash
# franz-go helper
go build -tags=franz ./...
```

- NATS (adapters/nats)
  - Constructors:
    - nats.New(c Client) where Client has: Publish(subject string, data []byte, headers map[string]string) error
    - nats.NewWithNATS(cfg Config)
  - Subject mapping:
    - EnqueueCommand → subject "cmd.<CommandType>" or "cmd.<QueueOptions.Queue>"
    - EnqueueListener → subject "listeners.<EventType>.<handlerName>" or "cmd.<QueueOptions.Queue>"
    - PublishIntegration → subject from e.Topic() or opts.TopicOverride; optional header "key" from opts.Key
  - Delay: sets header "x-delay" (seconds) when QueueOptions.DelaySeconds > 0 (advisory only).
  - Wiring example:

```go
ad, cleanup, err := nats.NewWithNATS(nats.Config{URL: "nats://localhost:4222"})
if err != nil { panic(err) }
defer cleanup()
bus := servicebus.New(ad, ad, nil)
```

- RabbitMQ (adapters/rabbitmq)
  - Constructors:
    - rabbitmq.New(p Publisher) where Publisher has: Publish(ctx context.Context, m PubMsg) error
    - rabbitmq.NewWithAMQPChannel(ch *amqp.Channel)
    - rabbitmq.NewWithAMQPConn(cfg Config) declares a durable topic exchange named "integration" for integration events (for convenience), but the default Adapter publishes to the default exchange "" by default.
  - RoutingKey mapping (default exchange "" used by the Adapter):
    - EnqueueCommand → "cmd.<CommandType>" or "cmd.<QueueOptions.Queue>"
    - EnqueueListener → "listener.<EventType>.<handlerName>" or "cmd.<QueueOptions.Queue>"
    - PublishIntegration → routing key from e.Topic() or opts.TopicOverride; when using NewWithAMQPConn, messages are published with DeliveryMode Persistent.
  - Delay: sets header "x-delay" (seconds). Requires RabbitMQ delayed-message plugin or equivalent infra to take effect.
  - Wiring example:

```go
ad, cleanup, err := rabbitmq.NewWithAMQPConn(rabbitmq.Config{URL: "amqp://guest:guest@localhost:5672/"})
if err != nil { panic(err) }
defer cleanup()
bus := servicebus.New(ad, ad, nil)
```

## Delays
QueueOptions.DelaySeconds is propagated as "x-delay" header by all built-in adapters (Kafka/NATS/RabbitMQ). None of the adapters enforce native delays or return ErrDelayUnsupported; honoring delay is infrastructure-dependent (broker plugins, consumers, or middleware).

## Errors
- Canonical errors are defined in contract/errors/errors.go (e.g., ErrHandlerExists, ErrHandlerNotFound, ErrHandlerTypeMismatch, ErrAsyncNotConfigured, ErrEnqueueFailed, ErrPublishFailed, ErrDelayUnsupported, ErrSerializationFailed).
- Library operations wrap these with %w adding context (use errors.Is/As to check).

## Example
A runnable example is included in examples/main.go. Run:

```bash
go run ./examples
```

The example uses the in-memory adapter by default. To switch to a real adapter, replace the constructor accordingly (see Adapters section). For franz-go Kafka helper, build with:

```bash
go run -tags=franz ./examples
```

## Compatibility & Versioning
- No backward compatibility with prior CQRS/registry/factory patterns.
- APIs may change prior to 1.0.

## License & Contributing
- License: MIT
- Contributions: PRs and issues welcome; keep changes minimal and idiomatic.
