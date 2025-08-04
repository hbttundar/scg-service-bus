package errors

// Error codes for the bus contracts. Keep stable; used across adapters and bus.
const (
	ErrCodeHandlerExists       = "servicebus.handler_exists"
	ErrCodeHandlerNotFound     = "servicebus.handler_not_found"
	ErrCodeHandlerTypeMismatch = "servicebus.handler_type_mismatch"
	ErrCodeAsyncNotConfigured  = "servicebus.async_not_configured"
	ErrCodeEnqueueFailed       = "servicebus.enqueue_failed"
	ErrCodePublishFailed       = "servicebus.publish_failed"
	ErrCodeDelayUnsupported    = "servicebus.delay_unsupported"
	ErrCodeSerializationFailed = "servicebus.serialization_failed"
)

// Code returns an error value that carries only a code string.
// It implements error by returning the code string in Error().
func Code(code string) error { return codedError(code) }

type codedError string

func (e codedError) Error() string { return string(e) }

var (
	ErrHandlerExists       = Code(ErrCodeHandlerExists)
	ErrHandlerNotFound     = Code(ErrCodeHandlerNotFound)
	ErrHandlerTypeMismatch = Code(ErrCodeHandlerTypeMismatch)
	ErrAsyncNotConfigured  = Code(ErrCodeAsyncNotConfigured)
	ErrEnqueueFailed       = Code(ErrCodeEnqueueFailed)
	ErrPublishFailed       = Code(ErrCodePublishFailed)
	ErrDelayUnsupported    = Code(ErrCodeDelayUnsupported)
	ErrSerializationFailed = Code(ErrCodeSerializationFailed)
)
