package inmemory

import (
	"context"
	"sync"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
)

// Enqueuer is a thread-safe in-memory implementation of cbus.JobEnqueuer.
// It records enqueued commands and listeners for testing and examples.
type Enqueuer struct {
	mu        sync.Mutex
	Commands  []cbus.Command
	Listeners []string
}

func (e *Enqueuer) EnqueueCommand(
	ctx context.Context,
	cmd cbus.Command,
	opts cbus.QueueOptions,
) error {
	e.mu.Lock()
	e.Commands = append(e.Commands, cmd)
	e.mu.Unlock()

	return nil
}

func (e *Enqueuer) EnqueueListener(
	ctx context.Context,
	ev cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	e.mu.Lock()
	e.Listeners = append(e.Listeners, handler)
	e.mu.Unlock()

	return nil
}

// Publisher is a thread-safe in-memory implementation of cbus.EventPublisher.
type Publisher struct {
	mu     sync.Mutex
	Events []cbus.IntegrationEvent
}

func (p *Publisher) PublishIntegration(
	ctx context.Context,
	e cbus.IntegrationEvent,
	opts cbus.PublishOptions,
) error {
	p.mu.Lock()
	p.Events = append(p.Events, e)
	p.mu.Unlock()

	return nil
}

// Adapter combines Enqueuer and Publisher to satisfy both interfaces.
// Use with servicebus.WithAdapter(inmemory.New()).
type Adapter struct {
	Enqueuer
	Publisher
}

// Ensure Adapter implements the combined contract.
var _ cbus.Adapter = (*Adapter)(nil)

// New creates a new in-memory adapter instance.
func New() *Adapter { return &Adapter{} }
