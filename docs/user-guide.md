# 利用者向けガイド

このドキュメントは、`cording-pilot` を「使う側」のための手順をまとめたものです。

## 対象読者

- リリース済みバイナリを使って実行したい人
- 手元で動作確認したい人
- 自分のリポジトリで要件から実装フローを回したい人

## 前提

- Git リポジトリで作業していること
- `GITHUB_TOKEN` が設定されていること（`repo` 権限推奨）

```bash
export GITHUB_TOKEN=ghp_xxx
```

## クイックスタート（ソースから実行）

1. リポジトリを取得

```bash
git clone https://github.com/takiguchi-yu/cording-pilot.git
cd cording-pilot
```

2. ビルド確認

```bash
make build
```

3. 実行

```bash
make run
```

実行すると対話で Issue 種別を選択し、Plan -> Implement -> Review -> Complete の順で処理が進みます。

## 既存 Issue から開始する

Issue 番号がある場合は、要件文字列の代わりに Issue を指定できます。

```bash
go run ./cmd/orchestrator --issue 123
```

## 実行オプション

```bash
go run ./cmd/orchestrator --help
```

主なオプション:

- `--config`: 設定ファイルパス（デフォルト: `.cording-pilot.yml`）
- `--docker`: Docker Executor を使用
- `--docker-image`: Docker イメージを上書き
- `--nix`: Nix Executor を使用
- `--issue`: 処理対象 Issue 番号を指定

## 実行環境の切り替え

`.cording-pilot.yml` の `environment.type` で切り替えられます。

- `local`: ホストで直接実行（デフォルト）
- `docker`: Docker コンテナ内で実行（`environment.image` 必須）
- `nix`: `flake.nix` を使った `nix develop` 環境内で実行

Nix を一時的に使う場合:

```bash
go run ./cmd/orchestrator --nix "実装要件"
```

## 配布バイナリの使い方

`main` へのマージ後、GitHub Releases に各 OS 向けバイナリが添付されます。

対応バイナリ:

- Linux (x86_64): `cording-pilot-linux-amd64`
- Linux (ARM64): `cording-pilot-linux-arm64`
- macOS (Intel): `cording-pilot-darwin-amd64`
- macOS (Apple Silicon): `cording-pilot-darwin-arm64`
- Windows (x86_64): `cording-pilot-windows-amd64.exe`

Linux/macOS の例:

```bash
chmod +x cording-pilot-darwin-arm64
./cording-pilot-darwin-arm64 "文字列を逆順にする関数"
```

Windows の例:

```powershell
.\cording-pilot-windows-amd64.exe "文字列を逆順にする関数"
```

## ログと中断

- 実行ログは `run.ndjson` に出力されます
- 途中で終了したい場合は `Ctrl+C` で中断できます
