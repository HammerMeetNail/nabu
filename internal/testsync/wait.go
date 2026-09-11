// Package testsync supplies bounded, condition-based synchronization for tests.
package testsync

import (
	"context"
	"testing"
	"time"
)

func Receive[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-ctx.Done():
		t.Fatal("concurrent operation did not reach the expected barrier before its deadline")
	}
	var zero T
	return zero
}
func Eventually(t *testing.T, ctx context.Context, condition func() bool) {
	t.Helper()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for !condition() {
		select {
		case <-ctx.Done():
			t.Fatal("concurrent operation did not reach the expected state before its deadline")
		case <-tick.C:
		}
	}
}
func Pause(ctx context.Context, resume <-chan struct{}) {
	select {
	case <-resume:
	case <-ctx.Done():
	}
}
