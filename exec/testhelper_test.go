package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildSignalChild 编译一个测试子进程：
//   - 启动后写 ready marker（表示已进入等待信号状态）
//   - 收到 os.Interrupt 时：若第三个参数为 "1" 则忽略信号继续挂起（模拟卡死）；
//     否则写 exited marker 并以 0 自行退出（证明是优雅退出而非被强杀）
func buildSignalChild(t *testing.T) string {
	t.Helper()
	const src = `package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ready := os.Args[1]
	exited := os.Args[2]
	ignore := len(os.Args) > 3 && os.Args[3] == "1"

	// 同时监听 SIGINT 与 SIGTERM：Windows 的 CTRL_BREAK_EVENT 派发为 os.Interrupt，
	// Unix 的优雅信号是 SIGTERM。两个都监听才能让本用例在两个平台等价通过。
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	os.WriteFile(ready, []byte("ready"), 0o644)

	select {
	case <-ch:
		if ignore {
			for {
				time.Sleep(time.Hour)
			}
		}
		os.WriteFile(exited, []byte("graceful"), 0o644)
		os.Exit(0)
	case <-time.After(60 * time.Second):
		os.Exit(9)
	}
}
`
	return buildGoHelper(t, "signalchild", src)
}

// buildGroupChild 编译一个会派生孙进程的测试子进程，用于验证进程组收敛：
//   - 启动孙进程（同一二进制，参数带 grand 标记）
//   - 孙进程写 grandReady 并等待信号；收到信号后写 grandExited 并退出
//   - 父级子进程同样等待信号后退出
func buildGroupChild(t *testing.T) string {
	t.Helper()
	const src = `package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 同时监听 SIGINT 与 SIGTERM（Windows 派发 os.Interrupt，Unix 用 SIGTERM）
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	if len(os.Args) > 4 && os.Args[4] == "grand" {
		grandReady := os.Args[2]
		grandExited := os.Args[3]
		os.WriteFile(grandReady, []byte("ready"), 0o644)
		select {
		case <-ch:
			os.WriteFile(grandExited, []byte("graceful"), 0o644)
			os.Exit(0)
		case <-time.After(60 * time.Second):
			os.Exit(9)
		}
	}

	childReady := os.Args[1]
	grand := exec.Command(os.Args[0], os.Args[1], os.Args[2], os.Args[3], "grand")
	grand.Stdout, grand.Stderr = os.Stdout, os.Stderr
	if err := grand.Start(); err != nil {
		os.Exit(8)
	}
	os.WriteFile(childReady, []byte("ready"), 0o644)

	select {
	case <-ch:
		os.Exit(0)
	case <-time.After(60 * time.Second):
		os.Exit(9)
	}
}
`
	return buildGoHelper(t, "groupchild", src)
}

// buildGoHelper 把一段 Go 源码编译为临时二进制并返回路径。
// 用临时二进制而非 go run：go run 会引入额外的中间进程，
// 使"进程组收敛"的断言对象不再唯一。
func buildGoHelper(t *testing.T, name, src string) string {
	t.Helper()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, name+".go")
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		t.Fatalf("写测试源码失败: %v", err)
	}
	bin := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBin += ".exe"
	}
	cmd := exec.Command(goBin, "build", "-o", bin, srcPath)
	// 清掉外部 GOFLAGS，避免宿主机上的全局标志污染测试内构建
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译测试子进程失败: %v\n%s", err, out)
	}
	return bin
}
