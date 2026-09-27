// Package timing carries the moment an inbound webhook was accepted through
// the context, so an unrelated layer can measure how long handling took without
// every signature in between growing a timestamp parameter.
package timing

import (
	"context"
	"time"
)

type contextKey struct{}

// WithReceived returns a context stamped with the acceptance time.
func WithReceived(ctx context.Context, at time.Time) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, at)
}

// Received reports the acceptance time carried by ctx, if any.
func Received(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	at, ok := ctx.Value(contextKey{}).(time.Time)
	return at, ok
}
