package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/takiguchi-yu/cording-pilot/pkg/retry"
)

var errFake = errors.New("fake error")

func TestDo_初回試行で成功する(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3, InitialDelay: 0, Multiplier: 1}, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls=%d; want 1", calls)
	}
}

func TestDo_2回目の試行で成功する(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3, InitialDelay: 0, Multiplier: 1}, func() error {
		calls++
		if calls < 2 {
			return errFake
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls=%d; want 2", calls)
	}
}

func TestDo_全試行を使い切る(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3, InitialDelay: 0, Multiplier: 1}, func() error {
		calls++
		return errFake
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errFake) {
		t.Errorf("error should wrap errFake; got: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls=%d; want 3", calls)
	}
}

func TestDo_無効なMaxAttempts(t *testing.T) {
	t.Parallel()
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 0, InitialDelay: 0, Multiplier: 1}, func() error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for MaxAttempts=0")
	}
}

func TestDo_スリープ中にコンテキストがキャンセルされる(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	calls := 0
	// Use a long delay so the context is guaranteed to expire before the retry.
	err := retry.Do(ctx, retry.Policy{MaxAttempts: 5, InitialDelay: 5 * time.Second, Multiplier: 1}, func() error {
		calls++
		return errFake
	})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error should wrap context.DeadlineExceeded; got: %v", err)
	}
	// Only 1 call before sleeping.
	if calls != 1 {
		t.Errorf("calls=%d; want 1", calls)
	}
}

func TestDo_1回のみ試行してリトライしない(t *testing.T) {
	t.Parallel()
	calls := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 1, InitialDelay: 0, Multiplier: 1}, func() error {
		calls++
		return errFake
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls=%d; want 1", calls)
	}
}
