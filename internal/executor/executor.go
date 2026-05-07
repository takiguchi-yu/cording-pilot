// Package executor はシェルコマンドを実行するための Strategy インターフェースを定義します。
package executor

import "context"

// Executor は外部コマンドを実行するための Strategy インターフェースです。
// 実装はローカル環境、Docker コンテナ、その他任意の実行バックエンドを対象にできます。
// 実装はゴルーチン安全である必要があります。
type Executor interface {
	// Run は dir 内で cmd を args 付きで実行します。
	// stdout/stderr の結合出力、コマンドが成功（終了コード 0）したかどうか、
	// およびインフラレベルのエラー（タイムアウトやバイナリ晲見つからないなど）を返します。
	// 非ゼロの終了コードは err ではなく success=false で表します。
	Run(ctx context.Context, dir, cmd string, args ...string) (output string, success bool, err error)
}
