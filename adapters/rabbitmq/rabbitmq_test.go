package rabbitmq_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hbttundar/scg-service-bus/adapters/rabbitmq"
	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
)

type fakePublisher struct {
	calls []struct {
		exchange   string
		routingKey string
		body       []byte
		headers    map[string]string
	}
	err error
}

func (f *fakePublisher) Publish(
	ctx context.Context,
	m rabbitmq.PubMsg,
) error {
	_ = ctx
	call := struct {
		exchange   string
		routingKey string
		body       []byte
		headers    map[string]string
	}{m.Exchange, m.RoutingKey, m.Body, m.Headers}
	f.calls = append(f.calls, call)

	return f.err
}

type cmd struct{ ID string }

type integ struct{ T string }

func (integ) Topic() string { return "evt.orders" }

type ev struct{ Name string }

func TestRabbitMQ_EnqueueCommand_And_PublishIntegration(t *testing.T) {
	fp := &fakePublisher{}
	ad := rabbitmq.New(fp)

	qo := cbus.QueueOptions{Queue: "jobs", DelaySeconds: 7, Headers: map[string]string{"h": "x"}}
	if err := ad.EnqueueCommand(t.Context(), cmd{ID: "5"}, qo); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if len(fp.calls) != 1 {
		t.Fatalf("want 1, got %d", len(fp.calls))
	}

	c := fp.calls[0]
	if c.exchange != "" || c.routingKey != "cmd.jobs" {
		t.Fatalf("routing: %q %q", c.exchange, c.routingKey)
	}

	if len(c.body) == 0 {
		t.Fatalf("body empty")
	}

	if c.headers["h"] != "x" || c.headers["x-delay"] != "7" {
		t.Fatalf("headers: %+v", c.headers)
	}

	po := cbus.PublishOptions{TopicOverride: "evt.orders", Key: "rk", Headers: map[string]string{"ph": "pv"}}
	if err := ad.PublishIntegration(t.Context(), integ{T: "t"}, po); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(fp.calls) != 2 {
		t.Fatalf("want 2, got %d", len(fp.calls))
	}

	p := fp.calls[1]
	if p.routingKey != "evt.orders" {
		t.Fatalf("routing key: %s", p.routingKey)
	}

	if p.headers["ph"] != "pv" {
		t.Fatalf("pub headers: %+v", p.headers)
	}
}

func TestRabbitMQ_NilPublisherError(t *testing.T) {
	ad := rabbitmq.New(nil)
	if err := ad.EnqueueCommand(t.Context(), cmd{}, cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected error")
	}

	if err := ad.EnqueueListener(t.Context(), ev{}, "H", cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected error")
	}

	if err := ad.PublishIntegration(t.Context(), integ{T: "t"}, cbus.PublishOptions{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRabbitMQ_Publish_ErrorWrapping_And_ContextCancel(t *testing.T) {
	fp := &fakePublisher{err: errors.New("boom")}
	ad := rabbitmq.New(fp)

	if err := ad.EnqueueListener(t.Context(), ev{Name: "e"}, "H", cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected wrapped error")
	}

	fp2 := &fakePublisher{err: context.Canceled}
	ad2 := rabbitmq.New(fp2)

	err := ad2.PublishIntegration(t.Context(), integ{T: "evt.orders"}, cbus.PublishOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRabbitMQ_EnqueueListener_WithQueueOverride(t *testing.T) {
	fp := &fakePublisher{}
	ad := rabbitmq.New(fp)

	if err := ad.EnqueueListener(t.Context(), ev{Name: "e"}, "H", cbus.QueueOptions{Queue: "Q"}); err != nil {
		t.Fatalf("enqueue listener: %v", err)
	}

	if len(fp.calls) != 1 || fp.calls[0].routingKey != "cmd.Q" {
		t.Fatalf("routing=%v", fp.calls[0].routingKey)
	}
}
