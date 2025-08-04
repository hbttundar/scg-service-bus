package rabbitmq_test

import (
	"errors"
	"testing"

	"github.com/hbttundar/scg-service-bus/adapters/rabbitmq"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
)

func TestNewWithAMQPConn_EmptyURL(t *testing.T) {
	_, _, err := rabbitmq.NewWithAMQPConn(rabbitmq.Config{URL: "", ConnTimeout: 0})
	if err == nil {
		t.Fatalf("expected error for empty URL")
	}

	if !errors.Is(err, berr.ErrPublishFailed) {
		t.Fatalf("want ErrPublishFailed prefix, got %v", err)
	}
}
