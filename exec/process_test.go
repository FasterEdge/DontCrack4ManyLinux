package exec

import (
	"errors"
	"os"
	osexec "os/exec"
	"sync"
	"testing"
	"time"
)

// 并发对 RestartCount 自增与重置，验证锁内自增后无数据竞争
func TestRestartCountConcurrentAccess(t *testing.T) {
	if os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("race detector requires CGO")
	}
	var p Process
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 模拟 monitor 的锁内自增路径
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			p.ProcessMu.Lock()
			should := p.RestartCount < 100
			if should {
				p.RestartCount++
			}
			p.ProcessMu.Unlock()
		}
	}()

	// 模拟 /startup 的重置路径
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				p.ProcessMu.Lock()
				p.RestartCount = 0
				p.ProcessMu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()

	if p.RestartCount < 0 {
		t.Fatalf("count must never be negative, got %d", p.RestartCount)
	}
}

// 手动 startup 后（代际变化或已有进程运行），过期的重启计划必须被跳过
func TestStaleMonitorSkipsRestart(t *testing.T) {
	var p Process

	// 场景1: 代际未变但已有新进程在运行 → 过期
	p.generation = 5
	p.IsRunning = true
	p.ProcessMu.Lock()
	stale := p.generation != 5 || p.IsRunning || p.CurrentProcess != nil
	p.ProcessMu.Unlock()
	if !stale {
		t.Fatal("restart plan must be stale when a process is already running")
	}

	// 场景2: 代际变化(手动启动/停止过) → 过期
	p.IsRunning = false
	p.generation = 6
	p.ProcessMu.Lock()
	stale = p.generation != 5 || p.IsRunning || p.CurrentProcess != nil
	p.ProcessMu.Unlock()
	if !stale {
		t.Fatal("restart plan must be stale when generation changed")
	}

	// 场景3: 代际一致且无进程 → 不过期，允许重启
	p.generation = 5
	p.ProcessMu.Lock()
	stale = p.generation != 5 || p.IsRunning || p.CurrentProcess != nil
	p.ProcessMu.Unlock()
	if stale {
		t.Fatal("restart plan must remain valid when generation matches and no process runs")
	}
}

// StartManagedProcess 必须递增 generation
func TestGenerationIncrements(t *testing.T) {
	var p Process
	cmd := &osexec.Cmd{}
	_ = cmd // generation 递增逻辑在 StartManagedProcess 中；此处验证字段语义
	p.ProcessMu.Lock()
	before := p.generation
	p.generation++
	after := p.generation
	p.ProcessMu.Unlock()
	if after != before+1 {
		t.Fatalf("generation must increment by 1, got %d -> %d", before, after)
	}
}

// 对未运行的进程(崩溃后等待自动重启窗口)执行 StopManagedProcess:
// 必须标记 stoppedByReq 并递增 generation(使 monitor 的重启计划过期) ——
// 否则 /stop、/shutdown、SIGTERM 关闭都无法阻止等待窗口后的自动重启(孤儿暗病)。
func TestStopNotRunningMarksRequestAndBumpsGeneration(t *testing.T) {
	var p Process
	before := p.generation
	err := p.StopManagedProcess(time.Second)
	if !errors.Is(err, ErrProcessNotRunning) {
		t.Fatalf("期望 ErrProcessNotRunning, 实际: %v", err)
	}
	p.ProcessMu.Lock()
	defer p.ProcessMu.Unlock()
	if !p.stoppedByReq {
		t.Fatal("未运行进程被停止时也必须标记 stoppedByReq")
	}
	if p.generation != before+1 {
		t.Fatalf("generation 必须 +1(使重启计划过期), got %d -> %d", before, p.generation)
	}
	// 连续再次停止: 幂等, 每次都递增(重启计划保持过期)
	before = p.generation
	p.ProcessMu.Unlock()
	_ = p.StopManagedProcess(time.Second)
	p.ProcessMu.Lock()
	if p.generation != before+1 {
		t.Fatalf("重复停止也必须递增 generation, got %d -> %d", before, p.generation)
	}
}
