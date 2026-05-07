# cording-pilot

Go製のAIエージェントオーケストレーターCLIです。  
要件整理、実装計画、実装、レビューの流れをステートマシンで管理し、品質チェックまで自動化することを目的としています。

[![CI](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/ci.yml/badge.svg)](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/ci.yml)

## できること

- Planner/Coder/Reviewer などの役割を持つエージェントを切り替えて実行
- ワークフローを段階的に進行（Plan -> Implement -> Review -> Complete）
- 実装時に品質チェックを実行（build/lint/test）

## 前提

- Go 1.26.2
- make

## 使い方

### ビルド

```bash
make build
```

### テスト

```bash
make test
```

### lint

```bash
make lint
```

### 実行

```bash
make run
```

## ディレクトリ構成（抜粋）

- cmd/orchestrator: CLIエントリーポイント
- internal/agent: エージェント生成と役割定義
- internal/executor: コマンド実行戦略
- internal/llm: LLMクライアント抽象
- internal/workflow: ワークフロー状態遷移
- pkg/logger: NDJSONロガー
- pkg/markdown: Markdownパーサー
- pkg/retry: リトライユーティリティ

## 開発メモ

- 変更前に `make test` と `make lint` を実行して、品質を確認してください。
