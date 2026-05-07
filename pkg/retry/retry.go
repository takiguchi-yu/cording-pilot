// Package retry は指数バックオフによる汎用リトライ機構を提供します。
package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// NonRetryableError はリトライ対象外として即時に終了すべきエラーを表します。
type NonRetryableError struct {
	err error
}

// Error implements error.
func (e *NonRetryableError) Error() string {
	if e.err == nil {
		return "retry: non-retryable error"
	}
	return e.err.Error()
}

// Unwrap implements error.
func (e *NonRetryableError) Unwrap() error {
	return e.err
}

// NonRetryable は err を「リトライ不要」なエラーとしてラップします。
func NonRetryable(err error) error {
	return &NonRetryableError{err: err}
}

// Policy は Do の動作を設定します。
type Policy struct {
	// MaxAttempts は操作を試みる最大回数です。
	// 1 を指定するとリトライなし（初回のみ）になります。
	MaxAttempts int
	// InitialDelay は 2 回目の試行前の待機時間です。
	InitialDelay time.Duration
	// Multiplier は失敗ごとに待機時間を乗算する係数です。
	// 2.0 を指定すると試行ごとに待機時間が 2 倍になります。
	Multiplier float64
}

// DefaultPolicy は合理的なデフォルト設定です。
// 最大 3 回試行、初回遅延 500ms、乗数 2×（500ms → 1s → 2s）。
var DefaultPolicy = Policy{
	MaxAttempts:  3,
	InitialDelay: 500 * time.Millisecond,
	Multiplier:   2.0,
}

// Do は fn を最大 p.MaxAttempts 回呼び出します。fn が nil を返すと即座に成功します。
// 試行間は p.InitialDelay と p.Multiplier から算出した指数的増加時間だけスリープします。
// スリープ中に ctx がキャンセルされた場合、ctx.Err() を最後の fn エラーでラップして返します。
func Do(ctx context.Context, p Policy, fn func() error) error {
	if p.MaxAttempts < 1 {
		return fmt.Errorf("retry: MaxAttempts must be >= 1, got %d", p.MaxAttempts)
	}

	var lastErr error
	delay := p.InitialDelay

	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			var nonRetryable *NonRetryableError
			if errors.As(err, &nonRetryable) {
				if unwrapped := nonRetryable.Unwrap(); unwrapped != nil {
					return unwrapped
				}
				return fmt.Errorf("retry: non-retryable error")
			}
			lastErr = err
		}

		if attempt == p.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("retry: context cancelled after %d attempt(s): %w (last error: %w)",
				attempt, ctx.Err(), lastErr)
		case <-time.After(delay):
		}

		delay = time.Duration(float64(delay) * p.Multiplier)
	}

	return fmt.Errorf("retry: all %d attempt(s) failed: %w", p.MaxAttempts, lastErr)
}
