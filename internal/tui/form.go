// Package tui は CLI 上でインタラクティブなフォームを描画・実行するユーティリティを提供します。
// フォームの描画ロジックはこのパッケージに閉じており、ドメインロジック（Agent や State）には
// huh への依存が漏れ出さないよう責務を分割しています。
package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/takiguchi-yu/cording-pilot/internal/agent"
)

// ErrAborted はユーザーが Ctrl+C または Esc でフォームを中断した場合に返されるエラーです。
var ErrAborted = errors.New("ユーザーによって入力が中断されました")

// RunForm は質問リストから huh フォームを動的に構築してユーザーに提示し、
// 回答を map[questionID]answer の形式で返します。
//
// 質問が空の場合は即座に空のマップを返します。
// ユーザーが Ctrl+C や Esc でフォームを中断した場合は ErrAborted を返します。
func RunForm(questions []agent.Question) (map[string]string, error) {
	if len(questions) == 0 {
		return map[string]string{}, nil
	}

	// 各質問の回答値を保持するポインタを管理します。
	boolAnswers := make(map[string]*bool, len(questions))
	stringAnswers := make(map[string]*string, len(questions))

	fields := make([]huh.Field, 0, len(questions))
	for _, q := range questions {
		switch q.Type {
		case "confirm":
			val := new(bool)
			boolAnswers[q.ID] = val
			fields = append(fields, huh.NewConfirm().
				Title(q.Text).
				Value(val))
		case "select":
			val := new(string)
			stringAnswers[q.ID] = val
			opts := make([]huh.Option[string], len(q.Options))
			for i, o := range q.Options {
				opts[i] = huh.NewOption(o, o)
			}
			fields = append(fields, huh.NewSelect[string]().
				Title(q.Text).
				Options(opts...).
				Value(val))
		default: // "text" またはその他
			val := new(string)
			stringAnswers[q.ID] = val
			fields = append(fields, huh.NewInput().
				Title(q.Text).
				Value(val))
		}
	}

	form := huh.NewForm(huh.NewGroup(fields...))
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, ErrAborted
		}
		return nil, fmt.Errorf("tui: form run: %w", err)
	}

	// 回答を文字列マップに集約します。
	answers := make(map[string]string, len(questions))
	for id, val := range boolAnswers {
		if *val {
			answers[id] = "はい"
		} else {
			answers[id] = "いいえ"
		}
	}
	for id, val := range stringAnswers {
		answers[id] = *val
	}

	return answers, nil
}
