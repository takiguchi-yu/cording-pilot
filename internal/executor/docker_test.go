package executor_test

import (
	"testing"
	"time"

	"github.com/takiguchi-yu/cording-pilot/internal/executor"
)

func TestNewDockerExecutor_空のimageはデフォルトイメージを使用する(t *testing.T) {
	t.Parallel()
	e := executor.NewDockerExecutor("")
	if e == nil {
		t.Fatal("nil DockerExecutor が返されました")
	}
}

func TestNewDockerExecutor_指定したimageで生成する(t *testing.T) {
	t.Parallel()
	e := executor.NewDockerExecutor("golang:1.22")
	if e == nil {
		t.Fatal("nil DockerExecutor が返されました")
	}
}

func TestNewDockerExecutorWithTimeout_カスタムタイムアウトで生成する(t *testing.T) {
	t.Parallel()
	e := executor.NewDockerExecutorWithTimeout("golang:1.22", 5*time.Minute)
	if e == nil {
		t.Fatal("nil DockerExecutor が返されました")
	}
}

func TestNewDockerExecutorWithTimeout_空のimageはデフォルトイメージを使用する(t *testing.T) {
	t.Parallel()
	e := executor.NewDockerExecutorWithTimeout("", 5*time.Minute)
	if e == nil {
		t.Fatal("nil DockerExecutor が返されました")
	}
}
