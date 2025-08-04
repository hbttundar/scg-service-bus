package errors_test

import (
	"errors"
	"testing"

	berr "github.com/hbttundar/scg-service-bus/contract/errors"
)

func TestCodeAndVars(t *testing.T) {
	e := berr.Code(berr.ErrCodePublishFailed)
	if e.Error() != berr.ErrCodePublishFailed {
		t.Fatalf("unexpected error string: %s", e.Error())
	}

	// exported variables must carry their codes
	tests := []struct {
		err  error
		code string
	}{
		{berr.ErrHandlerExists, berr.ErrCodeHandlerExists},
		{berr.ErrHandlerNotFound, berr.ErrCodeHandlerNotFound},
		{berr.ErrHandlerTypeMismatch, berr.ErrCodeHandlerTypeMismatch},
		{berr.ErrAsyncNotConfigured, berr.ErrCodeAsyncNotConfigured},
		{berr.ErrEnqueueFailed, berr.ErrCodeEnqueueFailed},
		{berr.ErrPublishFailed, berr.ErrCodePublishFailed},
		{berr.ErrDelayUnsupported, berr.ErrCodeDelayUnsupported},
		{berr.ErrSerializationFailed, berr.ErrCodeSerializationFailed},
	}

	for _, tc := range tests {
		if !errors.Is(tc.err, berr.Code(tc.code)) {
			t.Fatalf("expected %s to be %s", tc.err, tc.code)
		}
	}
}
