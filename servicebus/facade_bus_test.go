package servicebus_test

import (
	"context"
	"testing"
	"time"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	"github.com/hbttundar/scg-service-bus/servicebus"
)

type fCmd struct{ ID string }

type fQry struct{ K string }

type fRes struct{ V string }

type fDom struct{ N string }

type qlRaw struct{}

func (qlRaw) Handle(ctx context.Context, e fDom) error { return nil }
func (qlRaw) QueueName() string                        { return "fdom" }
func (qlRaw) Delay() time.Duration                     { return 0 }

type fakeEnq2 struct{ listeners []string }

func (f *fakeEnq2) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	return nil
}
func (f *fakeEnq2) EnqueueListener(
	ctx context.Context,
	evt cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	f.listeners = append(f.listeners, handler)
	return nil
}

type fakePub2 struct{}

func (fakePub2) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	return nil
}

func Test_CommandBus_QueryBus_And_AskGeneric(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	// bind command
	_ = b.BindCommandOf(fCmd{}, func(ctx context.Context, v any) error { return nil })

	// use CommandBus Dispatch and DispatchNow
	cb := servicebus.NewCommandBus(b)
	if err := cb.Dispatch(t.Context(), fCmd{ID: "1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if err := cb.DispatchNow(t.Context(), fCmd{ID: "2"}); err != nil {
		t.Fatalf("dispatch now: %v", err)
	}

	// bind query and use QueryBus + AskGeneric
	_ = b.BindQueryOf(fQry{}, func(ctx context.Context, q any) (any, error) { return fRes{V: q.(fQry).K}, nil })

	qb := servicebus.NewQueryBus(b)

	anyRes, err := qb.Ask(t.Context(), fQry{K: "k"})
	if err != nil || anyRes.(fRes).V != "k" {
		t.Fatalf("ask via qb: %v res=%+v", err, anyRes)
	}

	r, err := servicebus.AskGeneric[fQry, fRes](t.Context(), qb, fQry{K: "g"})
	if err != nil || r.V != "g" {
		t.Fatalf("ask generic: %v r=%+v", err, r)
	}
}

func Test_BindDomainEventRaw_EnqueueAndSync(t *testing.T) {
	// Enqueue path
	enq := &fakeEnq2{}
	b := servicebus.New(enq, fakePub2{}, nil)

	_ = b.BindDomainEventRaw(fDom{}, qlRaw{}, func(ctx context.Context, e any) error { return nil })

	if err := b.PublishDomain(t.Context(), fDom{N: "n"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(enq.listeners) != 1 {
		t.Fatalf("expected 1 enqueued listener, got %d", len(enq.listeners))
	}

	// Sync path (no enqueuer)
	b2 := servicebus.New(nil, nil, nil)
	called := 0
	_ = b2.BindDomainEventRaw(fDom{}, struct{}{}, func(ctx context.Context, e any) error { called++; return nil })

	if err := b2.PublishDomain(t.Context(), fDom{N: "n"}); err != nil {
		t.Fatalf("publish2: %v", err)
	}

	if called != 1 {
		t.Fatalf("want called once, got %d", called)
	}
}
