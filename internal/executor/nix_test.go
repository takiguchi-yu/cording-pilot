package executor_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/takiguchi-yu/cording-pilot/internal/executor"
)

// nixAvailable は、テスト実行環境に nix コマンドが存在するかどうかを返します。
func nixAvailable() bool {
	_, err := exec.LookPath("nix")
	return err == nil
}

func TestNewNixExecutor_nixが存在しない場合はエラーを返す(t *testing.T) {
	t.Parallel()
	if nixAvailable() {
		t.Skip("nix コマンドが存在するためスキップします")
	}
	_, err := executor.NewNixExecutor()
	if err == nil {
		t.Fatal("nix が存在しない環境ではエラーを期待しましたが nil でした")
	}
}

func TestNewNixExecutor_nixが存在する場合は生成に成功する(t *testing.T) {
	t.Parallel()
	if !nixAvailable() {
		t.Skip("nix コマンドが存在しないためスキップします")
	}
	e, err := executor.NewNixExecutor()
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if e == nil {
		t.Fatal("nil NixExecutor が返されました")
	}
}

func TestNewNixExecutorWithTimeout_nixが存在しない場合はエラーを返す(t *testing.T) {
	t.Parallel()
	if nixAvailable() {
		t.Skip("nix コマンドが存在するためスキップします")
	}
	_, err := executor.NewNixExecutorWithTimeout(5 * time.Minute)
	if err == nil {
		t.Fatal("nix が存在しない環境ではエラーを期待しましたが nil でした")
	}
}

func TestNewNixExecutorWithTimeout_nixが存在する場合は生成に成功する(t *testing.T) {
	t.Parallel()
	if !nixAvailable() {
		t.Skip("nix コマンドが存在しないためスキップします")
	}
	e, err := executor.NewNixExecutorWithTimeout(5 * time.Minute)
	if err != nil {
		t.Fatalf("予期しないエラー: %v", err)
	}
	if e == nil {
		t.Fatal("nil NixExecutor が返されました")
	}
}
