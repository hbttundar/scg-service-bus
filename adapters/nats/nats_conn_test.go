package nats_test

import (
	"errors"
	"testing"

	"github.com/hbttundar/scg-service-bus/adapters/nats"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
)

func TestNewWithNATS_EmptyURL(t *testing.T) {
	_, _, err := nats.NewWithNATS(nats.Config{})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, berr.ErrPublishFailed) {
		t.Fatalf("want ErrPublishFailed, got %v", err)
	}
}
