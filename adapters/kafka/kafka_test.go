package kafka_test

import (
	"testing"

	"github.com/hbttundar/scg-service-bus/adapters/kafka"
	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
)

// Unified Kafka adapter tests (single file).

type fakeWriter struct {
	calls []struct {
		topic   string
		key     []byte
		value   []byte
		headers map[string]string
	}
	err error
}

func (f *fakeWriter) Write(topic string, key, value []byte, headers map[string]string) error {
	f.calls = append(f.calls, struct {
		topic   string
		key     []byte
		value   []byte
		headers map[string]string
	}{topic, key, value, headers})

	return f.err
}

type cmd struct{ ID string }

type ev struct{ Name string }

func (ev) Topic() string { return "evt.orders" }

type ev2 struct{ X int }

type fakeWriter2 struct {
	calls []struct {
		topic string
		key   []byte
	}
}

func (f *fakeWriter2) Write(topic string, key, value []byte, headers map[string]string) error {
	f.calls = append(f.calls, struct {
		topic string
		key   []byte
	}{topic, key})

	return nil
}

func TestKafka_EnqueueCommand_And_PublishIntegration(t *testing.T) {
	fw := &fakeWriter{}
	ad := kafka.New(fw)

	qo := cbus.QueueOptions{Queue: "jobs", DelaySeconds: 5, Headers: map[string]string{"h": "1"}}
	if err := ad.EnqueueCommand(t.Context(), cmd{ID: "7"}, qo); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if len(fw.calls) != 1 {
		t.Fatalf("want 1, got %d", len(fw.calls))
	}

	c := fw.calls[0]

	if c.topic != "jobs.jobs" {
		t.Fatalf("topic: %s", c.topic)
	}

	if len(c.value) == 0 {
		t.Fatalf("value empty")
	}

	if c.headers["h"] != "1" || c.headers["x-delay"] != "5" {
		t.Fatalf("headers: %+v", c.headers)
	}

	po := cbus.PublishOptions{TopicOverride: "evt.orders", Key: "key1", Headers: map[string]string{"ph": "pv"}}

	if err := ad.PublishIntegration(t.Context(), ev{Name: "E"}, po); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(fw.calls) != 2 {
		t.Fatalf("want 2, got %d", len(fw.calls))
	}

	p := fw.calls[1]
	if p.topic != "evt.orders" {
		t.Fatalf("topic: %s", p.topic)
	}

	if string(p.key) != "key1" {
		t.Fatalf("key: %s", string(p.key))
	}

	if p.headers["ph"] != "pv" {
		t.Fatalf("pub headers: %+v", p.headers)
	}
}

func TestKafka_NilWriterError(t *testing.T) {
	ad := kafka.New(nil)
	if err := ad.EnqueueCommand(t.Context(), cmd{}, cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected error")
	}

	if err := ad.EnqueueListener(t.Context(), ev{}, "H", cbus.QueueOptions{}); err == nil {
		t.Fatalf("expected error")
	}

	if err := ad.PublishIntegration(t.Context(), ev{Name: "E"}, cbus.PublishOptions{}); err == nil {
		t.Fatalf("expected error")
	}
}

// For default topic test, make ev2 implement IntegrationEvent when used

type ev2WithTopic struct{ X int }

func (e ev2WithTopic) Topic() string { return "evt.ev2" }

func TestKafka_EnqueueListener_DefaultTopic(t *testing.T) {
	fw := &fakeWriter2{}
	ad := kafka.New(fw)

	err := ad.EnqueueListener(t.Context(), ev2{X: 1}, "Handler", cbus.QueueOptions{})
	if err != nil {
		t.Fatalf("enqueue listener: %v", err)
	}

	if len(fw.calls) != 1 {
		t.Fatalf("calls: %d", len(fw.calls))
	}

	if fw.calls[0].topic != "listeners.ev2.Handler" {
		t.Fatalf("topic: %s", fw.calls[0].topic)
	}
}

func TestKafka_Publish_DefaultTopic_WithPointerEvent(t *testing.T) {
	fw := &fakeWriter2{}
	ad := kafka.New(fw)
	e := &ev2WithTopic{X: 2}

	err := ad.PublishIntegration(t.Context(), e, cbus.PublishOptions{})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if len(fw.calls) != 1 {
		t.Fatalf("calls: %d", len(fw.calls))
	}

	if fw.calls[0].topic != "evt.ev2" {
		t.Fatalf("topic: %s", fw.calls[0].topic)
	}
}
