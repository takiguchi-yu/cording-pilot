# 開発者向けガイド

このドキュメントは、`cording-pilot` の開発・保守を行う人向けのガイドです。

## 対象読者

- このリポジトリに機能追加・修正を行う開発者
- 実装前後の品質チェックを回したい開発者

## 開発環境

- Go 1.26.2
- make

## 開発時の基本コマンド

```bash
make build
make test
make lint
```

動作確認:

```bash
make run
```

## アーキテクチャ概要

- cmd/orchestrator: エントリーポイント（依存関係の配線）
- internal/agent: Planner/Coder/Reviewer の生成と役割定義
- internal/executor: 実行戦略（local/docker/nix）
- internal/llm: LLM クライアント抽象
- internal/workflow: ステートマシン本体
- pkg/logger: NDJSON ロガー
- pkg/retry: リトライユーティリティ

詳細な設計方針は、リポジトリの Copilot 指示ファイルも参照してください。

## ワークフロー

処理は以下の状態遷移で進みます。

1. Interactive（Issue が無い場合に要件整理）
2. Plan
3. Implement
4. Review
5. Complete

Implement では Executor 経由で品質チェックパイプラインを実行します。

## 設定ファイル

デフォルト設定ファイル名は `.cording-pilot.yml` です。

主な設定項目:

- `version`
- `llm.provider`, `llm.model`, `llm.auto_fix_model`
- `environment.type`, `environment.image`
- `auto_fix`
- `pipeline`（実行コマンド列）

設定の実装仕様（internal/config）:

- YAML デコードは `KnownFields(true)` で厳格化（未知キーはエラー）
- 複数 YAML ドキュメントは非対応（単一ドキュメントのみ）
- `version` は `"1.0"` を必須化（省略時は補完）
- `llm.provider` は `"copilot"` のみ許可
- `llm.model` のデフォルトは `"gpt-4.1"`
- `llm.auto_fix_model` 省略時は `llm.model` を利用

CLI フラグは一部設定を上書きします（例: `--docker`, `--nix`, `--docker-image`）。

## 実装時の注意

- 変更前に `make test` と `make lint` を実行して現状確認
- 変更後に `make build`, `make test`, `make lint` を実行
- 既存の責務分割（internal と pkg の境界）を維持
- 既存コメントや GoDoc を不要に削除しない

## ドキュメントの位置づけ

- 利用方法は `docs/user-guide.md`
- このファイルは開発フローと保守観点に限定
