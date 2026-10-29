package auth

import (
	"context"
	"log/slog"
	"time"
)

// Ended sessions and spent sign-in tokens are swept a minute after start and
// then every sweepInterval, in deletes of at most sweepBatch rows each.
const (
	sweepDelay    = time.Minute
	sweepInterval = time.Hour
	sweepBatch    = 500
)

// sweepLockKey is the transaction-scoped advisory lock that lets one
// instance sweep at a time ("nabu_swp").
const sweepLockKey int64 = 0x6e6162755f737770

// sweeper deletes up to limit ended sessions and up to limit spent auth
// tokens, and reports how many of each it deleted.
type sweeper interface {
	SweepEnded(ctx context.Context, limit int) (sessions, tokens int64, err error)
}

// RunSweeps deletes ended sessions, with their devices' push registrations,
// and spent sign-in tokens until ctx ends. A session has ended once it
// expires or goes the idle timeout without a request, and a token is spent
// once it is used or expires. Neither can be used again, so no client sees a
// difference; the sweep only stops them piling up.
func (s *Service) RunSweeps(ctx context.Context) {
	sw, ok := s.store.(sweeper)
	if !ok {
		return
	}
	timer := time.NewTimer(sweepDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		sweep(ctx, sw)
		timer.Reset(sweepInterval)
	}
}

// sweep deletes in batches until a batch comes back short.
func sweep(ctx context.Context, sw sweeper) (sessions, tokens int64) {
	for ctx.Err() == nil {
		n, m, err := sw.SweepEnded(ctx, sweepBatch)
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("session sweep failed", "error", err)
			}
			break
		}
		sessions, tokens = sessions+n, tokens+m
		if n < sweepBatch && m < sweepBatch {
			break
		}
	}
	if sessions > 0 || tokens > 0 {
		slog.Info("swept ended sessions and spent sign-in tokens", "sessions", sessions, "tokens", tokens)
	}
	return sessions, tokens
}
