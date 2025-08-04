package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	cmdPrefix      = "cmd."
	listenerPrefix = "listener."
)

type PubMsg struct {
	Exchange   string
	RoutingKey string
	Body       []byte
	Headers    map[string]string
}

type Publisher interface {
	Publish(ctx context.Context, m PubMsg) error
}

type Adapter struct {
	Publisher Publisher
}

var _ cbus.Adapter = (*Adapter)(nil)

func New(p Publisher) *Adapter { return &Adapter{Publisher: p} }

func (a *Adapter) EnqueueCommand(ctx context.Context, cmd cbus.Command, opts cbus.QueueOptions) error {
	if err := a.ready(ctx, berr.ErrEnqueueFailed, "enqueue"); err != nil {
		return err
	}

	rk := routingForCommand(cmd, opts)
	headers := queueHeaders(opts)

	sa := &serializeArgs{
		routingKey: rk,
		payload:    cmd,
		headers:    headers,
		serr:       berr.ErrSerializationFailed,
		wrap:       berr.ErrEnqueueFailed,
		label:      "enqueue",
	}

	return a.serializeAndPublish(ctx, sa)
}

func (a *Adapter) EnqueueListener(
	ctx context.Context,
	e cbus.DomainEvent,
	handler string,
	opts cbus.QueueOptions,
) error {
	if err := a.ready(ctx, berr.ErrEnqueueFailed, "enqueue listener"); err != nil {
		return err
	}

	rk := routingForListener(e, handler, opts)
	headers := queueHeaders(opts)

	sa := &serializeArgs{
		routingKey: rk,
		payload:    e,
		headers:    headers,
		serr:       berr.ErrSerializationFailed,
		wrap:       berr.ErrEnqueueFailed,
		label:      "enqueue listener",
	}

	return a.serializeAndPublish(ctx, sa)
}

func (a *Adapter) PublishIntegration(ctx context.Context, e cbus.IntegrationEvent, opts cbus.PublishOptions) error {
	if err := a.ready(ctx, berr.ErrPublishFailed, "publish"); err != nil {
		return err
	}

	rk := routingForEvent(e, opts)
	headers := publishHeaders(opts)

	sa := &serializeArgs{
		routingKey: rk,
		payload:    e,
		headers:    headers,
		serr:       berr.ErrSerializationFailed,
		wrap:       berr.ErrPublishFailed,
		label:      "publish",
	}

	return a.serializeAndPublish(ctx, sa)
}

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

func routingForCommand(cmd any, o cbus.QueueOptions) string {
	if o.Queue != "" {
		return cmdPrefix + o.Queue
	}

	return cmdPrefix + typeName(cmd)
}

func routingForListener(e any, handler string, o cbus.QueueOptions) string {
	if o.Queue != "" {
		return cmdPrefix + o.Queue
	}

	return listenerPrefix + typeName(e) + "." + handler
}

func routingForEvent(e cbus.IntegrationEvent, o cbus.PublishOptions) string {
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

// internal helpers (serialization + publishing)

type serializeArgs struct {
	routingKey string
	payload    any
	headers    map[string]string
	serr       error
	wrap       error
	label      string
}

type publishArgs struct {
	exchange   string
	routingKey string
	body       []byte
	headers    map[string]string
	wrap       error
	label      string
}

func (a *Adapter) ready(ctx context.Context, base error, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if a.Publisher == nil {
		return fmt.Errorf("rabbitmq %s: %w", label, base)
	}

	return nil
}

func (a *Adapter) serializeAndPublish(ctx context.Context, sa *serializeArgs) error {
	body, err := mustJSON(sa.payload)
	if err != nil {
		return fmt.Errorf("rabbitmq %s serialize: %w", sa.label, errors.Join(sa.serr, err))
	}

	args := &publishArgs{
		exchange:   "",
		routingKey: sa.routingKey,
		body:       body,
		headers:    sa.headers,
		wrap:       sa.wrap,
		label:      sa.label,
	}

	return a.publish(ctx, args)
}

func (a *Adapter) publish(ctx context.Context, args *publishArgs) error {
	msg := PubMsg{
		Exchange:   args.exchange,
		RoutingKey: args.routingKey,
		Body:       args.body,
		Headers:    args.headers,
	}
	if err := a.Publisher.Publish(ctx, msg); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		return fmt.Errorf("rabbitmq %s publish: %w", args.label, errors.Join(args.wrap, err))
	}

	return nil
}

func mustJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return b, nil
}

type amqpChannelPublisher struct{ ch *amqp.Channel }

func (p amqpChannelPublisher) Publish(ctx context.Context, m PubMsg) error {
	var h amqp.Table
	if len(m.Headers) > 0 {
		h = amqp.Table{}
		for k, v := range m.Headers {
			h[k] = v
		}
	}

	return p.ch.PublishWithContext(
		ctx,
		m.Exchange,
		m.RoutingKey,
		false,
		false,
		amqp.Publishing{
			Headers:     h,
			Body:        m.Body,
			ContentType: "application/json",
		},
	)
}

func NewWithAMQPChannel(ch *amqp.Channel) *Adapter {
	return &Adapter{Publisher: amqpChannelPublisher{ch: ch}}
}
