<div align="center">
<img src="./Logo.png" alt="logo" width="100"/>
<h2>DontCrack4ManyLinux</h2>
<h3>通用 Linux (manylinux) 进程管理器</h3>
</div>


### 一、功能简介

- 通用 Linux 发行版 (manylinux) 下的进程管理器，用于提高后台/守护进程的健壮性、可用性、时序稳定性
- 通过 systemd unit 或命令行直接托管一个目标进程，避免应用自身崩溃导致服务不可用
- 支持管理这些类型的进程：`二进制可执行程序`、`sh脚本`
- 实现将进程对应到端口号，可通过 Restful API 实现获取日志、开关进程等操作
- 启动的进程可以配置独立的：程序路径、环境变量、启动参数、预处理脚本、是否自动重启、崩溃自动重启次数、重启退避基数与上限、是否立即启动、端口号、日志最大缓存行数、单行日志最大字节数、日志本地存储路径、日志本地存储周期等
- 自动重启采用指数退避（`min(base<<(n-1), max)`，与 DontCrack-Manager 对齐）并带稳定期重置：子进程一旦存活超过 30s，重启计数清零，因此偶发崩溃不会耗尽重试配额
- 优雅停止与探针收敛都向整个**进程组**发信号（Unix `SIGTERM` / Windows `CTRL_BREAK_EVENT`），仅在宽限期后仍未退出才强杀整棵进程树，不会留下孤儿孙进程
- 支持跨架构，免 CGO，支持任何可被 GO 编译器编译程序的架构使用 (amd64 / arm64 / armv7 等)
- 与 OpenHarmony、Android 版本共享同一套命令、HTTP 接口、文件结构

### 二、基础用法

```
./DontCrack \
  -path "/opt/DontCrack4ManyLinux/example/childproc/childproc" \
  -args "-mode normal -interval 500ms -lifetime 5s" \
  -env "EXTRA_INFO=from_manager RESTART_ENV_COUNT=0" \
  -file-log -log-path ./example/logs/ -log-life-day 7 \
  -auto-restart -max-retries 2 \
  -start-now \
  -password 123456
```

| 配置项                | 类型     | 默认值                      | 说明                                                            |
| ------------------ | ------ | ------------------------ | ------------------------------------------------------------- |
| path               | string | ""                       | 要管理的程序路径（支持可执行文件、shell脚本等）                                    |
| args               | string | ""                       | 传递给程序的参数（可选）                                                  |
| pre                | string | ""                       | 启动前要执行的命令（在 sh 中执行，可用&&/;/||连接多条命令，默认为空）                      |
| env                | string | ""                       | 为子进程追加环境变量，如: "PATH=/usr/local/bin:/usr/bin FOO=bar"；用空格或分号分隔 |
| auto-restart       | bool   | false                    | 是否自动重启                                                        |
| max-retries        | int    | 3                        | 最大重试次数（-1表示无限次，默认3次）                                          |
| restart-delay      | duration | 2s                     | 自动重启退避基数：第 n 次重启前等待 `min(base<<(n-1), restart-delay-max)`，首次重启等 base |
| restart-delay-max  | duration | 0                     | 退避等待上限（默认 0 即不封顶；不得小于 restart-delay）                           |
| start-now          | bool   | false                    | 是否立即启动                                                        |
| probe-cmd          | string | ""                       | 子进程健康检查命令（留空禁用；连续失败达阈值会**优雅停止并收敛整个进程组**）            |
| probe-interval     | int    | 30                       | 探针间隔（秒）                                                      |
| probe-timeout      | int    | 5                        | 探针超时（秒）                                                      |
| probe-failure-limit| int    | 3                        | 连续失败多少次判定为不健康（默认3）                                          |
| password           | string | ""                       | 管理进程的密码（可选，默认为空且不开启密码保护）                                      |
| listen-address     | string | 127.0.0.1                | HTTP 监听地址（默认仅本机；对外监听必须配置密码）                                 |
| port               | int    | 11883                    | HTTP服务端口                                                      |

> **探针联动注意（重要）**: 探针连续失败达阈值时，管理器会走与 `/shutdown` 完全相同的
> 停止路径：先向子进程所在**进程组**发优雅信号（Unix: `SIGTERM`；Windows:
> `CTRL_BREAK_EVENT`），5 秒内未退出再强杀**整棵进程树**（Unix: `SIGKILL` 至进程组；
> Windows: `taskkill /T /F`）。此后是否自动拉起取决于 `-auto-restart` ——
> **启用探针做健康自愈时请务必同时开启 `-auto-restart`**（`-max-retries -1` 可无限重试），
> 否则探针停止后子进程不会自动恢复，需手动调 `/startup`。若只是用探针做"检测并下线"，
> 可不开启。
| log-capacity       | int    | 200                      | 日志缓存的最大行数（默认200）                                              |
| log-max-line-bytes | int    | 1048576                  | 单行日志的最大字节数（用于bufio.Scanner，默认1MiB）                            |
| file-log           | bool   | false                    | 是否启用文件日志（默认false）                                             |
| log-path           | string | ./logs/proc_manager/    | 本地日志文件目录（默认 ./logs/proc_manager/，按进程名创建子目录）               |
| log-life-day       | int    | 7                        | 本地日志文件保存天数（默认7天，新日志写入时会清理过期文件）                                |

> **参数解析注意（重要）**: `-args` 的值若以 `-` 开头（如 `-mode normal`），Go flag 包会
> 把它吞并为后续 flag 的值，导致 `-port` 等后续配置错位，甚至使解析提前终止——现已
> fail-closed 拒绝启动并提示。请给 `-args` 传值加引号（`-args "-mode normal"`），或使用
> 等号形式（`-args=-mode\ normal`，对含空格/前导 `-` 的任何值都无歧义，程序化调用推荐）。


### 三、接口文档

> `/`

- 接口说明：根路径，返回纯文本横幅（`DontCrack_linux By FasterEdge`），可用于连通性探测
- 请求方式：GET
- 请求参数：无
- 返回类型：文本
- 返回示例：
  ```
  DontCrack_linux By FasterEdge
  ```

> `/ui`（及 `/ui/`）

- 接口说明：嵌入的单文件 HTML 控制台（`core/webui/index.html`，`go:embed` 编译进二进制），
  提供状态查看、启动/停止按钮与日志刷新，不依赖任何外部静态文件
- 请求方式：GET
- 请求参数：无
- 返回类型：HTML
- **鉴权说明（如实）**：`/ui` 与 `/ui/` 两个 handler **不做密码校验**，任何人都能取到这份
  静态页面。页面本身不含任何进程信息；页面内的数据请求（`/heartbeat`）与操作请求
  （`/startup`、`/shutdown`）仍然要过密码校验，因此配置了 `-password` 时，未持密码者
  只能看到一个空壳页面
- 说明：页面每 3 秒轮询一次 `/heartbeat`；URL 上带 `?password=` 仅在初始化时读入输入框，
  实际请求一律走 `Authorization: Bearer` 头，密码不会进入浏览器历史与访问日志

> `/healthz`

- 接口说明：健康检查（K8s `livenessProbe` / 负载均衡健康检查）。管理器自身存活由其 HTTP
  服务隐式保证（此端口能响应即管理器在运行）；只有当被托管的子进程正在运行时才返回 200
- 请求方式：GET
- 请求参数
  ```
  password: 密钥（可选params参数，配置了 -password 时为必填）
  ```
- 返回类型：文本
- 返回示例：
  ```
  ok
  ```
  子进程未运行时返回 `503` + `process not running`

> `/readyz`

- 接口说明：就绪检查（K8s `readinessProbe`）。**与 `/healthz` 语义完全相同、判定条件完全
  相同**（同样要求子进程正在运行才返回 200），拆成两个路径只是为了在 K8s 配置里可以把
  `livenessProbe` 与 `readinessProbe` 指向不同地址，便于分别调整探测参数。二者唯一的区别
  是响应文案：`/healthz` 返回 `ok`，`/readyz` 返回 `ready`
- 请求方式：GET
- 请求参数
  ```
  password: 密钥（可选params参数，配置了 -password 时为必填）
  ```
- 返回类型：文本
- 返回示例：
  ```
  ready
  ```
  子进程未运行时返回 `503` + `not ready`

> `/metrics`

- 接口说明：Prometheus 文本格式指标（`text/plain; version=0.0.4`），仅允许 GET，其它方法返回 405
- 请求方式：GET
- 请求参数
  ```
  password: 密钥（可选params参数，配置了 -password 时为必填）
  ```
- 返回类型：文本（Prometheus exposition format）
- 指标清单：

  | 指标名 | 类型 | 含义 |
  | --- | --- | --- |
  | `dontcrack_up` | gauge | 管理器是否在运行（恒为 1） |
  | `dontcrack_process_state` | gauge | 子进程运行状态，1=running，0=stopped |
  | `dontcrack_process_pid` | gauge | 子进程 PID（未运行时为 0） |
  | `dontcrack_restart_count` | counter | 累计自动重启次数 |
  | `dontcrack_last_exit_code` | gauge | 子进程上次退出码 |
  | `dontcrack_last_exit_time_seconds` | gauge | 子进程上次退出的 unix 时间戳（0 表示从未退出） |
  | `dontcrack_log_lines` | gauge | 内存中缓存的日志行数 |
  | `dontcrack_info` | gauge | 版本/平台信息，标签 `version`、`file_type`、`process_path` |

- 返回示例：
  ```
  # HELP dontcrack_up 管理器是否在运行(始终 1)
  # TYPE dontcrack_up gauge
  dontcrack_up 1
  # HELP dontcrack_process_state 子进程运行状态 1=running 0=stopped
  # TYPE dontcrack_process_state gauge
  dontcrack_process_state 0
  # HELP dontcrack_process_pid 子进程 PID
  # TYPE dontcrack_process_pid gauge
  dontcrack_process_pid 0
  # HELP dontcrack_restart_count 累计自动重启次数
  # TYPE dontcrack_restart_count counter
  dontcrack_restart_count 0
  # HELP dontcrack_last_exit_code 子进程上次退出码
  # TYPE dontcrack_last_exit_code gauge
  dontcrack_last_exit_code 0
  # HELP dontcrack_last_exit_time_seconds 子进程上次退出 unix 时间戳
  # TYPE dontcrack_last_exit_time_seconds gauge
  dontcrack_last_exit_time_seconds 0
  # HELP dontcrack_log_lines 内存中缓存的日志行数
  # TYPE dontcrack_log_lines gauge
  dontcrack_log_lines 0
  # HELP dontcrack_info 版本/平台信息
  # TYPE dontcrack_info gauge
  dontcrack_info{version="1.0.20261123",file_type="shell_script",process_path="/opt/.../childproc"} 1
  ```

> /startup

- 接口说明：启动进程，同时会重置重启次数
- 请求方式：POST
- 请求参数
  ```
  password: 密钥（可选params参数）
  ```
- 返回类型：文本
- 返回示例：
    ```
    ok
    ```

> /heartbeat

- 接口说明：获得心跳信息，会输出启动情况和缓存中的日志（同时会清除缓存）
- 请求方式：GET
- 请求参数
  ```
  password: 密钥（可选params参数）
  ```
- 返回类型：JSON
- 返回示例：
     ```
	{
	"version": "1.0.20261123",
	"state": "stopped",
	"info": "进程管理器正常运行",
	"timestamp": "2026-08-25 15:28:04",
	"logs": [
	"[STDERR] 2026/08/25 15:27:55.647714 env restart count -> 1",
	"[STDERR] 2026/08/25 15:27:55.648316 childproc start | pid=32054 | mode=normal | interval=1s | lifetime=5s | msg=",
	"[STDERR] 2026/08/25 15:27:55.648345 args: /opt/.../childproc -mode normal -interval 1s -lifetime 5s",
	"[STDERR] 2026/08/25 15:27:55.648352 env EXTRA_INFO=from_manager",
	"[STDERR] 2026/08/25 15:27:55.648353 env RESTART_ENV_COUNT=0",
	"[STDERR] 2026/08/25 15:27:56.668982 tick at 2026-08-25T15:27:56.64879725+08:00",
	"[STDERR] 2026/08/25 15:28:00.686300 lifetime reached, exiting normally"
	],
	"process_pid": 0,
	"process_path": "/opt/.../childproc",
	"restart_count": 0,
	"file_type": "binary_executable",
	"last_exit_time": "2026-08-25 15:28:00",
	"program_args": "-mode normal -interval 1s -lifetime 5s",
	"extra_env_raw": "PATH=/usr/local/bin EXTRA_INFO=from_manager RESTART_ENV_COUNT=0"
	}
	```

> /shutdown

- 接口说明：终止进程
- 请求方式：POST
- 请求参数
  ```
  password: 密钥（可选params参数）
  ```
- 返回类型：文本
- 返回示例：
  ```
  ok
  ```

### 四、细节说明

- 目标管理的进程的 Path 尽量使用全路径
- 标准 Linux 默认 shell 为 `/bin/sh`，本版直接使用，不再像 Android 版那样探测多种路径
- 运行的文件使用 .sh 结尾、首行包含 `#!` 都将被识别为脚本文件，由 `/bin/sh` 执行
- 开启密码时，接口鉴权凭据来源优先级：`Authorization: Bearer <密码>` 请求头 > `X-DontCrack-Password` 请求头 > URL `?password=` 查询参数（兼容保留，将弃用）；例如 `xxx/startup?password=123456`
- 与 OpenHarmony / Android 版的差异:
  - 启动横幅、根路径消息改为 `DontCrack_linux`
  - 不再探测 Android 默认 shell 路径，硬编码 `/bin/sh`
  - 默认日志目录改为相对路径 `./logs/proc_manager/` (而非绝对路径 `/data/...`)
  - 示例改为 `systemd` unit (`example/dontcrack-edgecore.service`)

#### 平台差异与移植（exec/ 的构建标签边界）

进程管理的公共逻辑（生命周期、代际校验、指数退避、稳定期重置、重启判定）只有**一份**
实现，位于 `exec/process.go`；真正跨平台的部分只有「如何对进程组发信号」，被抽到按
GOOS 分文件的平台层：

| 文件 | 构建标签 | 提供能力 |
| --- | --- | --- |
| `exec/process.go` | 无（公共） | `Process` 生命周期、`monitor`、`RestartPolicy.BackoffDelay`、`HealthProbe` |
| `exec/platform_unix.go` | `//go:build unix` | `applyLaunchGroup`(setpgid)、`terminateGroup`(SIGTERM/SIGKILL 至进程组) |
| `exec/platform_windows.go` | `//go:build windows` | `applyLaunchGroup`(CREATE_NEW_PROCESS_GROUP)、`terminateGroup`(CTRL_BREAK_EVENT / taskkill /T /F) |
| `exec/probe.go` | 无（公共） | 探针阈值判定与"优雅停止→强杀"处置，复用 `StopManagedProcess` |

平台层对外只有三个符号，**新增一个平台需要改的东西因此是一眼可见的**：

1. `applyLaunchGroup(cmd *exec.Cmd)` —— 在 `cmd.Start()` 之前把子进程纳入可收敛的进程组
   （Unix 用 `SysProcAttr.Setpgid=true`，Windows 用 `CreationFlags |= CREATE_NEW_PROCESS_GROUP`）；
2. `terminateGroup(pid *os.Process, force bool) error` —— `force=false` 发优雅信号，
   `force=true` 强杀并收敛整棵进程树；
3. `waitStatus` —— `syscall.WaitStatus` 的类型别名，供 `monitor` 判断"是否被信号终止"。

判定归属只需一条规则：**该平台的 GOOS 属于 Unix 家族（linux / android / darwin /
ios / *BSD / solaris / illumos …）就无需任何改动**，`platform_unix.go` 已覆盖；
否则新建 `exec/platform_<goos>.go` 实现上述三个符号即可。

> Windows 上的优雅信号是 `CTRL_BREAK_EVENT`，依赖子进程所在控制台组；
> 因此 `-pre`、探针命令等仍通过 `/bin/sh` 执行，在 Windows 下不可用 ——
> 本仓库的目标平台是 Linux，Windows 构建仅用于开发期交叉验证。

### 五、systemd 集成

将编译好的 `DontCrack` 放到 `/usr/local/bin/`，然后把示例 service 复制到 `/etc/systemd/system/`：

```bash
sudo cp example/dontcrack-edgecore.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dontcrack-edgecore.service
sudo systemctl status dontcrack-edgecore
```

完整示例见 `example/dontcrack-edgecore.service`，记得按实际情况调整 `ExecStart` 中的路径、端口、预处理脚本等参数。

### 六、使用技巧

- 单独使用时如果在会话中使用 & 作为命令结尾时一般会话结束的时候这个操作也会被杀死
- Go 语言程序的 log.Printf 默认将数据写到 os.Stderr，所以子进程中日志类型会显示为 [STDERR]，可以换成 fmt.Println 得到非 [STDERR] 的消息
- 因为可以通过 HTTP 带加密的形式操作进程，你可以根据此文档将进程操作结合系统角色，再结合 AI + MCP（或 Skills）完成各种操作
- 除了可以使用直接功能保证进程健壮性，还可以利用反复重启机制实现进程轮询，预先脚本也能实现延迟启动、等待依赖进程等操作
- `-restart-delay` 是退避基数：崩溃风暴时等待会指数增长（`2s → 4s → 8s …`），
  可用 `-restart-delay-max` 封顶，避免频繁重启把日志刷爆；子进程一旦存活超过 30s，
  重启计数会清零，因此"偶发崩溃"不会耗尽 `-max-retries` 配额

### 七、与其它平台仓的关系（如实说明）

DontCrack 家族在本地共有 **7 个仓库**：**5 个平台仓**（`DontCrack4ManyLinux` 本仓、
`DontCrack4Windows`、`DontCrack4Mac`、`DontCrack4OpenHarmonyLinuxKernelSide`、
`DontCrack4AndroidLinuxKernelSide`）+ 2 个非平台仓（`DontCrack-Manager`、`DontCrack-Desktop`）。
5 个平台仓目前仍是同一份代码的**五份独立拷贝**，而不是共享同一个模块。

**本次重构只落在本仓库内，没有同步到其余四个平台仓**——它们的代码仍是重构前的状态。
各仓最新提交（`git log -1 --date=format:'%Y-%m-%d %H:%M' --format='%cd | %h | %s'` 实测）：

| 仓库 | 最新提交 | 提交说明 |
| --- | --- | --- |
| `DontCrack4ManyLinux`（本仓） | 2026-11-22 17:35 · `823b7ec` | 本次重构：平台差异收敛到构建标签、重启退避与优雅探针 |
| `DontCrack-Manager` | 2026-11-22 17:35 · `c869c7c` | 探针语义分离、Windows 进程树收容与 e2e 接入 CI |
| `DontCrack4Windows` | 2026-11-16 19:48 · `1db7a62` | chore: update dependencies for v1.0.20261116 |
| `DontCrack4Mac` | 2026-11-16 19:48 · `20c5eb6` | chore: update dependencies for v1.0.20261116 |
| `DontCrack4OpenHarmonyLinuxKernelSide` | 2026-11-16 19:48 · `67e3c0f` | chore: update dependencies for v1.0.20261116 |
| `DontCrack4AndroidLinuxKernelSide` | 2026-11-16 19:47 · `157b343` | chore: update dependencies for v1.0.20261116 |
| `DontCrack-Desktop` | 2026-11-16 22:09 · `1b8f55c` | chore(deps): 更新 @tauri-apps/api 至 2.12.5 |

> 因此不能说"其余仓库未做任何修改"——四个平台仓在 2026-11-16 都各自做了依赖更新提交，
> 只是**没有**包含本次的退避/探针/进程组收敛重构。

各仓当前 `exec/process.go` 的 md5（`md5sum exec/process.go` 实测值，用于说明拷贝关系）：

| 仓库 | `exec/process.go` md5 |
| --- | --- |
| `DontCrack4ManyLinux`（本仓，已重构） | `a22250fa854f68a843fcf6ed6cf52b10` |
| `DontCrack4AndroidLinuxKernelSide` | `3cf063ca3e4621fa06d16cefdd6bdee6` |
| `DontCrack4OpenHarmonyLinuxKernelSide` | `3cf063ca3e4621fa06d16cefdd6bdee6`（与 Android 完全相同） |
| `DontCrack4Windows` | `1e098e0d985db99d4ca56b44b35a5493` |
| `DontCrack4Mac` | `aebf56a34622078fa8687b44fc18729d` |

本仓重构前 `exec/process.go` 的 md5 为 `69f19f353aa5c32c5b8f80957c54afd2`，重构后已变为上表
的 `a22250fa…`。

四仓 `exec/process_test.go` md5 相同（`2058cb89…`），只有本仓的测试因本次重构而不同
（本仓为 `ee93a139…`）。
真正的跨平台分支历史上只有 `StopManagedProcess` 里的信号差异（Unix `SIGTERM` /
Windows `os.Interrupt`）。

**待收敛清单（尚未完成，需在对应仓库各自改动并验证）**：

1. `DontCrack4AndroidLinuxKernelSide`：按本仓 `exec/platform_unix.go` 收敛信号路径；
   同时补上"探针优雅停止 + 进程组收敛"与"指数退避 + 稳定期重置"。
2. `DontCrack4OpenHarmonyLinuxKernelSide`：同上（注意其 release.yml 用 `GOOS=linux` 编译，
   天然落入 `//go:build unix`）。
3. `DontCrack4Windows`：按本仓 `exec/platform_windows.go` 把 `os.Interrupt` 换成
   `CTRL_BREAK_EVENT`，并把强杀从 `Process.Kill` 换成 `taskkill /T /F`。
4. `DontCrack4Mac`：与 Android 相同（Unix 家族，`platform_unix.go` 可直接覆盖）。
5. 中期目标：把 `exec/` 抽成独立 Go module（如 `github.com/FasterEdge/dontcrack-exec`），
   五个平台仓以依赖方式引用，从根上消除"五份拷贝"。这一步尚未开始，需要先确认各仓
   release 流水线与 go.mod 的改造窗口。

> 本次改动**没有**为了"看起来一致"而去同步修改其它四个平台仓——那会让没有验证过的代码
> 冒充已验证。本仓的验证结论（见下）只对本仓库成立。

### 八、本仓改动的验证方式

```bash
go vet ./...
go test ./...
go test -race ./...
```

覆盖本次新增行为的测试（`exec/` 包）：

- `backoff_test.go`：退避序列 `base, 2·base, 4·base`、上限封顶、左移溢出保护、
  历史默认 2s 回落、稳定期阈值恒为 30s
- `probe_test.go`：探针阈值判定（含 `limit<=0` fail-closed）、收敛必须标记
  `stoppedByReq` 并递增代际（否则 monitor 会把进程拉起来）、nil 防御
- `e2e_signal_test.go`：真实启动子进程验证优雅信号（子进程自行退出而非被强杀）、
  忽略信号后超时强杀、进程组收敛（孙进程同时收到信号）。测试子进程同时监听
  `os.Interrupt` 与 `SIGTERM`，因此在 Windows 与 Unix 下都等价成立
- `e2e_backoff_test.go`：由子进程启动时间戳反推退避间隔确为指数级、上限真的生效；
  存活 35s(>30s)后重启计数被清零

其中三个 e2e 用例经过变异验证（人为破坏实现后测试必须失败），确保断言不是恒真。
