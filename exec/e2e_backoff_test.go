package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// 端到端：自动重启确实按指数退避等待，而不是固定 2s。
// 子进程每次启动即退出(崩溃)，记录每次启动时间戳；
// 由相邻时间戳的间隔反推退避序列，必须为 base, 2*base, 4*base。
func TestAutoRestartUsesExponentialBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("端到端退避测试跳过")
	}
	dir := t.TempDir()
	stamps := filepath.Join(dir, "starts.txt")
	child := buildCrashChild(t)

	base := 300 * time.Millisecond
	var p Process
	h := Hooks{
		CreateCmd: func() (*exec.Cmd, error) {
			// stamps 作为命令行参数传入，避免写死在子进程源码里
			return exec.Command(child, stamps), nil
		},
		RestartDelay: base,
		AutoRestart:  true,
		RestartTimes: 4,
	}

	if err := p.StartManagedProcess(h); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	// 等前 3 次重启发生（第 1 次启动 + 3 次重启 = 4 条记录）
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if n := countLines(t, stamps); n >= 4 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// 停止自动重启链，避免测试进程悬挂
	if err := p.StopManagedProcess(5 * time.Second); err != nil && !isNotRunning(err) {
		t.Fatalf("停止失败: %v", err)
	}

	got := readLines(t, stamps)
	if len(got) < 4 {
		t.Fatalf("期望至少 4 次启动记录, 实际 %d 条: %v", len(got), got)
	}
	// 第 1 次间隔为 base，第 2 次为 2*base，第 3 次为 4*base。
	// 允许 ±40% 抖动，覆盖 CI 调度噪声；下界必须 > base 才能证明是指数而非固定。
	gaps := gapsOf(got)
	wantGaps := []time.Duration{base, 2 * base, 4 * base}
	for i, want := range wantGaps {
		if i >= len(gaps) {
			break
		}
		if gaps[i] < want*6/10 || gaps[i] > want*14/10 {
			t.Fatalf("第 %d 次退避间隔应为约 %s (容差±40%%), 实际 %s\n全部间隔: %v", i+1, want, gaps[i], gaps)
		}
	}
}

// 端到端：退避上限必须真的生效。
// base=250ms、max=600ms：第 4 次重启理论间隔 2s，但必须被压到 600ms。
// 若上限未透传到 RestartPolicy（比如 Hooks 少了字段），间隔会是指数值，本测试失败。
func TestAutoRestartRespectsBackoffCap(t *testing.T) {
	if testing.Short() {
		t.Skip("端到端退避测试跳过")
	}
	dir := t.TempDir()
	stamps := filepath.Join(dir, "starts.txt")
	child := buildCrashChild(t)

	base := 250 * time.Millisecond
	cap := 600 * time.Millisecond
	var p Process
	h := Hooks{
		CreateCmd:       func() (*exec.Cmd, error) { return exec.Command(child, stamps), nil },
		RestartDelay:    base,
		RestartDelayMax: cap,
		AutoRestart:     true,
		RestartTimes:    6,
	}
	if err := p.StartManagedProcess(h); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if countLines(t, stamps) >= 6 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := p.StopManagedProcess(5 * time.Second); err != nil && !isNotRunning(err) {
		t.Fatalf("停止失败: %v", err)
	}

	gaps := gapsOf(readLines(t, stamps))
	if len(gaps) < 4 {
		t.Fatalf("期望至少 4 个间隔, 实际 %d 个: %v", len(gaps), gaps)
	}
	// 第 4 个间隔：未封顶应为 2s(=250ms<<3)，封顶后必须 <= 600ms
	if gaps[3] > cap*3/2 {
		t.Fatalf("第 4 次退避间隔应被封顶到约 %s, 实际 %s(疑似上限未生效)\n全部间隔: %v",
			cap, gaps[3], gaps)
	}
	// 反向确认：前两个间隔必须仍在指数区间内(不能被一起压平)
	if gaps[0] > base*3/2 {
		t.Fatalf("第 1 次间隔应约等于 base %s, 实际 %s", base, gaps[0])
	}
	if gaps[1] > 2*base*3/2 {
		t.Fatalf("第 2 次间隔应约等于 2*base %s, 实际 %s", 2*base, gaps[1])
	}
}

// 端到端：子进程存活达到稳定期(30s)后重启计数必须清零。
//
// 观测方式：在 OnExit 回调里读取 RestartCount。monitor 先做稳定期判定并写回
// 计数，再调用 OnExit，因此回调里看到的就是"清零后、下一次自增前"的值。
// 不加稳定期重置时，第 2 次退出必然读到 1（每次重启都会自增）；
// 读到 0 即证明稳定期重置生效。
func TestAutoRestartResetsCountAfterStablePeriod(t *testing.T) {
	if testing.Short() {
		t.Skip("稳定期测试需要真实时钟(>30s)，-short 下跳过")
	}
	child := buildStableThenCrashChild(t)

	var p Process
	var mu sync.Mutex
	counts := map[int]int{} // 第 n 次退出(n 从 1 开始) → 当时的 RestartCount
	startedAt := map[int]time.Time{}
	exitNo := 0
	h := Hooks{
		CreateCmd:    func() (*exec.Cmd, error) { return exec.Command(child), nil },
		RestartDelay: 100 * time.Millisecond,
		AutoRestart:  true,
		RestartTimes: -1, // 不因次数上限中止，保证两次退出都能被观察到
		OnStarted:    func(*exec.Cmd) { startedAt[0] = time.Now() },
		OnExit: func(ProcessExit) {
			mu.Lock()
			defer mu.Unlock()
			exitNo++
			p.ProcessMu.Lock()
			counts[exitNo] = p.RestartCount
			p.ProcessMu.Unlock()
		},
	}
	if err := p.StartManagedProcess(h); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	deadline := time.Now().Add(120 * time.Second)
	for {
		mu.Lock()
		n := exitNo
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := p.StopManagedProcess(5 * time.Second); err != nil && !isNotRunning(err) {
		t.Fatalf("停止失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if exitNo < 2 {
		t.Fatalf("期望观测到 2 次退出, 实际 %d 次", exitNo)
	}
	if counts[1] != 0 {
		t.Fatalf("第 1 次退出时重启计数应为 0, 实际 %d", counts[1])
	}
	// 第 2 次退出时子进程已存活 35s+ > 30s 稳定期 → 计数必须被清零。
	// 若未清零，此处必为 1（第 1 次退出后自增，且再无重置点）。
	if counts[2] != 0 {
		t.Fatalf("稳定期后重启计数未清零: 第 2 次退出读到 %d, 期望 0"+
			"（RestartTimes 有限时会因此永久放弃自愈）", counts[2])
	}
}

// countLines 统计文件行数。
func countLines(t *testing.T, path string) int {
	t.Helper()
	return len(readLines(t, path))
}

// readLines 读取并解析文件中所有十进制整数(时间戳)。
func readLines(t *testing.T, path string) []int64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []int64
	var cur int64
	var digits int
	for _, c := range string(b) {
		if c >= '0' && c <= '9' {
			cur = cur*10 + int64(c-'0')
			digits++
			continue
		}
		if digits > 0 {
			out = append(out, cur)
			cur, digits = 0, 0
		}
	}
	return out
}

// gapsOf 计算相邻时间戳的间隔(纳秒转 Duration)。
func gapsOf(ts []int64) []time.Duration {
	var out []time.Duration
	for i := 1; i < len(ts); i++ {
		if ts[i] > ts[i-1] {
			out = append(out, time.Duration(ts[i]-ts[i-1]))
		}
	}
	return out
}

// isNotRunning 判断是否为幂等的"进程未运行"错误。
func isNotRunning(err error) bool {
	return err == ErrProcessNotRunning
}

// buildCrashChild 编译一个"每次启动即崩溃退出"的测试子进程。
// stamps 文件路径由 os.Args[1] 传入，子进程把每次启动的时间戳追加进去，
// 供上层反推退避间隔。
func buildCrashChild(t *testing.T) string {
	t.Helper()
	const src = `package main

import (
	"os"
	"strconv"
	"time"
)

func main() {
	f, err := os.OpenFile(os.Args[1], os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		f.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10) + "\n")
		f.Close()
	}
	os.Exit(1)
}
`
	return buildGoHelper(t, "crashchild", src)
}

// buildStableThenCrashChild 编译一个"存活 35s(>30s 稳定期)后崩溃"的测试子进程。
func buildStableThenCrashChild(t *testing.T) string {
	t.Helper()
	const src = `package main

import (
	"os"
	"time"
)

func main() {
	time.Sleep(35 * time.Second)
	os.Exit(3)
}
`
	return buildGoHelper(t, "stablechild", src)
}
