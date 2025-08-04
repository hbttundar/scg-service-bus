package nats_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hbttundar/scg-service-bus/adapters/nats"
	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
)

type fakeClient struct {
	calls []struct {
		subject string
		data    []byte
		headers map[string]string
	}
	err error
}

func (f *fakeClient) Publish(subject string, data []byte, headers map[string]string) error {
	f.calls = append(f.calls, struct {
		subject string
		data    []byte
		headers map[string]string
	}{subject, data, headers})

	return f.err
}

type cmd struct{ ID string }

type ev struct{ Name string }

// integ event with topic

type integ struct{ T string }

func (i integ) Topic() string { return i.T }

func TestNATS_EnqueueCommand_And_PublishIntegration(t *testing.T) {
	fc := &fakeClient{}
	ad := nats.New(fc)

	// Command enqueue with minimal options struct
	qo := cbus.QueueOptions{Queue: "jobs", DelaySeconds: 3, Headers: map[string]string{"h1": "v1"}}
	if err := ad.EnqueueCommand(t.Context(), cmd{ID: "1"}, qo); err != nil {
		t.Fatalf("enqueue cmd: %v", err)
	}

	if len(fc.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fc.calls))
	}

	c := fc.calls[0]
	if c.subject != "cmd.jobs" {
		t.Fatalf("subject mismatch: %s", c.subject)
	}

	if len(c.data) == 0 {
		t.Fatalf("expected data body")
	}

	if c.headers["h1"] != "v1" || c.headers["x-delay"] != "3" {
		t.Fatalf("headers missing or wrong: %+v", c.headers)
	}

	// PublishIntegration with struct options
	po := cbus.PublishOptions{TopicOverride: "orders", Key: "k", Headers: map[string]string{"ph": "pv"}}
	if err := ad.PublishIntegration(t.Context(), integ{T: "unused"}, po); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(fc.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fc.calls))
	}

	p := fc.calls[1]
	if p.subject != "orders" {
		t.Fatalf("topic mismatch: %s", p.subject)
	}

	if p.headers["key"] != "k" || p.headers["ph"] != "pv" {
		t.Fatalf("publish headers mismatch: %+v", p.headers)
	}
}

func TestNATS_NilClientError(t *testing.T) {
	ad := nats.New(nil)

	if err := ad.EnqueueCommand(t.Context(), cmd{ID: "x"}, cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected error for nil client")
	}

	if err := ad.EnqueueListener(t.Context(), ev{}, "H", cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected error for nil client")
	}

	if err := ad.PublishIntegration(t.Context(), integ{T: "t"}, cbus.PublishOptions{}); err == nil {
		t.Fatalf("expected error for nil client")
	}
}

func TestNATS_Publish_ErrorWrapping_And_ContextCancel(t *testing.T) {
	// client returns generic error -> should wrap
	fc := &fakeClient{err: errors.New("boom")}
	ad := nats.New(fc)

	if err := ad.EnqueueCommand(t.Context(), cmd{ID: "x"}, cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected wrapped error")
	}

	// client returns context.Canceled -> propagate as-is
	fc2 := &fakeClient{err: context.Canceled}
	ad2 := nats.New(fc2)

	err := ad2.PublishIntegration(t.Context(), integ{T: "t"}, cbus.PublishOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestNATS_SubjectForListener_WithQueueOverride(t *testing.T) {
	fc := &fakeClient{}
	ad := nats.New(fc)

	if err := ad.EnqueueListener(t.Context(), ev{Name: "e"}, "H", cbus.QueueOptions{Queue: "Q"}); err != nil {
		t.Fatalf("enqueue listener: %v", err)
	}

	if len(fc.calls) != 1 || fc.calls[0].subject != "cmd.Q" {
		t.Fatalf("subject=%v", fc.calls[0].subject)
	}
}
