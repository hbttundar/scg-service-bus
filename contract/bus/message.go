package bus

// Command is a marker interface for commands (intent to change state).
// A command should have a single handler.
type Command interface{}

// Query is a marker interface for queries. Queries are handled synchronously and must not change state.
type Query interface{}
