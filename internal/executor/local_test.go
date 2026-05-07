package executor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/takiguchi-yu/cording-pilot/internal/executor"
)

func TestNewLocalExecutorWithTimeout_カスタムタイムアウトで生成する(t *testing.T) {
	t.Parallel()
	e := executor.NewLocalExecutorWithTimeout(30 * time.Second)
	if e == nil {
		t.Fatal("nil Executor が返されました")
	}
}

func TestLocalExecutor_Run_コマンドが成功する(t *testing.T) {
	t.Parallel()
	e := executor.NewLocalExecutor()
	out, ok, err := e.Run(context.Background(), t.TempDir(), "echo", "hello")
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if !ok {
		t.Error("success=true を期待しましたが false でした")
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("出力に 'hello' が含まれていません; got: %q", out)
	}
}

func TestLocalExecutor_Run_非ゼロ終了コードはsuccess_falseかつerr_nil(t *testing.T) {
	t.Parallel()
	e := executor.NewLocalExecutor()
	// `false` コマンドは常に終了コード 1 を返す。
	_, ok, err := e.Run(context.Background(), t.TempDir(), "false")
	if err != nil {
		t.Fatalf("非ゼロ終了コードは err=nil であるべきですが %v でした", err)
	}
	if ok {
		t.Error("success=false を期待しましたが true でした")
	}
}

func TestLocalExecutor_Run_エグゼキュータータイムアウト時にエラーを返す(t *testing.T) {
	t.Parallel()
	e := executor.NewLocalExecutorWithTimeout(10 * time.Millisecond)
	_, _, err := e.Run(context.Background(), t.TempDir(), "sleep", "10")
	if err == nil {
		t.Fatal("タイムアウト後にエラーを期待しましたが nil でした")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("エラーメッセージに 'timed out' が含まれていません; got: %v", err)
	}
}

func TestLocalExecutor_Run_コンテキストタイムアウト時にエラーを返す(t *testing.T) {
	t.Parallel()
	// エグゼキューターのタイムアウト(5s) より先に親 ctx が切れるケース。
	e := executor.NewLocalExecutorWithTimeout(5 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, _, err := e.Run(ctx, t.TempDir(), "sleep", "10")
	if err == nil {
		t.Fatal("コンテキストタイムアウト後にエラーを期待しましたが nil でした")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("エラーメッセージに 'timed out' が含まれていません; got: %v", err)
	}
}

func TestLocalExecutor_Run_stdoutとstderrが結合して返る(t *testing.T) {
	t.Parallel()
	e := executor.NewLocalExecutor()
	// sh -c で stdout と stderr の両方に書き込んで終了コード 1 を返す。
	out, _, err := e.Run(
		context.Background(), t.TempDir(),
		"sh", "-c", "echo stdout_msg; echo stderr_msg >&2; exit 1",
	)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if !strings.Contains(out, "stdout_msg") {
		t.Errorf("stdout が含まれていません; got: %q", out)
	}
	if !strings.Contains(out, "stderr_msg") {
		t.Errorf("stderr が含まれていません; got: %q", out)
	}
}
