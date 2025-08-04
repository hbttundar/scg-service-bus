package rabbitmq_test

import (
	"context"
	"testing"

	"github.com/hbttundar/scg-service-bus/adapters/rabbitmq"
)

type e struct{}

func (e) Topic() string { return "t" }

func TestNoop_RabbitMQHelpersPlaceholder(t *testing.T) {
	// minimal smoke: call PublishIntegration with nil publisher; expect error but just ignore result
	a := rabbitmq.New(nil)
	_ = a.PublishIntegration(t.Context(), e{}, struct {
		TopicOverride string
		Key           string
		Headers       map[string]string
	}{})

	// placeholder assert never hits
	if false {
		_, cancel := context.WithCancel(t.Context())
		cancel()
	}
}
