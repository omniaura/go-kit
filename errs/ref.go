package errs

import (
	"context"

	"github.com/rs/xid"
	"github.com/rs/zerolog/hlog"
)

// A ref is the one identifier the user, the team and an agent can all hold for
// the same failure. By default it is the request id zerolog's hlog puts on
// every log line of the request (and in the Request-Id response header when
// hlog.RequestIDHandler is configured that way), so a ref copied out of a
// client finds the logs and the recorded row. Work outside a request mints one
// with WithNewRef.

type refKey struct{}

// WithRef attaches ref to ctx; errors made from ctx report under it.
func WithRef(ctx context.Context, ref string) context.Context {
	return context.WithValue(ctx, refKey{}, ref)
}

// WithNewRef attaches a fresh ref to ctx unless it already has one, and returns
// the ref in effect.
func WithNewRef(ctx context.Context) (context.Context, string) {
	if ref := RefFrom(ctx); ref != "" {
		return ctx, ref
	}
	ref := xid.New().String()
	return WithRef(ctx, ref), ref
}

// RefFrom returns the ref for ctx, or "" when there is none.
func RefFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if ref, ok := ctx.Value(refKey{}).(string); ok && ref != "" {
		return ref
	}
	return current().refFunc(ctx)
}

func defaultRef(ctx context.Context) string {
	if id, ok := hlog.IDFromCtx(ctx); ok {
		return id.String()
	}
	return ""
}

// resolveRef is RefFrom, minting a ref when there is none so every reported
// error can be looked up.
func resolveRef(ctx context.Context) string {
	if ref := RefFrom(ctx); ref != "" {
		return ref
	}
	return xid.New().String()
}
