package nats

import (
	"fmt"
	"time"

	berr "github.com/hbttundar/scg-service-bus/contract/errors"
	"github.com/nats-io/nats.go"
)

// Concrete NATS connection-backed Client and constructor.

type Config struct {
	URL           string
	Name          string
	ConnTimeout   time.Duration
	MaxReconnects int
}

type natsClient struct{ nc *nats.Conn }

func (c natsClient) Publish(subject string, data []byte, headers map[string]string) error {
	msg := &nats.Msg{Subject: subject, Data: data}

	var h nats.Header
	if len(headers) > 0 {
		h = nats.Header{}
		for k, v := range headers {
			h.Add(k, v)
		}
	}

	msg.Header = h

	if err := c.nc.PublishMsg(msg); err != nil {
		return err
	}

	return c.nc.Flush()
}

// NewWithNATS creates a real NATS connection and returns an Adapter and a cleanup.
func NewWithNATS(cfg Config) (*Adapter, func(), error) {
	if cfg.URL == "" {
		return nil, nil, fmt.Errorf("%w: nats url required", berr.ErrPublishFailed)
	}

	opts := []nats.Option{}
	if cfg.Name != "" {
		opts = append(opts, nats.Name(cfg.Name))
	}

	if cfg.ConnTimeout > 0 {
		opts = append(opts, nats.Timeout(cfg.ConnTimeout))
	}

	if cfg.MaxReconnects != 0 {
		opts = append(opts, nats.MaxReconnects(cfg.MaxReconnects))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: nats connect: %w", berr.ErrPublishFailed, err)
	}

	ad := New(natsClient{nc: nc})
	cleanup := func() {
		if nc != nil && !nc.IsClosed() {
			_ = nc.Drain() //nolint:errcheck // best-effort shutdown; cannot return error here
			nc.Close()
		}
	}

	return ad, cleanup, nil
}
