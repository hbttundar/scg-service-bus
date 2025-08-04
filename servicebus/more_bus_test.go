package servicebus_test

import (
	"context"
	"errors"
	"testing"

	cbus "github.com/hbttundar/scg-service-bus/contract/bus"
	berr "github.com/hbttundar/scg-service-bus/contract/errors"
	"github.com/hbttundar/scg-service-bus/servicebus"
)

type gCmd struct{ ID string }

type gQry struct{ K string }

type gRes struct{ V string }

type gCmdHandler struct{ seen *[]string }

func (h gCmdHandler) Handle(ctx context.Context, c gCmd) error {
	*h.seen = append(*h.seen, c.ID)
	return nil
}

type gQryHandler struct{}

func (gQryHandler) Handle(ctx context.Context, q gQry) (gRes, error) { return gRes{V: q.K}, nil }

type badCmd struct{ X int }

func Test_GenericBindAndDispatch(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	// Bind generic command handler
	var seen []string
	if err := servicebus.BindCommand[gCmd](b, gCmdHandler{seen: &seen}); err != nil {
		t.Fatalf("bind cmd: %v", err)
	}

	// Duplicate should error
	if err := servicebus.BindCommand[gCmd](b, gCmdHandler{seen: &seen}); !errors.Is(err, berr.ErrHandlerExists) {
		t.Fatalf("want ErrHandlerExists, got %v", err)
	}

	// Dispatch happy path
	if err := b.DispatchSync(t.Context(), gCmd{ID: "x"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(seen) != 1 || seen[0] != "x" {
		t.Fatalf("seen=%v", seen)
	}

	// Bind generic query
	if err := servicebus.BindQuery[gQry, gRes](b, gQryHandler{}); err != nil {
		t.Fatalf("bind query: %v", err)
	}
	// Ask generic happy path
	r, err := servicebus.Ask[gQry, gRes](t.Context(), b, gQry{K: "k"})
	if err != nil || r.V != "k" {
		t.Fatalf("ask: %v r=%+v", err, r)
	}

	// Type mismatch on dispatch via generics path
	// Use internal dispatchWithMiddleware by calling DispatchSync with a bad type bound under generics
	// First, register a handler that expects gCmd; then pass wrong type
	err = b.DispatchSync(t.Context(), badCmd{X: 1})
	// no handler for badCmd
	if !errors.Is(err, berr.ErrHandlerNotFound) {
		t.Fatalf("want ErrHandlerNotFound, got %v", err)
	}
}

func Test_DispatchWithMiddleware_OrderAndWrapping(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	_ = b.BindCommandOf(gCmd{}, func(ctx context.Context, v any) error { return nil })

	calls := []string{}
	mw1 := func(next func(ctx context.Context, cmd any) error) func(ctx context.Context, cmd any) error {
		return func(ctx context.Context, cmd any) error {
			calls = append(calls, "mw1-before")
			err := next(ctx, cmd)

			calls = append(calls, "mw1-after")

			return err
		}
	}
	mw2 := func(next func(ctx context.Context, cmd any) error) func(ctx context.Context, cmd any) error {
		return func(ctx context.Context, cmd any) error {
			calls = append(calls, "mw2-before")
			err := next(ctx, cmd)

			calls = append(calls, "mw2-after")

			return err
		}
	}

	// Global registration order matters
	opt := servicebus.WithCommandMiddleware(mw1, mw2)
	opt(b)

	if err := b.DispatchWithMiddleware(t.Context(), gCmd{ID: "1"}); err != nil {
		t.Fatalf("dispatch with mw: %v", err)
	}

	want := []string{"mw1-before", "mw2-before", "mw2-after", "mw1-after"}
	if len(calls) != len(want) {
		t.Fatalf("calls len=%d want=%d", len(calls), len(want))
	}

	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("order mismatch at %d: %s != %s", i, calls[i], want[i])
		}
	}

	// Alias DispatchNow should work
	if err := b.DispatchNow(t.Context(), gCmd{ID: "2"}); err != nil {
		t.Fatalf("dispatch now: %v", err)
	}
}

func Test_Chain_StopsOnFirstError(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	var i int

	_ = b.BindCommandOf(gCmd{}, func(ctx context.Context, v any) error {
		i++
		if i == 2 {
			return errors.New("boom")
		}

		return nil
	})

	err := b.Chain(t.Context(), gCmd{ID: "1"}, gCmd{ID: "2"}, gCmd{ID: "3"})
	if err == nil {
		t.Fatalf("expected error")
	}

	if i != 2 { // third should not run
		t.Fatalf("ran %d handlers, want 2", i)
	}
}

func Test_Batch_Progress_Error_AndCancel(t *testing.T) {
	b := servicebus.New(nil, nil, nil)

	// Handler errors on specific ID
	_ = b.BindCommandOf(gCmd{}, func(ctx context.Context, v any) error {
		id := v.(gCmd).ID
		if id == "bad" {
			return errors.New("bad")
		}
		// Simulate minimal work
		return nil
	})

	var prog []int

	var errs []string

	opts := []servicebus.BatchOpt{
		servicebus.WithBatchProgress(func(done, total int) { prog = append(prog, done) }),
		servicebus.WithBatchOnError(func(index int, cmd cbus.Command, err error) {
			errs = append(errs, cmd.(gCmd).ID)
		}),
	}

	cmds := []cbus.Command{gCmd{ID: "a"}, gCmd{ID: "bad"}, gCmd{ID: "b"}}
	err := b.Batch(t.Context(), cmds, opts...)

	if err == nil {
		t.Fatalf("expected aggregated error")
	}

	// Progress should be 1,2,3
	if len(prog) != 3 || prog[0] != 1 || prog[2] != 3 {
		t.Fatalf("progress=%v", prog)
	}
	// OnError should capture the "bad" command
	if len(errs) != 1 || errs[0] != "bad" {
		t.Fatalf("errs=%v", errs)
	}

	// Now test cancel before loop starts
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = b.Batch(ctx, []cbus.Command{gCmd{ID: "x"}})

	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled joined, got %v", err)
	}
}
