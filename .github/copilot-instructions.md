# cording-pilot — AI Agent Orchestrator CLI

## Role & Project Overview

あなたはシニアGoエンジニアであり、AIオーケストレーションの専門家です。
このプロジェクト `cording-pilot` は、Go製のAIエージェントオーケストレーターCLIです。
LLMを用いた自律的な要件定義、TDDスタイルのコード生成、およびコンテナ環境での静的解析・テスト実行ループ（Fix Loop）を管理します。

## Build & Test

```bash
make build      # ビルドチェック
make test       # テスト実行
make lint       # lintチェック（警告ゼロを維持すること）
make run        # 動作確認
```

## Architecture & Directory Layout

Standard Go Project Layout およびクリーンアーキテクチャに準拠します。
`internal/`（ドメインロジック）と `pkg/`（汎用ライブラリ）の境界を厳格に守ってください。

```text
cmd/orchestrator/       # エントリーポイント。フラグ解析、DIコンテナによる依存解決とワイヤリング
internal/               # ドメイン依存ロジック（外部プロジェクトからのimport禁止）
  agent/                # [Factory] ペルソナ（Planner/Coder/Reviewer）のシステムプロンプトと生成
  executor/             # [Strategy] コマンド実行環境（Local / Docker Worktree）
  llm/                  # [Strategy] LLM APIクライアント（OpenAI / Anthropic 等）
  workflow/             # [State] 状態遷移（Context, Plan, Implement, Review, Complete）
pkg/                    # ドメイン非依存の汎用ライブラリ（他プロジェクトでも再利用可能に保つ）
  logger/               # NDJSON形式の構造化ロガー（トレーサビリティ担保）
  markdown/             # LLM出力からのMarkdownコードブロック抽出パーサー
  retry/                # 指数バックオフ（Exponential Backoff）によるリトライ機構
```

## Core Workflow (State Machine)

処理は以下のステートマシンとして進行します。各フェーズの実装を逸脱しないでください。

1. **Interactive/Issue Phase**: Issueが存在しない場合、Planner Agentがユーザーと対話し要件を明確化してからIssue化する。
2. **Plan Phase**: Issueから実装計画と影響範囲を定義。
3. **Implement Phase (TDD & Quality Check Fix Loop)**:
   - 先にテストコード(`*_test.go`)を生成。
   - プロダクトコード(`*.go`)を生成。
   - `Executor` (Docker等隔離環境) で以下の品質チェックパイプラインを順に実行する：
     1. **Format**: `go fmt ./...` および `goimports -w .` を実行。
     2. **Type Check**: `go build ./...` でコンパイル・型チェックを実行。
     3. **Lint**: `golangci-lint run` で静的解析を実行。
     4. **Test**: `go test -v ./...` を実行。
   - 上記の**いずれかのステップでエラーが出た場合（Red）**、そのエラー出力をCoder Agentに渡し、コードを修正させる（すべてGreenになるまで最大N回ループ）。
4. **Review Phase**: Reviewer Agentが要件と差分を突合。アーキテクチャ上の懸念があればImplementへ差し戻し。
5. **Complete Phase**: 変更をコミット/PR作成し、NDJSONログをフラッシュ。

## Design Principles

- **State Pattern**: `internal/workflow` の各フェーズは `State` インターフェース (`Execute(ctx, *Context) (State, error)`) を実装し、巨大な `switch` 文での分岐を避ける。
- **Strategy Pattern**: `llm.Client` と `executor.Executor` はインターフェースで抽象化。`go fmt` や `golangci-lint` 等の複数コマンド実行も `Executor` 経由で行う。具象型は `cmd/orchestrator/main.go` でDIする。
- **Factory Method**: `agent.Factory` が各AIエージェントのプロンプトと振る舞いをカプセル化する。

## Go Coding Conventions (厳守)

### 1. エラーハンドリングと型安全

- `_ = err` によるエラーの握りつぶしは**絶対に行わない**こと。
- エラーは必ず上位にラップして返す。 `fmt.Errorf("component: operation: %w", err)`
- 終端処理（`main`等）以外で `log.Fatal` や `panic` を使用しない。
- `interface{}` や `any` の乱用を避け、厳格な静的型付けを維持する。

### 2. Contextの伝播

- 外部通信（LLM API）や外部プロセス実行（Executor）を伴う、またはブロックする可能性のある全ての関数の第一引数には `ctx context.Context` を渡し、タイムアウトとキャンセル処理を実装すること。

### 3. インターフェース設計

- 小さなインターフェースを好む（Goのことわざ: "The bigger the interface, the weaker the abstraction"）。
- 1〜2つのメソッドのみを持つシンプルなインターフェースを基本とする。

### 4. ロギング

- `fmt.Println` や標準の `log` パッケージによるアドホックな出力は避け、`pkg/logger` のNDJSONロガーを使用して構造化ログを出力する。

### 5. GoDocとコードスタイル

- 全ての exported シンボル（型、インターフェース、関数、定数）に GoDoc コメントを付与すること。
- 変数名・関数名は Effective Go に従う（`camelCase`、略語は `Id` ではなく `ID`、`Url` ではなく `URL` 等）。
- `gofmt` および `goimports` 適用済みの状態を出力すること。

## AI Assistant Rules (メタ指示)

- 提案するコードは、常に上記ディレクトリ構造と依存関係ルールに従うこと。`internal` から `pkg` への依存は可、逆は不可。
- 既存のコメントやGoDocを勝手に削除しないこと。
- テストコード（`_test.go`）の生成を求められた場合は、標準の `testing` パッケージを使用し、Table Driven Testsの形式で書くこと。
- Implement Phaseにおける `Executor` の呼び出しは、Format -> Type Check -> Lint -> Test のパイプラインを必ず実装に含めること。
