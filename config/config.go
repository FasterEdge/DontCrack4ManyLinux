// FasterEdge 开源项目 - Github: https://github.com/FasterEdge - Gitee: https://gitee.com/FasterEdge
package config

import "time"

// FasterEdge - 对称、可靠、安全的多场景边缘计算框架
// https://github.com/FasterEdge
// DontCrack 通用 Linux (manylinux) 版
// https://github.com/FasterEdge/DontCrack4ManyLinux
// tyza66
// https://github.com/tyza66
type Config struct {
	// 系统相关
	Version string // 系统版本号(主方法中写死定义)
	// 子进程相关
	Path         string // 打开子进程的可执行文件路径(必填)
	Args         string // 打开子进程所携带的参数(可选)
	Pre          string // 启动前要执行的命令(可选，只能一条但可以是sh脚本或&&连接的命令)
	Env          string // 子进程的环境变量(可选，如: "PATH=/usr/local/bin:/usr/bin FOO=bar")
	AutoRestart  bool   // 子进程退出后是否自动重启(可选，默认关闭)
	RestartTimes int    // 最大重试次数(可选，默认3次，-1表示无限次)
	// 重启退避：Delay 为退避基数，DelayMax 为退避上限(0 表示不封顶)
	// 第 n 次重启前等待 min(base<<(n-1), DelayMax)，语义与 DontCrack-Manager 的
	// nextBackoff(base<<shift) 一致；子进程存活达到 StableThreshold 后计数清零。
	RestartDelay    time.Duration // 自动重启退避基数(可选，默认2s)
	RestartDelayMax time.Duration // 退避等待上限(可选，默认0即不封顶)
	StartNow        bool          // 是否立即启动子进程(可选，默认关闭)
	// 健康探针
	ProbeCmd          string // 探针命令(如 "curl -sf http://localhost:8080/health")，空则禁用
	ProbeInterval     int    // 探针间隔(秒，默认30)
	ProbeTimeout      int    // 探针超时(秒，默认5)
	ProbeFailureLimit int    // 连续失败多少次算不健康(默认3)
	// 管理器相关
	Port            int    // 管理进程使用的进程端口(可选，默认11883)
	ListenAddress   string // HTTP 监听地址(可选，默认 127.0.0.1，仅本机访问)
	Password        string // 管理进程使用的密码(可选)
	LogCapacity     int    // 日志缓存的最大行数(可选，默认200行)
	LogMaxLineBytes int    // 日志单行最大字节数(可选，默认1MiB)
	FileLogEnabled  bool   // 是否启用文件日志(可选，默认关闭)
	LocalLogPath    string // 日志文件目录(可选，默认 ./logs/proc_manager/进程名)
	LocalLogLifeDay int    // 本地日志文件保存天数(可选，默认7天)
}

// DefaultRestartDelay 是自动重启退避基数的缺省值(与历史固定 2s 一致)。
const DefaultRestartDelay = 2 * time.Second

// 将传入的参数转换为全局配置结构体
func ParseConfig(version string, path string, args string, pre string, env string, autoRestart bool, restartTimes int, startNow bool,
	port int, password string, logCapacity int, logMaxLineBytes int, fileLogEnabled bool, localLogPath string, localLogLifeDay int) *Config {
	return ParseConfigListen(version, path, args, pre, env, autoRestart, restartTimes, startNow,
		port, "127.0.0.1", password, logCapacity, logMaxLineBytes, fileLogEnabled, localLogPath, localLogLifeDay)
}

// ParseConfigListen 是带监听地址的完整构造函数
func ParseConfigListen(version string, path string, args string, pre string, env string, autoRestart bool, restartTimes int, startNow bool,
	port int, listenAddress string, password string, logCapacity int, logMaxLineBytes int, fileLogEnabled bool, localLogPath string, localLogLifeDay int) *Config {
	config := new(Config)
	config.Version = version
	config.Path = path
	config.Args = args
	config.Pre = pre
	config.Env = env
	config.AutoRestart = autoRestart
	config.RestartTimes = restartTimes
	// 缺省退避与历史行为等价：固定 2s，无指数增长
	config.RestartDelay = DefaultRestartDelay
	config.RestartDelayMax = 0
	config.StartNow = startNow
	// 探针默认关闭，所有参数留零值，stater.go 中会用 cfg.ProbeCmd != "" 判断是否启用
	config.Port = port
	if listenAddress == "" {
		listenAddress = "127.0.0.1"
	}
	config.ListenAddress = listenAddress
	config.Password = password
	config.LogCapacity = logCapacity
	config.LogMaxLineBytes = logMaxLineBytes
	config.FileLogEnabled = fileLogEnabled
	config.LocalLogPath = localLogPath
	config.LocalLogLifeDay = localLogLifeDay
	return config
}

// ParseConfigWithProbe 是带探针参数的扩展构造函数
func ParseConfigWithProbe(version string, path string, args string, pre string, env string, autoRestart bool, restartTimes int, startNow bool,
	port int, password string, logCapacity int, logMaxLineBytes int, fileLogEnabled bool, localLogPath string, localLogLifeDay int,
	probeCmd string, probeInterval int, probeTimeout int, probeFailureLimit int) *Config {
	return ParseConfigListenWithProbe(version, path, args, pre, env, autoRestart, restartTimes, startNow,
		port, "127.0.0.1", password, logCapacity, logMaxLineBytes, fileLogEnabled, localLogPath, localLogLifeDay,
		probeCmd, probeInterval, probeTimeout, probeFailureLimit)
}

// ParseConfigListenWithProbe 是带监听地址和探针参数的完整构造函数
func ParseConfigListenWithProbe(version string, path string, args string, pre string, env string, autoRestart bool, restartTimes int, startNow bool,
	port int, listenAddress string, password string, logCapacity int, logMaxLineBytes int, fileLogEnabled bool, localLogPath string, localLogLifeDay int,
	probeCmd string, probeInterval int, probeTimeout int, probeFailureLimit int) *Config {
	return ParseConfigListenWithProbeAndBackoff(version, path, args, pre, env, autoRestart, restartTimes, startNow,
		port, listenAddress, password, logCapacity, logMaxLineBytes, fileLogEnabled, localLogPath, localLogLifeDay,
		probeCmd, probeInterval, probeTimeout, probeFailureLimit, DefaultRestartDelay, 0)
}

// ParseConfigListenWithProbeAndBackoff 是带退避参数的完整构造函数。
// restartDelay<=0 时回落到 DefaultRestartDelay；restartDelayMax<0 视为 0(不封顶)。
func ParseConfigListenWithProbeAndBackoff(version string, path string, args string, pre string, env string, autoRestart bool, restartTimes int, startNow bool,
	port int, listenAddress string, password string, logCapacity int, logMaxLineBytes int, fileLogEnabled bool, localLogPath string, localLogLifeDay int,
	probeCmd string, probeInterval int, probeTimeout int, probeFailureLimit int,
	restartDelay time.Duration, restartDelayMax time.Duration) *Config {
	c := ParseConfigListen(version, path, args, pre, env, autoRestart, restartTimes, startNow,
		port, listenAddress, password, logCapacity, logMaxLineBytes, fileLogEnabled, localLogPath, localLogLifeDay)
	c.ProbeCmd = probeCmd
	c.ProbeInterval = probeInterval
	c.ProbeTimeout = probeTimeout
	c.ProbeFailureLimit = probeFailureLimit
	c.RestartDelay = restartDelay
	if c.RestartDelay <= 0 {
		c.RestartDelay = DefaultRestartDelay
	}
	if restartDelayMax < 0 {
		restartDelayMax = 0
	}
	c.RestartDelayMax = restartDelayMax
	return c
}
