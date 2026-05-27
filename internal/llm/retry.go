package llm

import (
	"context"
	"errors"
	"time"
)

// maxAttempts caps total tries (initial call plus retries).
const maxAttempts = 3

// backoffSchedule is the delay before attempt i+1 after attempt i fails:
// 2s, then 4s, then 8s. Indexes beyond the slice clamp to the last value.
var backoffSchedule = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

// Retry runs fn with the package retry policy: up to maxAttempts tries, backing
// off 2s -> 4s -> 8s, retrying only errors that match ErrTransient. It is the
// transport-level helper used by Client implementations.
func Retry(ctx context.Context, fn func() error) error {
	return retry(ctx, maxAttempts, sleepCtx, fn)
}

// retry is the testable core of Retry with the attempt count and sleep injected.
func retry(ctx context.Context, attempts int, sleep func(context.Context, time.Duration) error, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrTransient) {
			return err
		}
		if i == attempts-1 {
			break
		}
		if serr := sleep(ctx, backoff(i)); serr != nil {
			return serr
		}
	}
	return err
}

func backoff(i int) time.Duration {
	if i >= len(backoffSchedule) {
		i = len(backoffSchedule) - 1
	}
	return backoffSchedule[i]
}

// sleepCtx waits for d or until ctx is cancelled, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
