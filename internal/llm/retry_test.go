package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// noSleep is an injected sleep that records the requested delays without waiting.
type recorder struct{ delays []time.Duration }

func (r *recorder) sleep(_ context.Context, d time.Duration) error {
	r.delays = append(r.delays, d)
	return nil
}

func TestRetryTransientThenSuccess(t *testing.T) {
	rec := &recorder{}
	calls := 0
	err := retry(context.Background(), 3, rec.sleep, func() error {
		calls++
		if calls < 2 {
			return errors.Join(ErrTransient, errors.New("boom"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if len(rec.delays) != 1 || rec.delays[0] != 2*time.Second {
		t.Fatalf("delays = %v, want [2s]", rec.delays)
	}
}

func TestRetryPermanentNoRetry(t *testing.T) {
	rec := &recorder{}
	permanent := errors.New("4xx bad request")
	calls := 0
	err := retry(context.Background(), 3, rec.sleep, func() error {
		calls++
		return permanent
	})
	if !errors.Is(err, permanent) {
		t.Fatalf("err = %v, want permanent", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on permanent)", calls)
	}
	if len(rec.delays) != 0 {
		t.Fatalf("delays = %v, want none", rec.delays)
	}
}

func TestRetryExhaustsAttempts(t *testing.T) {
	rec := &recorder{}
	calls := 0
	err := retry(context.Background(), 3, rec.sleep, func() error {
		calls++
		return errors.Join(ErrTransient, errors.New("boom"))
	})
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second}
	if len(rec.delays) != len(want) || rec.delays[0] != want[0] || rec.delays[1] != want[1] {
		t.Fatalf("delays = %v, want %v", rec.delays, want)
	}
}

func TestRetryContextCancelledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled, so sleepCtx returns immediately

	calls := 0
	err := retry(ctx, 3, sleepCtx, func() error {
		calls++
		return errors.Join(ErrTransient, errors.New("boom"))
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (cancelled before second attempt)", calls)
	}
}
