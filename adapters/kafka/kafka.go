package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
)

const (
	jobsPrefix      = "jobs."
	listenersPrefix = "listeners."
)

// Writer is a minimal Kafka-like writer interface.
// Users can adapt segmentio/kafka-go or any other client to this.
type Writer interface {
	Write(topic string, key, value []byte, headers map[string]string) error
}

// Adapter implements cbus.Adapter using an injected Writer.
type Adapter struct {
	Writer Writer
}

var _ cbus.Adapter = (*Adapter)(nil)

// New creates a new Kafka adapter instance with the provided writer.
func New(w Writer) *Adapter { return &Adapter{Writer: w} }

func (a *Adapter) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if a.Writer == nil {
		return fmt.Errorf("kafka enqueue: %w", berr.ErrEnqueueFailed)
	}

	val, err := mustJSON(cmd)
	if err != nil {
		return fmt.Errorf("kafka enqueue serialize: %w", berr.ErrSerializationFailed)
	}

	topic := topicForCommand(cmd, opts)
	headers := queueHeaders(opts)

	if err = a.Writer.Write(topic, nil, val, headers); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		// separate return from preceding multi-line block (wsl)
		return fmt.Errorf("kafka enqueue write: %w", errors.Join(berr.ErrEnqueueFailed, err))
	}

	return nil
}

func (a *Adapter) EnqueueListener(
	ctx context.Context,
	e cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if a.Writer == nil {
		return fmt.Errorf("kafka enqueue listener: %w", berr.ErrEnqueueFailed)
	}

	val, err := mustJSON(e)
	if err != nil {
		return fmt.Errorf("kafka enqueue listener serialize: %w", berr.ErrSerializationFailed)
	}

	topic := topicForListener(e, handler, opts)
	headers := queueHeaders(opts)

	if err = a.Writer.Write(topic, nil, val, headers); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		// separate return from preceding multi-line block (wsl)
		return fmt.Errorf("kafka enqueue listener write: %w", errors.Join(berr.ErrEnqueueFailed, err))
	}

	return nil
}

func (a *Adapter) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if a.Writer == nil {
		return fmt.Errorf("kafka publish: %w", berr.ErrPublishFailed)
	}

	val, err := mustJSON(e)
	if err != nil {
		return fmt.Errorf("kafka publish serialize: %w", berr.ErrSerializationFailed)
	}

	topic := topicForEvent(e, opts)
	key := []byte(opts.Key)
	headers := publishHeaders(opts)

	if err = a.Writer.Write(topic, key, val, headers); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		// separate return from preceding multi-line block (wsl)
		return fmt.Errorf("kafka publish write: %w", errors.Join(berr.ErrPublishFailed, err))
	}

	return nil
}

// helpers (duplicated for simplicity and test isolation)

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	name := t.Name()
	if name == "" {
		name = t.String()
	}

	return name
}

func topicForCommand(cmd any, o cbus.QueueOptions) string {
	if o.Queue != "" {
		return jobsPrefix + o.Queue
	}

	return jobsPrefix + typeName(cmd)
}

func topicForListener(e any, handler string, o cbus.QueueOptions) string {
	if o.Queue != "" {
		return jobsPrefix + o.Queue
	}

	return listenersPrefix + typeName(e) + "." + handler
}

func topicForEvent(e cbus.IntegrationEvent, o cbus.PublishOptions) string {
	if o.TopicOverride != "" {
		return o.TopicOverride
	}

	return e.Topic()
}

func queueHeaders(o cbus.QueueOptions) map[string]string {
	h := make(map[string]string, len(o.Headers)+1)
	for k, v := range o.Headers {
		h[k] = v
	}

	if o.DelaySeconds > 0 {
		h["x-delay"] = fmt.Sprint(o.DelaySeconds)
	}

	return h
}

func publishHeaders(o cbus.PublishOptions) map[string]string {
	h := make(map[string]string, len(o.Headers))
	for k, v := range o.Headers {
		h[k] = v
	}

	return h
}

func mustJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return b, nil
}
