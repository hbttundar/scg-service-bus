package bus

import "context"

// Context is re-exported for convenience in handler signatures.
// It avoids importing context in user packages when referencing bus types.
type Context = context.Context
