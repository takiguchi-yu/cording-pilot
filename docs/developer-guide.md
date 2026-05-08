# 開発者向けガイド

このドキュメントは、`cording-pilot` を開発・保守するための実践ガイドです。

利用手順は [user-guide.md](./user-guide.md) を参照してください。
エージェント間の流れは [agent-sequence-diagram.mmd](./agent-sequence-diagram.mmd) を参照してください。

## このプロジェクトで大事にしていること

- ワークフローを `Interactive -> Plan -> Implement -> Review -> Complete` の状態遷移で明示的に管理する
- Implement フェーズで品質チェックを自動実行し、失敗時は Fix Loop で修復する
- `internal/` と `pkg/` の責務境界を保ち、設計の見通しを維持する

## 開発環境

- Go 1.26.2
- `make`
- `golangci-lint`
- `goimports`（`pipeline` で使う場合）

## 開発クイックスタート

```bash
git clone https://github.com/takiguchi-yu/cording-pilot.git
cd cording-pilot

make build
make test
make lint
```

実行確認:

```bash
make run
```

## 主要コマンド（開発時）

| コマンド            | 用途                                       |
| ------------------- | ------------------------------------------ |
| `make build`        | 全パッケージをビルドして型・依存関係を確認 |
| `make test`         | ユニットテストを実行                       |
| `make lint`         | `golangci-lint` による静的解析             |
| `make run`          | サンプル要件でオーケストレーターを起動     |
| `make ollama-serve` | Ollama サーバーをバックグラウンド起動      |
| `make ollama-pull`  | 推奨 Ollama モデルを取得                   |

### Ollama が重い場合の Tips

- 既定の推奨モデルは `qwen3-coder-next:q4_K_M`（4bit 量子化）です。
- それでも重い場合は、`OLLAMA_MODEL` をより小さいモデルに上書きしてください（例: `3b`）。

```bash
OLLAMA_MODEL=qwen3:3b make ollama-pull
```

## アーキテクチャ概要

| ディレクトリ        | 役割                                               |
| ------------------- | -------------------------------------------------- |
| `cmd/orchestrator`  | エントリーポイント、CLI フラグ解析、依存関係の配線 |
| `internal/agent`    | Planner/Coder/Reviewer の生成と役割プロンプト      |
| `internal/executor` | `local` / `docker` / `nix` 実行戦略                |
| `internal/llm`      | LLM クライアント抽象とプロバイダー実装             |
| `internal/workflow` | ステートマシン本体（状態遷移と実行制御）           |
| `pkg/logger`        | NDJSON 形式の構造化ログ                            |
| `pkg/retry`         | 指数バックオフ再試行                               |

## ワークフロー実装の要点

### 状態遷移

1. `Interactive`: Issue が未指定なら対話で要件整理
2. `Plan`: 実装計画を作成
3. `Implement`: テスト生成 -> 実装生成 -> パイプライン実行（失敗時は最大 3 回の Fix Loop）
4. `Review`: 要件と差分を突合して承認/差し戻し
5. `Complete`: 結果の反映、必要に応じて GitHub 連携

### Implement フェーズの品質パイプライン

`internal/config` のデフォルトでは次の順で実行されます。

1. `goimports -w .`
2. `go fmt ./...`
3. `go build ./...`
4. `golangci-lint run`
5. `go test -v ./...`

## 設定ファイル（.cording-pilot.yml）

デフォルトファイル名は `.cording-pilot.yml` です。

### 検証ルール

- YAML は `KnownFields(true)` で厳格にデコード（未知キーはエラー）
- 複数 YAML ドキュメント（`---` 区切り）は非対応
- `version` は `"1.0"` 固定
- `llm.provider` は `copilot` または `ollama`
- `environment.type` は `local` / `docker` / `nix`

### 主な設定項目

- `llm`: provider、モデル、ロール別モデル、リトライ、レート制御
- `environment`: 実行環境種別と Docker イメージ
- `auto_fix`: パイプライン前の修復コマンド列（失敗しても続行）
- `pipeline`: 品質ゲートとして順序実行されるコマンド列

### CLI フラグによる上書き

- `--docker` / `--nix`: `environment.type` を上書き
- `--docker-image`: `environment.image` を上書き
- `--config`: 読み込む設定ファイルを差し替え

## LLM プロバイダー開発メモ

- `copilot` を使う場合は `GITHUB_TOKEN` 必須
- `ollama` は `llm.base_url` 未指定時に `http://localhost:11434/v1` を使用
- モデルは `planner_*`, `coder_model`, `reviewer_model` に分離可能

## 変更時チェックリスト

1. 変更前に `make test` と `make lint` を実行してベースライン確認
2. 実装後に `make build` / `make test` / `make lint` を再実行
3. `internal` と `pkg` の依存境界が崩れていないことを確認
4. 既存 GoDoc・コメントを不必要に削除していないことを確認
5. ドキュメント影響がある場合は `docs/` を同時更新

## 関連ドキュメント

- 利用手順: [user-guide.md](./user-guide.md)
- 概要: [../README.md](../README.md)
- エージェントシーケンス図: [agent-sequence-diagram.mmd](./agent-sequence-diagram.mmd)
