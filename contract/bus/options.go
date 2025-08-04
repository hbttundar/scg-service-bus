package bus

// QueueOptions represents enqueue parameters for commands or listeners.
// DelaySeconds is preferred over time units for transport-agnostic mapping.
type QueueOptions struct {
	Queue        string
	DelaySeconds int
	Headers      map[string]string
}

// PublishOptions controls integration event publishing.
type PublishOptions struct {
	TopicOverride string
	Key           string
	Headers       map[string]string
}
