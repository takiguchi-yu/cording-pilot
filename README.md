# cording-pilot

Go製のAIエージェントオーケストレーターCLIです。  
要件整理、実装計画、実装、レビューの流れをステートマシンで管理し、品質チェックまで自動化することを目的としています。

[![CI](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/ci.yml/badge.svg)](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/ci.yml)

## できること

- Planner/Coder/Reviewer などの役割を持つエージェントを切り替えて実行
- ワークフローを段階的に進行（Plan -> Implement -> Review -> Complete）
- 実装時に品質チェックを実行（build/lint/test）

## ドキュメント

- 利用者向け: [docs/user-guide.md](docs/user-guide.md)
- 開発者向け: [docs/developer-guide.md](docs/developer-guide.md)

## クイックスタート

- Go 1.26.2
- make

```bash
make build
make run
```

詳しい使い方:

- エンドユーザーとして使う場合は [docs/user-guide.md](docs/user-guide.md)
- 開発・保守する場合は [docs/developer-guide.md](docs/developer-guide.md)
