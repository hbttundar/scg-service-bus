package servicebus

// revive:disable:max-public-structs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
)

// revive:disable:max-public-structs
// Bus is a thin in-process mediator with an internal binder.
// It supports synchronous command/query handling and domain event publication,
// and integrates with async adapters for command enqueueing and integration events.
//
// Bus is concurrency-safe and contains no global state.
type Bus struct {
	mu sync.RWMutex

	cmd map[reflect.Type]func(ctx context.Context, cmd any) error
	qry map[reflect.Type]func(ctx context.Context, q any) (any, error)
	dom map[reflect.Type][]domainEntry

	// global command middleware executed in registration order
	cmdMW []CommandMiddleware

	enq    cbus.JobEnqueuer
	pub    cbus.EventPublisher
	logger *slog.Logger
}

// CommandBus is a thin facade over Bus for commands.
type CommandBus struct{ b *Bus }

// NewCommandBus constructs a CommandBus over a Bus.
func NewCommandBus(b *Bus) *CommandBus { return &CommandBus{b: b} }

// Dispatch dispatches a command using the underlying Bus.
func (c *CommandBus) Dispatch(ctx context.Context, cmd cbus.Command) error {
	return c.b.Dispatch(ctx, cmd)
}

// DispatchNow executes a command synchronously using the underlying Bus.
func (c *CommandBus) DispatchNow(ctx context.Context, cmd cbus.Command) error {
	return c.b.DispatchNow(ctx, cmd)
}

// QueryBus is a thin facade over Bus for queries.
type QueryBus struct{ b *Bus }

// NewQueryBus constructs a QueryBus over a Bus.
func NewQueryBus(b *Bus) *QueryBus { return &QueryBus{b: b} }

// revive:enable:max-public-structs

// Ask executes an untyped query using the underlying Bus.
func (q *QueryBus) Ask(ctx context.Context, query any) (any, error) { return q.b.Ask(ctx, query) }

// AskGeneric is a typed helper to execute queries via a QueryBus.
func AskGeneric[Q cbus.Query, R any](ctx context.Context, qb *QueryBus, query Q) (R, error) {
	return Ask[Q, R](ctx, qb.b, query)
}

// New constructs a new Bus with optional enqueuer and publisher.
func New(jobs cbus.JobEnqueuer, pub cbus.EventPublisher, logger *slog.Logger) *Bus { //nolint:ireturn
	return &Bus{
		cmd:    make(map[reflect.Type]func(context.Context, any) error),
		qry:    make(map[reflect.Type]func(context.Context, any) (any, error)),
		dom:    make(map[reflect.Type][]domainEntry),
		enq:    jobs,
		pub:    pub,
		logger: logger,
	}
}

// WithCommandMiddleware registers global command middleware via an option.
func WithCommandMiddleware(mw ...CommandMiddleware) BusOption {
	return func(b *Bus) { b.cmdMW = append(b.cmdMW, mw...) }
}

// BindCommandOf registers a handler for a specific command type.
// Provide a zero value of the command type via sample.
func (b *Bus) BindCommandOf(sample any, handler func(ctx context.Context, cmd any) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := reflect.TypeOf(sample)
	if _, exists := b.cmd[t]; exists {
		return fmt.Errorf("bind command %s: %w", t.String(), berr.ErrHandlerExists)
	}

	b.cmd[t] = func(ctx context.Context, v any) error { return handler(ctx, v) }

	return nil
}

// BindQueryOf registers a handler for a specific query type returning any result.
func (b *Bus) BindQueryOf(sample any, handler func(ctx context.Context, q any) (any, error)) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := reflect.TypeOf(sample)
	if _, exists := b.qry[t]; exists {
		return fmt.Errorf("bind query %s: %w", t.String(), berr.ErrHandlerExists)
	}

	b.qry[t] = func(ctx context.Context, v any) (any, error) { return handler(ctx, v) }

	return nil
}

// BindDomainEventOf registers a domain event handler for a specific event type.
// For queueable listeners, prefer BindDomainEventRaw with a raw handler that implements QueueableListener.
func (b *Bus) BindDomainEventOf(sample any, handler func(ctx context.Context, e any) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := reflect.TypeOf(sample)
	entry := domainEntry{call: handler, raw: handler}
	b.dom[t] = append(b.dom[t], entry)

	return nil
}

// BindDomainEventRaw registers a domain event handler providing a raw handler object and a callable.
// The raw object is used for QueueableListener detection and name resolution when enqueuing.
func (b *Bus) BindDomainEventRaw(sample, raw any, call func(ctx context.Context, e any) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := reflect.TypeOf(sample)
	entry := domainEntry{call: call, raw: raw}
	b.dom[t] = append(b.dom[t], entry)

	return nil
}

// Ask executes a query handler synchronously and returns an untyped result.
func (b *Bus) Ask(ctx context.Context, q any) (any, error) {
	b.mu.RLock()
	f, ok := b.qry[reflect.TypeOf(q)]
	b.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("ask %s: %w", reflect.TypeOf(q).String(), berr.ErrHandlerNotFound)
	}

	return f(ctx, q)
}

type domainEntry struct {
	call func(ctx context.Context, e any) error
	raw  any // original handler, for QueueableListener detection
}

// BusOption configures a Bus instance.
type BusOption func(*Bus)

// CommandMiddleware wraps command handler execution. Middlewares are executed in registration order.
type CommandMiddleware func(next func(ctx context.Context, cmd any) error) func(ctx context.Context, cmd any) error

// BindCommand registers a handler for command type C. Duplicate bindings are rejected.
func BindCommand[C cbus.Command](b *Bus, h cbus.CommandHandler[C]) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var zero C

	t := reflect.TypeOf(zero)

	if _, exists := b.cmd[t]; exists {
		return fmt.Errorf("bind command %s: %w", t.String(), berr.ErrHandlerExists)
	}

	b.cmd[t] = func(ctx context.Context, v any) error {
		c, ok := v.(C)
		if !ok {
			return fmt.Errorf("dispatch %s: %w", reflect.TypeOf(v).String(), berr.ErrHandlerTypeMismatch)
		}

		return h.Handle(ctx, c)
	}

	return nil
}

// BindQuery registers a handler for query type Q producing R. Duplicate bindings are rejected.
func BindQuery[Q cbus.Query, R any](b *Bus, h cbus.QueryHandler[Q, R]) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var zero Q
	t := reflect.TypeOf(zero)

	if _, exists := b.qry[t]; exists {
		return fmt.Errorf("bind query %s: %w", t.String(), berr.ErrHandlerExists)
	}

	b.qry[t] = func(ctx context.Context, v any) (any, error) {
		q, ok := v.(Q)
		if !ok {
			return nil, fmt.Errorf("ask %s: %w", reflect.TypeOf(v).String(), berr.ErrHandlerTypeMismatch)
		}

		return h.Handle(ctx, q)
	}

	return nil
}

// BindDomainEvent registers a domain event handler. Multiple handlers are allowed.
func BindDomainEvent[E cbus.DomainEvent](b *Bus, h cbus.DomainEventHandler[E]) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var zero E
	t := reflect.TypeOf(zero)
	entry := domainEntry{
		call: func(ctx context.Context, v any) error {
			e, ok := v.(E)
			if !ok {
				return fmt.Errorf("publish domain %s: %w", reflect.TypeOf(v).String(), berr.ErrHandlerTypeMismatch)
			}
			return h.Handle(ctx, e)
		},
		raw: h,
	}
	b.dom[t] = append(b.dom[t], entry)

	return nil
}

// Dispatch dispatches a command. If the command implements Queueable and a JobEnqueuer is configured,
// it will be enqueued with minimal QueueOptions; otherwise, it executes synchronously via DispatchSync.
func (b *Bus) Dispatch(ctx context.Context, cmd cbus.Command) error {
	if q, ok := cmd.(cbus.Queueable); ok && b.enq != nil {
		qo := cbus.QueueOptions{Queue: q.QueueName(), DelaySeconds: int(q.Delay().Seconds())}
		return b.enq.EnqueueCommand(ctx, cmd, qo)
	}

	return b.DispatchSync(ctx, cmd)
}

// DispatchSync executes the command handler synchronously (with middleware).
func (b *Bus) DispatchSync(ctx context.Context, cmd cbus.Command) error {
	return b.dispatchWithMiddleware(ctx, cmd)
}

// Ask executes a query handler synchronously and returns the result.
func Ask[Q cbus.Query, R any](ctx context.Context, b *Bus, q Q) (R, error) {
	b.mu.RLock()
	f, ok := b.qry[reflect.TypeOf(q)]
	b.mu.RUnlock()

	var zero R
	if !ok {
		return zero, fmt.Errorf("ask %s: %w", reflect.TypeOf(q).String(), berr.ErrHandlerNotFound)
	}

	res, err := f(ctx, q)
	if err != nil {
		return zero, err
	}

	r, ok := res.(R)
	if !ok {
		return zero, fmt.Errorf("ask %s: %w", reflect.TypeOf(q).String(), berr.ErrHandlerTypeMismatch)
	}

	return r, nil
}

// PublishDomain publishes a domain event to all handlers. Handlers that implement QueueableListener
// are enqueued if a JobEnqueuer is configured; otherwise they are invoked synchronously.
// All errors are aggregated with errors.Join and returned.
func (b *Bus) PublishDomain(ctx context.Context, e cbus.DomainEvent) error {
	b.mu.RLock()
	entries := append([]domainEntry(nil), b.dom[reflect.TypeOf(e)]...)
	b.mu.RUnlock()

	if len(entries) == 0 {
		return nil
	}

	var errs []error

	for _, ent := range entries {
		if err := b.handleDomainEntry(ctx, e, ent); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (b *Bus) handleDomainEntry(
	ctx context.Context,
	event cbus.DomainEvent,
	entry domainEntry,
) error {
	// If no enqueuer, invoke synchronously.
	if b.enq == nil {
		return entry.call(ctx, event)
	}

	// Enqueue if handler is a QueueableListener and enqueuer configured.
	if ql, ok := entry.raw.(cbus.QueueableListener); ok {
		qo := cbus.QueueOptions{Queue: ql.QueueName(), DelaySeconds: int(ql.Delay().Seconds())}
		name := reflect.TypeOf(entry.raw).String()

		if err := b.enq.EnqueueListener(ctx, event, name, qo); err != nil {
			return err
		}

		return nil
	}

	// Fallback to sync invocation.
	return entry.call(ctx, event)
}

// PublishIntegration publishes an integration event via the configured EventPublisher.
func (b *Bus) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	if b.pub == nil {
		return fmt.Errorf("publish integration %T: %w", e, berr.ErrAsyncNotConfigured)
	}

	return b.pub.PublishIntegration(ctx, e, opts)
}

// DispatchNow is a deprecation-friendly alias for DispatchSync.
func (b *Bus) DispatchNow(ctx context.Context, cmd cbus.Command) error {
	return b.DispatchSync(ctx, cmd)
}

// DispatchWithMiddleware executes a command with additional per-call middleware.
func (b *Bus) DispatchWithMiddleware(ctx context.Context, cmd cbus.Command, mws ...CommandMiddleware) error {
	return b.dispatchWithMiddleware(ctx, cmd, mws...)
}

func (b *Bus) dispatchWithMiddleware(ctx context.Context, cmd cbus.Command, mws ...CommandMiddleware) error {
	b.mu.RLock()
	f, ok := b.cmd[reflect.TypeOf(cmd)]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("dispatch %s: %w", reflect.TypeOf(cmd).String(), berr.ErrHandlerNotFound)
	}

	// Combine global and per-call middleware
	chain := make([]CommandMiddleware, 0, len(b.cmdMW)+len(mws))
	chain = append(chain, b.cmdMW...)
	chain = append(chain, mws...)

	// Build chain so the first registered middleware runs first
	final := f
	for i := len(chain) - 1; i >= 0; i-- {
		final = chain[i](final)
	}

	return final(ctx, cmd)
}

// Chain executes commands in order and stops on the first error.
func (b *Bus) Chain(ctx context.Context, cmds ...cbus.Command) error {
	for _, c := range cmds {
		if err := b.dispatchWithMiddleware(ctx, c); err != nil {
			return err
		}
	}

	return nil
}

// revive:disable:max-public-structs
// BatchOptions controls Batch execution behavior.
// OnProgress is called after each command completes (success or failure) with done and total.
// OnError is called when a command returns an error with its index, the command value, and the error.
type BatchOptions struct {
	OnProgress func(done, total int)
	OnError    func(index int, cmd cbus.Command, err error)
}

// revive:enable:max-public-structs

// BatchOpt configures BatchOptions.
type BatchOpt func(*BatchOptions)

// WithBatchProgress sets the progress callback.
func WithBatchProgress(fn func(done, total int)) BatchOpt { //nolint:ireturn
	return func(o *BatchOptions) { o.OnProgress = fn }
}

// WithBatchOnError sets the error callback.
func WithBatchOnError(fn func(index int, cmd cbus.Command, err error)) BatchOpt { //nolint:ireturn
	return func(o *BatchOptions) { o.OnError = fn }
}

// Batch executes the provided commands sequentially.
// It respects context cancellation, reports progress, and aggregates errors.
func (b *Bus) Batch(ctx context.Context, cmds []cbus.Command, opts ...BatchOpt) error {
	var o BatchOptions
	for _, f := range opts {
		f(&o)
	}

	total := len(cmds)

	var errs []error

	for i, c := range cmds {
		if err := ctx.Err(); err != nil { // canceled or deadline exceeded
			return errors.Join(append(errs, err)...)
		}

		err := b.dispatchWithMiddleware(ctx, c)
		if err != nil {
			if o.OnError != nil {
				o.OnError(i, c, err)
			}

			errs = append(errs, err)
		}

		if o.OnProgress != nil {
			o.OnProgress(i+1, total)
		}
	}

	return errors.Join(errs...)
}
