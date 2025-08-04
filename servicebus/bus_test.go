package servicebus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
	"github.com/hbttundar/scg-service-bus/servicebus"
)

type testCmd struct{ ID string }

type testQry struct{ ID string }

type testRes struct{ ID string }

type testDom struct{ ID string }

type queueCmd struct{ ID string }

func (queueCmd) QueueName() string    { return "commands" }
func (queueCmd) Delay() time.Duration { return time.Second }

type qListener struct{}

func (qListener) Handle(ctx context.Context, e testDom) error { return nil }
func (qListener) QueueName() string                           { return "listeners" }
func (qListener) Delay() time.Duration                        { return 2 * time.Second }

// fakes

type fakeEnq struct {
	cmds         []any
	listeners    []string
	cmdOpts      []cbus.QueueOptions
	listenerOpts []cbus.QueueOptions
}

func (f *fakeEnq) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	f.cmds = append(f.cmds, cmd)
	f.cmdOpts = append(f.cmdOpts, opts)

	return nil
}
func (f *fakeEnq) EnqueueListener(
	ctx context.Context,
	evt cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	f.listeners = append(f.listeners, handler)
	f.listenerOpts = append(f.listenerOpts, opts)

	return nil
}

type testOut struct{ T string }

func (o testOut) Topic() string { return o.T }

type fakePub struct {
	events []cbus.IntegrationEvent
	opts   []cbus.PublishOptions
}

func (f *fakePub) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	f.events = append(f.events, e)
	f.opts = append(f.opts, opts)

	return nil
}

func Test_BindAndErrors(t *testing.T) {
	b := servicebus.New(nil, nil, nil)
	if err := b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error { return nil }); err != nil {
		t.Fatalf("bind cmd: %v", err)
	}

	err := b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error { return nil })
	if !errors.Is(err, berr.ErrHandlerExists) {
		t.Fatalf("want ErrHandlerExists, got %v", err)
	}

	if err := b.DispatchSync(t.Context(), struct{ X int }{1}); !errors.Is(err, berr.ErrHandlerNotFound) {
		t.Fatalf("want ErrHandlerNotFound, got %v", err)
	}

	b2 := servicebus.New(nil, nil, nil)
	_ = b2.BindQueryOf(testQry{}, func(ctx context.Context, q any) (any, error) { return 1, nil })

	_, err = servicebus.Ask[testQry, testRes](t.Context(), b2, testQry{ID: "g1"})
	if !errors.Is(err, berr.ErrHandlerTypeMismatch) {
		t.Fatalf("want ErrHandlerTypeMismatch, got %v", err)
	}
}

func Test_DispatchSync_Ask_PublishDomain(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	var seen []testCmd

	_ = b.BindCommandOf(testCmd{}, func(ctx context.Context, v any) error {
		seen = append(seen, v.(testCmd))
		return nil
	})

	if err := b.DispatchSync(t.Context(), testCmd{ID: "c1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(seen) != 1 || seen[0].ID != "c1" {
		t.Fatalf("seen=%v", seen)
	}

	_ = b.BindQueryOf(testQry{}, func(ctx context.Context, q any) (any, error) {
		return testRes{ID: q.(testQry).ID}, nil
	})

	raw, err := b.Ask(t.Context(), testQry{ID: "g1"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}

	if raw.(testRes).ID != "g1" {
		t.Fatalf("bad res: %+v", raw)
	}

	calls := 0
	_ = b.BindDomainEventOf(testDom{}, func(ctx context.Context, e any) error {
		calls++
		return nil
	})

	if err := b.PublishDomain(t.Context(), testDom{ID: "d1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func Test_QueueablePaths(t *testing.T) {
	enq := &fakeEnq{}
	b := servicebus.New(enq, nil, nil)

	if err := b.Dispatch(t.Context(), queueCmd{ID: "a1"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(enq.cmds) != 1 {
		t.Fatalf("want 1 enqueued cmd, got %d", len(enq.cmds))
	}

	if err := servicebus.BindDomainEvent[testDom](b, qListener{}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	if err := b.PublishDomain(t.Context(), testDom{ID: "d1"}); err != nil {
		t.Fatalf("publish dom: %v", err)
	}

	if len(enq.listeners) != 1 {
		t.Fatalf("want 1 enqueued listener, got %d", len(enq.listeners))
	}
}

func Test_PublishIntegrationErrors(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	err := b.PublishIntegration(t.Context(), testOut{T: "orders"}, cbus.PublishOptions{Key: "k"})
	if !errors.Is(err, berr.ErrAsyncNotConfigured) {
		t.Fatalf("want ErrAsyncNotConfigured, got %v", err)
	}

	pub := &fakePub{}

	b = servicebus.New(nil, pub, nil)

	err = b.PublishIntegration(t.Context(), testOut{T: "orders"}, cbus.PublishOptions{Key: "k"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(pub.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(pub.events))
	}
}
