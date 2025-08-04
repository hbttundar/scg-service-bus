package nats

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
	cmdPrefix      = "cmd."
	listenerPrefix = "listeners."
)

// Client is a minimal NATS-like publisher interface decoupled from any concrete library.
// Users can provide a wrapper around their NATS connection to satisfy this.
type Client interface {
	// Publish publishes a message to a subject with optional headers.
	Publish(subject string, data []byte, headers map[string]string) error
}

// Adapter implements cbus.Adapter using an injected NATS-like Client.
type Adapter struct {
	Client Client
}

// Ensure Adapter implements the combined contract.
var _ cbus.Adapter = (*Adapter)(nil)

// New creates a new NATS adapter instance with the provided client.
func New(c Client) *Adapter { return &Adapter{Client: c} }

func (a *Adapter) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	subj := subjectForCommand(cmd, opts)
	headers := queueHeaders(opts)

	sa := &serializeArgs{
		subject: subj,
		payload: cmd,
		headers: headers,
		serr:    berr.ErrSerializationFailed,
		wrap:    berr.ErrEnqueueFailed,
		label:   "enqueue",
	}

	return a.buildAndSend(ctx, sa)
}

func (a *Adapter) EnqueueListener(
	ctx context.Context,
	e cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	subj := subjectForListener(e, handler, opts)
	headers := queueHeaders(opts)

	sa := &serializeArgs{
		subject: subj,
		payload: e,
		headers: headers,
		serr:    berr.ErrSerializationFailed,
		wrap:    berr.ErrEnqueueFailed,
		label:   "enqueue listener",
	}

	return a.buildAndSend(ctx, sa)
}

func (a *Adapter) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	subj := topicForEvent(e, opts)
	headers := publishHeaders(opts)

	sa := &serializeArgs{
		subject: subj,
		payload: e,
		headers: headers,
		serr:    berr.ErrSerializationFailed,
		wrap:    berr.ErrPublishFailed,
		label:   "publish",
	}

	return a.buildAndSend(ctx, sa)
}

func (a *Adapter) buildAndSend(ctx context.Context, sa *serializeArgs) error {
	if err := a.ready(ctx, sa.wrap, sa.label); err != nil {
		return err
	}

	return a.serializeAndPublish(ctx, sa)
}

type publishArgs struct {
	subject string
	body    []byte
	headers map[string]string
	wrap    error
	label   string
}

func (a *Adapter) publish(ctx context.Context, args *publishArgs) error {
	if err := a.Client.Publish(args.subject, args.body, args.headers); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		return fmt.Errorf("nats %s publish: %w", args.label, errors.Join(args.wrap, err))
	}

	return nil
}

func (a *Adapter) ready(ctx context.Context, base error, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if a.Client == nil {
		return fmt.Errorf("nats %s: %w", label, base)
	}

	return nil
}

type serializeArgs struct {
	subject string
	payload any
	headers map[string]string
	serr    error
	wrap    error
	label   string
}

func (a *Adapter) serializeAndPublish(ctx context.Context, sa *serializeArgs) error {
	body, err := mustJSON(sa.payload)
	if err != nil {
		return fmt.Errorf("nats %s serialize: %w", sa.label, errors.Join(sa.serr, err))
	}

	args := &publishArgs{
		subject: sa.subject,
		body:    body,
		headers: sa.headers,
		wrap:    sa.wrap,
		label:   sa.label,
	}

	return a.publish(ctx, args)
}

// helpers

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	name := t.Name()
	if name == "" { // unnamed (e.g., map/struct literal)
		name = t.String()
	}

	return name
}

func subjectForCommand(cmd any, o cbus.QueueOptions) string {
	if o.Queue != "" {
		return cmdPrefix + o.Queue
	}

	return cmdPrefix + typeName(cmd)
}

func subjectForListener(e any, handler string, o cbus.QueueOptions) string {
	if o.Queue != "" {
		return cmdPrefix + o.Queue
	}

	return listenerPrefix + typeName(e) + "." + handler
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
	h := make(map[string]string, len(o.Headers)+1)
	for k, v := range o.Headers {
		h[k] = v
	}

	if o.Key != "" {
		h["key"] = o.Key
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
