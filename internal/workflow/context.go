// Package workflow はオーケストレーションのステートマシンを実装します。
// パイプラインの各フェーズ（Plan、Implement、Review、Complete）は、次に遷移する State を知る
// 具象的な State として表現されます。
package workflow

import "github.com/takiguchi-yu/cording-pilot/internal/config"

// Context は State 間で持ち回す全ての可変データを保持します。
// Redux の Store に相当する役割を杉たします。
type Context struct {
	// Requirement は元の自然言語によるタスク記述です。
	Requirement string
	// PlanText は Planner エージェントが生成した実装計画を保持します。
	PlanText string
	// WorkDir は隔離ビルド環境として使用する一時ディレクトリです。
	WorkDir string
	// LastTestOutput は直近のテスト実行で取得した stdout/stderr です。
	LastTestOutput string
	// TryCount はフィックスループの実行済み回数です。
	TryCount int
	// ReviewFeedback は Reviewer エージェントからの最新のレビュー結果です。
	ReviewFeedback string
	// Config は .cording-pilot.yml から読み込んだプロジェクト設定です。
	Config *config.Config
	// IssueNumber は対応する GitHub Issue 番号です。0 の場合は未設定を示します。
	IssueNumber int
	// BranchName は生成した作業ブランチ名です。Complete フェーズで設定されます。
	BranchName string
	// DeterministicFallbackUsed は Implement フェーズで deterministic fallback を適用したかを示します。
	DeterministicFallbackUsed bool
}
