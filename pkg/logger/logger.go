// Package logger はオーケストレーション実行中に発生するイベントを記録するための
// NDJSON 形式の構造化ロガーを提供します。
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Level はログエントリの重要度を表します。
type Level string

const (
	// LevelInfo は通常の情報イベントに使用します。
	LevelInfo Level = "INFO"
	// LevelWarn は回復可能な異常に使用します。
	LevelWarn Level = "WARN"
	// LevelError はエラー状態に使用します。
	LevelError Level = "ERROR"
)

// Entry は NDJSON ログストリームにおける 1 行分の JSON オブジェクトです。
type Entry struct {
	Timestamp string `json:"timestamp"`
	Level     Level  `json:"level"`
	Event     string `json:"event"`
	Message   string `json:"message"`
}

// Logger は基底の io.Writer に対して 1 行につき 1 エントリ（NDJSON 形式）を書き込みます。
type Logger struct {
	enc *json.Encoder
}

// New は w に書き込む Logger を生成します。
// '<'、'>'、'&' がエスケープされないよう HTML エスケープを無効化しています。
func New(w io.Writer) *Logger {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &Logger{enc: enc}
}

// Log は指定した重要度でエントリを記録します。
func (l *Logger) Log(level Level, event, message string) error {
	e := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Event:     event,
		Message:   message,
	}
	if err := l.enc.Encode(e); err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	return nil
}

// Info は INFO レベルのエントリを記録します。
func (l *Logger) Info(event, message string) error {
	return l.Log(LevelInfo, event, message)
}

// Warn は WARN レベルのエントリを記録します。
func (l *Logger) Warn(event, message string) error {
	return l.Log(LevelWarn, event, message)
}

// Error は ERROR レベルのエントリを記録します。
func (l *Logger) Error(event, message string) error {
	return l.Log(LevelError, event, message)
}
