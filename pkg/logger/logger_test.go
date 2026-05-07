package logger_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/takiguchi-yu/cording-pilot/pkg/logger"
)

// decodeEntries はバッファ b 内の全 NDJSON 行を logger.Entry のスライスに読み込みます。
func decodeEntries(t *testing.T, b *bytes.Buffer) []logger.Entry {
	t.Helper()
	var entries []logger.Entry
	dec := json.NewDecoder(b)
	for dec.More() {
		var e logger.Entry
		if err := dec.Decode(&e); err != nil {
			t.Fatalf("decode entry: %v", err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestLogger_Infoレベルのログを出力する(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := logger.New(&buf)

	if err := l.Info("test.event", "hello info"); err != nil {
		t.Fatalf("Info returned error: %v", err)
	}

	entries := decodeEntries(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Level != logger.LevelInfo {
		t.Errorf("level=%q; want %q", e.Level, logger.LevelInfo)
	}
	if e.Event != "test.event" {
		t.Errorf("event=%q; want %q", e.Event, "test.event")
	}
	if e.Message != "hello info" {
		t.Errorf("message=%q; want %q", e.Message, "hello info")
	}
	if e.Timestamp == "" {
		t.Error("timestamp must not be empty")
	}
}

func TestLogger_Warnレベルのログを出力する(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := logger.New(&buf)

	if err := l.Warn("test.warn", "beware"); err != nil {
		t.Fatalf("Warn returned error: %v", err)
	}

	entries := decodeEntries(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != logger.LevelWarn {
		t.Errorf("level=%q; want %q", entries[0].Level, logger.LevelWarn)
	}
}

func TestLogger_Errorレベルのログを出力する(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := logger.New(&buf)

	if err := l.Error("test.error", "something failed"); err != nil {
		t.Fatalf("Error returned error: %v", err)
	}

	entries := decodeEntries(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != logger.LevelError {
		t.Errorf("level=%q; want %q", entries[0].Level, logger.LevelError)
	}
}

func TestLogger_複数エントリを連続記録する(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := logger.New(&buf)

	events := []string{"a", "b", "c"}
	for _, ev := range events {
		if err := l.Info(ev, "msg"); err != nil {
			t.Fatalf("Info(%q): %v", ev, err)
		}
	}

	entries := decodeEntries(t, &buf)
	if len(entries) != len(events) {
		t.Fatalf("expected %d entries, got %d", len(events), len(entries))
	}
	for i, e := range entries {
		if e.Event != events[i] {
			t.Errorf("[%d] event=%q; want %q", i, e.Event, events[i])
		}
	}
}

func TestLogger_HTMLエスケープが無効(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := logger.New(&buf)

	if err := l.Info("ev", "<b>bold</b> & more"); err != nil {
		t.Fatalf("Info: %v", err)
	}

	raw := buf.String()
	if strings.Contains(raw, `\u003c`) || strings.Contains(raw, `\u0026`) {
		t.Errorf("HTML escaping should be disabled, got: %s", raw)
	}
}
