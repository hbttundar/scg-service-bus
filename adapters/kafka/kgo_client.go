//go:build franz

package kafka

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"

	berr "github.com/hbttundar/scg-service-bus/contract/errors"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Concrete franz-go based constructor and writer wrapper.

type SASLConfig struct {
	Mechanism string // not implemented yet; placeholder
	Username  string
	Password  string
}

type Config struct {
	Brokers     []string
	TLS         *tls.Config
	SASL        *SASLConfig
	Acks        kgo.Acks
	Idempotent  bool
	ClientID    string
	Compression kgo.CompressionType
}

type kgoWriter struct{ cl *kgo.Client }

func (w kgoWriter) Write(topic string, key, value []byte, headers map[string]string) error {
	rec := &kgo.Record{Topic: topic, Key: key, Value: value}
	if len(headers) > 0 {
		rec.Headers = make([]kgo.RecordHeader, 0, len(headers))
		for k, v := range headers {
			rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
		}
	}
	return w.cl.ProduceSync(context.Background(), rec).FirstErr()
}

// NewWithKgo builds a franz-go client based Adapter. The returned cleanup should be called to close the client.
func NewWithKgo(cfg Config) (*Adapter, func(), error) {
	if len(cfg.Brokers) == 0 {
		return nil, nil, fmt.Errorf("%w: kafka brokers required", berr.ErrPublishFailed)
	}
	opts := []kgo.Opt{kgo.SeedBrokers(cfg.Brokers...)}
	if cfg.ClientID != "" {
		opts = append(opts, kgo.ClientID(cfg.ClientID))
	}
	if cfg.TLS != nil {
		opts = append(opts, kgo.DialTLSConfig(cfg.TLS))
	}
	if cfg.Idempotent {
		opts = append(opts, kgo.IdempotentProducer())
		if cfg.Compression != 0 {
			opts = append(opts, kgo.ProducerBatchCompression(cfg.Compression))
		}
	}
	if cfg.Acks != 0 {
		opts = append(opts, kgo.RequiredAcks(cfg.Acks))
	}
	// Minimal SASL validation hook
	if cfg.SASL != nil && cfg.SASL.Mechanism != "" {
		return nil, nil, fmt.Errorf("%w: SASL mechanism not configured in adapter", berr.ErrPublishFailed)
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: kafka client init: %w", berr.ErrPublishFailed, err)
	}
	ad := New(kgoWriter{cl: cl})
	cleanup := func() { cl.Close() }
	return ad, cleanup, nil
}

// Ensure error wrapping parity for context errors similar to reference implementations.
func wrapProduceErr(topic string, err error, publish bool) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if publish {
		return fmt.Errorf("%w: kafka publish to %q: %w", berr.ErrPublishFailed, topic, err)
	}
	return fmt.Errorf("%w: kafka enqueue to %q: %w", berr.ErrEnqueueFailed, topic, err)
}

// marshal helper (kept local to avoid exporting the existing mustJSON)
func marshalJSON(v any) ([]byte, error) { return json.Marshal(v) }
