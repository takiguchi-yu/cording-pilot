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

## 配布バイナリの使い方

`main` にマージされると GitHub Actions で各OS向けバイナリが自動ビルドされ、
GitHub Releases に添付されます。

### 1. バイナリをダウンロード

Releases ページから使用環境に合うファイルを取得してください。

- Linux (x86_64): `cording-pilot-linux-amd64`
- Linux (ARM64): `cording-pilot-linux-arm64`
- macOS (Intel): `cording-pilot-darwin-amd64`
- macOS (Apple Silicon): `cording-pilot-darwin-arm64`
- Windows (x86_64): `cording-pilot-windows-amd64.exe`

### 2. 実行権限を付与（Linux/macOS）

```bash
chmod +x cording-pilot-<os>-<arch>
```

例（macOS Apple Silicon）:

```bash
chmod +x cording-pilot-darwin-arm64
./cording-pilot-darwin-arm64 "文字列を逆順にする関数"
```

### 3. Windows での実行

PowerShell で次のように実行します。

```powershell
.\cording-pilot-windows-amd64.exe "文字列を逆順にする関数"
```

### 4. チェックサム検証（任意）

Release に含まれる `checksums.txt` を使って整合性を検証できます。

```bash
sha256sum -c checksums.txt
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

## 実行環境の選択

`.cording-pilot.yml` の `environment.type` で実行バックエンドを切り替えられます。

| type                   | 説明                                                               |
| ---------------------- | ------------------------------------------------------------------ |
| `local` （デフォルト） | ホストマシンで直接コマンドを実行します                             |
| `docker`               | Docker コンテナ内で実行します（`image` フィールドも必要）          |
| `nix`                  | リポジトリの `flake.nix` を用いた `nix develop` 環境内で実行します |

### Nix を使った隔離実行モード

`flake.nix` を持つリポジトリで再現性の高い環境を利用できます。

**前提**: ホストマシンに [Nix](https://nixos.org/download.html) がインストールされていること。

設定ファイル (`.cording-pilot.yml`) で有効化:

```yaml
environment:
    type: "nix"
```

または CLI フラグで一時的に有効化:

```bash
orchestrator --nix "実装要件"
```

> **Note**: `nix develop` は初回起動時にパッケージのダウンロード・ビルドが走るため、
> デフォルトタイムアウトは 15 分に設定されています。

## 開発メモ

- 変更前に `make test` と `make lint` を実行して、品質を確認してください。
