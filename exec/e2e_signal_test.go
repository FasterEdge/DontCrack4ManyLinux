package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// 端到端：StopManagedProcess 必须走"优雅信号"路径，让子进程自行退出，
// 而不是硬杀。子进程收到信号后会写下 exitedMarker 并 exit(0)；
// 若被硬杀(SIGKILL / TerminateProcess)，该文件不会出现，本测试即失败。
func TestStopManagedProcessSendsGracefulSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("端到端信号测试跳过")
	}
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.marker")
	exited := filepath.Join(dir, "exited.marker")
	child := buildSignalChild(t)

	var p Process
	h := Hooks{
		CreateCmd:    func() (*exec.Cmd, error) { return exec.Command(child, ready, exited), nil },
		RestartDelay: time.Second,
	}
	if err := p.StartManagedProcess(h); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitForFile(t, ready) // 子进程已进入"等待信号"状态

	if err := p.StopManagedProcess(5 * time.Second); err != nil {
		t.Fatalf("优雅停止失败: %v", err)
	}
	// exited 由子进程的 signal handler 写出 ⇒ 证明是自行退出，不是被强杀
	waitForFile(t, exited)

	p.ProcessMu.Lock()
	stopped := p.stoppedByReq
	running := p.IsRunning
	p.ProcessMu.Unlock()
	if !stopped {
		t.Fatal("优雅停止后必须标记 stoppedByReq，否则 monitor 会把它自动拉起")
	}
	if running {
		t.Fatal("优雅停止返回后进程必须已不在运行")
	}
}

// 端到端：忽略优雅信号的子进程，必须在宽限期后被强杀收敛。
// 该用例证明"优雅优先、强杀兜底"是完整的两段式，而非停在优雅阶段。
func TestStopManagedProcessEscalatesToForceKill(t *testing.T) {
	if testing.Short() {
		t.Skip("端到端信号测试跳过")
	}
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready.marker")
	exited := filepath.Join(dir, "exited.marker")
	child := buildSignalChild(t)

	var p Process
	h := Hooks{
		// ignore=1：子进程收到信号后不退出，迫使管理器走强杀分支
		CreateCmd:    func() (*exec.Cmd, error) { return exec.Command(child, ready, exited, "1"), nil },
		RestartDelay: time.Second,
	}
	if err := p.StartManagedProcess(h); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitForFile(t, ready)

	const grace = 1 * time.Second
	start := time.Now()
	if err := p.StopManagedProcess(grace); err != nil {
		t.Fatalf("强杀收敛失败: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < grace {
		t.Fatalf("强杀必须发生在优雅宽限 %s 之后, 实际仅 %s —— 说明没给优雅信号留时间", grace, elapsed)
	}
	if _, err := os.Stat(exited); err == nil {
		t.Fatal("子进程声明忽略信号，exited marker 不应出现（出现即说明优雅信号未被正确处理）")
	}
	p.ProcessMu.Lock()
	notRunning := !p.IsRunning && p.CurrentProcess == nil
	p.ProcessMu.Unlock()
	if !notRunning {
		t.Fatal("强杀后 IsRunning 必须为 false 且 CurrentProcess 必须被释放")
	}
}

// 端到端：停止必须收敛整个进程组，而不是只杀主进程。
// 子进程会派生孙进程；停止后孙进程也必须收到信号并退出，否则即孤儿泄漏。
func TestStopManagedProcessConvergesProcessGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("端到端信号测试跳过")
	}
	dir := t.TempDir()
	childReady := filepath.Join(dir, "child.ready")
	grandReady := filepath.Join(dir, "grand.ready")
	grandExited := filepath.Join(dir, "grand.exited")
	child := buildGroupChild(t)

	var p Process
	h := Hooks{
		CreateCmd: func() (*exec.Cmd, error) {
			return exec.Command(child, childReady, grandReady, grandExited), nil
		},
		RestartDelay: time.Second,
	}
	if err := p.StartManagedProcess(h); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	waitForFile(t, grandReady) // 孙进程已就绪

	if err := p.StopManagedProcess(5 * time.Second); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	// 孙进程只在收到信号时才写该文件 ⇒ 出现即证明信号覆盖了整棵进程树
	waitForFile(t, grandExited)
}

// waitForFile 等待文件出现，超时即失败。
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("等待文件超时: %s", path)
}
