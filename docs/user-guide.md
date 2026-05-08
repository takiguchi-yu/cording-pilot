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

必要に応じて `.cording-pilot.yml` の `llm.default.model` も同じモデル名に合わせてください。

## エージェント別 LLM ハイブリッド構成

`cording-pilot` はエージェントごとに異なる LLM プロバイダーとモデルを設定できます。  
「高品質な要件定義は Copilot/GPT、高速な Fix Loop はローカル Ollama」のような使い分けが可能です。

### 設定例: Coder だけ Ollama にする

```yaml
llm:
    default:
        provider: copilot
        model: gpt-4o-mini
    coder:
        provider: ollama
        model: qwen2.5-coder:3b
        base_url: http://localhost:11434/v1
```

| エージェント | プロバイダー              | 用途                                           |
| ------------ | ------------------------- | ---------------------------------------------- |
| Planner      | copilot / gpt-4o-mini     | 要件定義・実装計画（Default にフォールバック） |
| Coder        | ollama / qwen2.5-coder:3b | コード生成・Fix Loop（ローカルで無料・高速）   |
| Reviewer     | copilot / gpt-4o-mini     | コードレビュー（Default にフォールバック）     |

### 設定できるエージェント

| キー           | 説明                                                 |
| -------------- | ---------------------------------------------------- |
| `llm.default`  | 全エージェントのデフォルト設定（必須）               |
| `llm.planner`  | Planner エージェント専用（省略時は default を使用）  |
| `llm.coder`    | Coder エージェント専用（省略時は default を使用）    |
| `llm.reviewer` | Reviewer エージェント専用（省略時は default を使用） |

各エージェント設定で `provider` を省略すると `llm.default.provider` が補完されます。つまり、モデルだけを上書きして別モデルを試す、といった使い方もできます。

```yaml
llm:
    default:
        provider: copilot
        model: gpt-4o-mini
    planner:
        model: gpt-4o # provider は省略 → copilot が補完される
```

旧フォーマット（`llm.provider` / `llm.model` 直書き）はサポート対象外です。必ず `llm.default` 配下で設定してください。

## 言語非依存（Polyglot）設定

`cording-pilot` は `project` と `pipeline` の設定を変えることで Go 以外のプロジェクトでも利用できます。

### project 設定

| フィールド          | 説明                                                    |
| ------------------- | ------------------------------------------------------- |
| `project.language`  | 対象言語（例: `"TypeScript"`, `"Python"`, `"Go"`）      |
| `project.framework` | フレームワーク名（省略可。例: `"Next.js"`, `"Django"`） |

`project.language` が未指定の場合は `"Go"` がデフォルトとなります。

### pipeline 設定

| フィールド          | 説明                                                               |
| ------------------- | ------------------------------------------------------------------ |
| `pipeline.auto_fix` | 品質チェック前に自動修復を試みるコマンドのリスト（失敗しても続行） |
| `pipeline.check`    | 品質チェックコマンドのリスト（いずれかが失敗したら Fix Loop へ）   |

コマンドは `sh -c` 経由で実行されるため、パイプ（`|`）やリダイレクト（`>`）などのシェル構文も使用できます。`pipeline.check` が未指定の場合は `["go build ./...", "go test ./..."]` がデフォルトとなります。

### TypeScript / Node.js (Jest) プロジェクトの例

```yaml
version: "1.0"
project:
    language: "TypeScript"
    framework: "Node.js (Jest)"
llm:
    default:
        provider: copilot
        model: "gpt-4.1"
environment:
    type: local
pipeline:
    auto_fix:
        - "npm run lint -- --fix"
    check:
        - "npx tsc --noEmit"
        - "npm test"
```

### Python / Django プロジェクトの例

```yaml
version: "1.0"
project:
    language: "Python"
    framework: "Django"
llm:
    default:
        provider: copilot
        model: "gpt-4.1"
environment:
    type: local
pipeline:
    auto_fix:
        - "ruff check --fix ."
        - "black ."
    check:
        - "mypy ."
        - "pytest"
```

### Rust プロジェクトの例

```yaml
version: "1.0"
project:
    language: "Rust"
llm:
    default:
        provider: copilot
        model: "gpt-4.1"
environment:
    type: local
pipeline:
    auto_fix:
        - "cargo fmt"
    check:
        - "cargo build"
        - "cargo clippy -- -D warnings"
        - "cargo test"
```
