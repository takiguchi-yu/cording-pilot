# ユーザーガイド

このドキュメントは、`cording-pilot` を日常利用するための最小手順をまとめたガイドです。

## 事前準備

- Go と `make` をインストール
- LLM プロバイダーを準備（`copilot` または `ollama`）

## 基本の実行

```bash
cording-pilot "要件をここに記述"
```

設定ファイルは既定で `.cording-pilot.yml` が読み込まれます。

## Ollama 利用時のセットアップ

```bash
make ollama-pull
make ollama-serve
```

- `make ollama-pull` は既定で `qwen3-coder-next:q4_K_M` を取得します。
- `make ollama-serve` は負荷抑制のため `OLLAMA_NUM_PARALLEL=1` と `OLLAMA_MAX_QUEUE=1` を指定して起動します。

## PC が重い場合の Tips

`OLLAMA_MODEL` をより小さいサイズへ切り替えると、メモリ・GPU 使用量を抑えられます。

```bash
OLLAMA_MODEL=qwen3:3b make ollama-pull
```

必要に応じて `.cording-pilot.yml` の `llm.model` も同じモデル名に合わせてください。
