package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/hbttundar/scg-service-bus/adapters/inmemory"
	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	"github.com/hbttundar/scg-service-bus/servicebus"
)

// --- Commands & Handlers ---

type CreateUser struct{ ID, Name string }

// Implement Queueable to demonstrate async enqueue on Dispatch.
func (CreateUser) QueueName() string    { return "users" }
func (CreateUser) Delay() time.Duration { return 0 }

// Compile-time assertion that CreateUser is a Command.
var _ cbus.Command = (*CreateUser)(nil)

type CreateUserHandler struct{}

func (CreateUserHandler) Handle(ctx context.Context, c CreateUser) error {
	// pretend we persist the user
	slog.InfoContext(ctx, "CreateUser", "id", c.ID, "name", c.Name)
	return nil
}

var _ cbus.CommandHandler[CreateUser] = (*CreateUserHandler)(nil)

// Non-queueable command to show sync behavior.
type RenameUser struct{ ID, NewName string }

var _ cbus.Command = (*RenameUser)(nil)

type RenameUserHandler struct{}

func (RenameUserHandler) Handle(ctx context.Context, c RenameUser) error {
	slog.InfoContext(ctx, "RenameUser", "id", c.ID, "new", c.NewName)
	return nil
}

// --- Queries & Handlers ---

type GetUser struct{ ID string }

var _ cbus.Query = (*GetUser)(nil)

type User struct{ ID, Name string }

type GetUserHandler struct{}

func (GetUserHandler) Handle(ctx context.Context, q GetUser) (User, error) {
	// pretend we query storage
	return User{ID: q.ID, Name: "Jane"}, nil
}

var _ cbus.QueryHandler[GetUser, User] = (*GetUserHandler)(nil)

// --- Domain Events & Handlers ---

type UserCreated struct{ ID string }

var _ cbus.DomainEvent = (*UserCreated)(nil)

type SendWelcomeEmail struct{}

// Implement QueueableListener so handler will be enqueued when a JobEnqueuer is configured.
func (SendWelcomeEmail) QueueName() string    { return "emails" }
func (SendWelcomeEmail) Delay() time.Duration { return time.Second }

func (SendWelcomeEmail) Handle(ctx context.Context, e UserCreated) error {
	slog.InfoContext(ctx, "SendWelcomeEmail", "user", e.ID)
	return nil
}

var _ cbus.DomainEventHandler[UserCreated] = (*SendWelcomeEmail)(nil)
var _ cbus.QueueableListener = (*SendWelcomeEmail)(nil)

// --- Integration Events ---

type UserExported struct{ ID string }

func (UserExported) Topic() string { return "users.exported" }

var _ cbus.IntegrationEvent = (*UserExported)(nil)

func main() {
	ctx := context.Background()

	// Build a logger (optional, can be nil)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Use the in-memory adapter for both queueing and publishing.
	ad := inmemory.New()
	bus := servicebus.New(ad, ad, logger)

	// Add a simple logging middleware.
	servicebus.WithCommandMiddleware(func(next func(ctx context.Context, cmd any) error) func(ctx context.Context, cmd any) error {
		return func(ctx context.Context, cmd any) error {
			slog.DebugContext(ctx, "before", "cmd", fmt.Sprintf("%T", cmd))
			err := next(ctx, cmd)
			slog.DebugContext(ctx, "after", "cmd", fmt.Sprintf("%T", cmd), "err", err)
			return err
		}
	})(bus)

	// Bind handlers using generics (type-safe) helpers.
	_ = servicebus.BindCommand[CreateUser](bus, CreateUserHandler{})
	_ = servicebus.BindCommand[RenameUser](bus, RenameUserHandler{})
	_ = servicebus.BindQuery[GetUser, User](bus, GetUserHandler{})
	_ = servicebus.BindDomainEvent[UserCreated](bus, SendWelcomeEmail{})

	// Demonstrate Dispatch: CreateUser implements Queueable, so it will be enqueued via the adapter.
	_ = bus.Dispatch(ctx, CreateUser{ID: "u-1", Name: "Jane"})

	// Non-queueable command executes synchronously.
	_ = bus.Dispatch(ctx, RenameUser{ID: "u-1", NewName: "Janet"})

	// Ask query (typed helper and untyped facade).
	u, err := servicebus.Ask[GetUser, User](ctx, bus, GetUser{ID: "u-1"})
	if err != nil {
		panic(err)
	}
	slog.Info("Ask result", "user", u)

	// Publish a domain event. Listener implements QueueableListener, so it will be enqueued.
	_ = bus.PublishDomain(ctx, UserCreated{ID: "u-1"})

	// Publish an integration event. Goes through adapter publisher.
	pubErr := bus.PublishIntegration(ctx, UserExported{ID: "u-1"}, cbus.PublishOptions{Key: "u-1"})
	if pubErr != nil {
		panic(pubErr)
	}

	// Use facades.
	cmdBus := servicebus.NewCommandBus(bus)
	qryBus := servicebus.NewQueryBus(bus)
	_ = cmdBus.DispatchNow(ctx, RenameUser{ID: "u-1", NewName: "Jill"})
	_, _ = qryBus.Ask(ctx, GetUser{ID: "u-1"})

	// Chain and Batch execution.
	_ = bus.Chain(ctx,
		RenameUser{ID: "u-1", NewName: "A"},
		RenameUser{ID: "u-1", NewName: "B"},
	)

	batchErr := bus.Batch(ctx,
		[]cbus.Command{
			RenameUser{ID: "u-1", NewName: "C"},
			RenameUser{ID: "u-1", NewName: "D"},
		},
		servicebus.WithBatchProgress(func(done, total int) { slog.Info("batch", "done", done, "total", total) }),
		servicebus.WithBatchOnError(func(i int, cmd cbus.Command, err error) { slog.Error("batch error", "i", i, "err", err) }),
	)
	if batchErr != nil && !errors.Is(batchErr, context.Canceled) {
		slog.Warn("batch completed with errors", "err", batchErr)
	}

	// For demo visibility, show what the in-memory adapter recorded.
	slog.Info("enqueued commands", "count", len(ad.Commands))
	slog.Info("enqueued listeners", "count", len(ad.Listeners))
}
