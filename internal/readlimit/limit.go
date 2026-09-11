// Package readlimit bounds export collection reads before they allocate an
// unbounded result. Exceeding a limit is an error, never a truncated success.
package readlimit

import (
	"context"
	"errors"
	"strconv"
)

var ErrExceeded = errors.New("export exceeds the row or size limit; choose a smaller date range")

type limitKey struct{}

func With(ctx context.Context, rows int) context.Context {
	return context.WithValue(ctx, limitKey{}, max(0, rows))
}
func Remaining(ctx context.Context, used int) context.Context {
	if limit, ok := ctx.Value(limitKey{}).(int); ok {
		return With(ctx, limit-used)
	}
	return ctx
}
func Check(ctx context.Context, rows int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if limit, ok := ctx.Value(limitKey{}).(int); ok && rows > limit {
		return ErrExceeded
	}
	return nil
}

// SQL is composed only from a server-owned integer. One extra row distinguishes
// a complete result from an oversized one; callers must Check before returning.
func SQL(ctx context.Context) string {
	if limit, ok := ctx.Value(limitKey{}).(int); ok {
		return " LIMIT " + strconv.Itoa(limit+1)
	}
	return ""
}
