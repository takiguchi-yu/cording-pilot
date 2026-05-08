# cording-pilot

Go製のAIエージェントオーケストレーターCLIです。要件整理から実装・レビューループ、品質チェックまでをステートマシンで実行します。

[![CI](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/ci.yml/badge.svg)](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/ci.yml) [![Release](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/release.yml/badge.svg)](https://github.com/takiguchi-yu/cording-pilot/actions/workflows/release.yml)

## サマリー

- 要件を対話または Issue から受け取り、Planner/Coder/Reviewer の役割で段階的に実行します
- Implement フェーズはテスト先行の TDD スタイル（テスト生成→実装生成→パイプライン実行）で実装を作成し、Fix Loop で再試行します
- 実行時の品質ゲート（format → type check → lint → test）をパイプラインとして定義できます

## できること

- 要件から実装計画を自動生成（Planner）
- テストと実装コードを LLM（Coder）で生成
- 差分を自動でレビュー（Reviewer）し、承認/差し戻しを制御
- `local` / `docker` / `nix` の実行環境を切り替えて隔離実行

## クイックスタート

```bash
# GitHub Releases から実行環境に合うバイナリを取得して PATH に配置

# 要件文字列を渡して実行
cording-pilot "文字列を逆順にする関数を追加してください"
```

初回実行では対話で要件の補足や repo 情報の入力を求められます。Issue 番号や Issue URL を指定して開始することも可能です。

## 入力パターン

### 1. 要件文字列を直接渡す

```bash
cording-pilot "HTTP ハンドラにバリデーションを追加して"
```

### 2. GitHub Issue 番号を指定する

```bash
cording-pilot --issue 123
```

### 3. GitHub Issue URL を指定する

```bash
cording-pilot https://github.com/owner/repo/issues/42
```

Issue URL を渡した場合は `owner/repo/番号` が自動解析されます。

## 主要コマンド（CLI）

|                       コマンド | 説明                                                   |
| -----------------------------: | ------------------------------------------------------ |
|       `cording-pilot "<要件>"` | 要件文字列で実行                                       |
| `cording-pilot --issue <番号>` | GitHub Issue 番号で実行                                |
|    `cording-pilot <issue-url>` | Issue URL を直接指定                                   |
|              `--config <path>` | 設定ファイルを指定（デフォルト: `.cording-pilot.yml`） |
|           `--docker` / `--nix` | 実行環境を一時上書き（`environment.type` を上書き）    |

## CLI オプション（詳細）

| オプション               | 説明                                                 |
| ------------------------ | ---------------------------------------------------- |
| `--config <path>`        | 設定ファイルパス（デフォルト: `.cording-pilot.yml`） |
| `--docker`               | Docker Executor を使用（`environment.type` 上書き）  |
| `--docker-image <image>` | Docker イメージを上書き                              |
| `--nix`                  | Nix Executor を使用（`environment.type` 上書き）     |
| `--issue <number>`       | 処理対象の GitHub Issue 番号を指定                   |

## 実行環境の切り替え

`.cording-pilot.yml` の `environment.type` で切り替えます。

- `local`: ホストで直接実行（デフォルト）
- `docker`: Docker コンテナで実行（`environment.image` が必要な場合あり）
- `nix`: `flake.nix` を利用した `nix develop` 環境で実行

例（Nix を一時的に使用）:

```bash
cording-pilot --nix "実装要件"
```

## 設定（`.cording-pilot.yml`）

設定ファイルは厳格に検証されます。主な制約:

- `version` は `"1.0"`
- `llm.default.provider` は `copilot` または `ollama`
- `environment.type` は `local` / `docker` / `nix`（`docker` の場合は `environment.image` 必須）
- YAML の未知フィールドはエラー（`KnownFields(true)`）
- 複数 YAML ドキュメントはサポート外

最小例:

```yaml
version: "1.0"
llm:
    default:
        provider: "copilot"
        model: "gpt-4.1"
environment:
    type: "local"
```

### エージェント別 LLM ハイブリッド構成

`llm.default` にデフォルト設定を書き、`llm.planner` / `llm.coder` / `llm.reviewer` でエージェントごとに別のプロバイダーとモデルを指定できます。
省略したエージェントは `llm.default` の設定にフォールバックします。

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

このように書くことで「実装フェーズ（Coder）だけはローカルの軽量モデルを使って爆速・無料で回す」という運用が可能になります。Planner と Reviewer は `llm.default` の Copilot/GPT が担当し、高品質な要件定義とコードレビューを維持しながら、Fix Loop のコスト・速度を最適化できます。

### プロジェクト固有の知識（Knowledge）の注入

`.cording-pilot.yml` の `knowledge` フィールドで、プロジェクト固有のコーディング規約やアーキテクチャルールを記述したファイル・ディレクトリを指定できます。
指定した内容は Planner・Coder の各エージェントのプロンプト先頭に自動注入されます。

```yaml
knowledge:
    - ".github/copilot-instructions.md" # ファイル指定
    - "docs/architecture.md" # ファイル指定
    - "docs/rules/" # ディレクトリ指定（.md / .txt のみ読み込まれます）
```

**Tips: LLM に伝わりやすいドキュメントの書き方**

- 「〜すること」「〜は使用しないこと」のような明確なルールベースの記述が効果的です。
- 禁止事項と推奨事項を箇条書きで分けると理解しやすくなります。
- コード例を含めることで、LLM が正確なパターンを学習しやすくなります。
- 1 ファイルには 1 つのトピックを記述し、ファイル名を意味のある名前にしてください（例: `error-handling.md`、`api-conventions.md`）。

既定では `internal/config.DefaultGoConfig()` のパイプライン (`goimports`, `go fmt`, `go build`, `golangci-lint`, `go test`) が使われます。

## Ollama 利用時の推奨設定

- `make ollama-pull` は既定で `qwen3-coder-next:q4_K_M` を取得します。
- `make ollama-serve` は `OLLAMA_NUM_PARALLEL=1` と `OLLAMA_MAX_QUEUE=1` で起動し、PC 全体の負荷上昇を抑えます。
- 動作が重い場合は、より小さいモデルへ変更して起動してください（例: `3b`）。

```bash
OLLAMA_MODEL=qwen3:3b make ollama-pull
```

## 実行ログと中断

- 実行ログはカレントディレクトリの `run.ndjson` に NDJSON 形式で出力されます。

## 開発者向け情報

- 開発者向けガイド: [docs/developer-guide.md](./docs/developer-guide.md)
