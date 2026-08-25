<div align="center">
<img src="https://s2.loli.net/2025/10/30/whQl7sJryj1GbHU.png" style="width:100px;" width="100"/>
<h2>DontCrack4ManyLinux</h2>
<h3>通用 Linux (manylinux) 进程管理器</h3>
</div>


### 一、功能简介

- 通用 Linux 发行版 (manylinux) 下的进程管理器，用于提高后台/守护进程的健壮性、可用性、时序稳定性
- 通过 systemd unit 或命令行直接托管一个目标进程，避免应用自身崩溃导致服务不可用
- 支持管理这些类型的进程：`二进制可执行程序`、`sh脚本`
- 实现将进程对应到端口号，可通过 Restful API 实现获取日志、开关进程等操作
- 启动的进程可以配置独立的：程序路径、环境变量、启动参数、预处理脚本、是否自动重启、崩溃自动重启次数、是否立即启动、端口号、日志最大缓存行数、单行日志最大字节数、日志本地存储路径、日志本地存储周期等
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
| start-now          | bool   | false                    | 是否立即启动                                                        |
| password           | string | ""                       | 管理进程的密码（可选，默认为空且不开启密码保护）                                      |
| port               | int    | 11883                    | HTTP服务端口                                                      |
| log-capacity       | int    | 200                      | 日志缓存的最大行数（默认200）                                              |
| log-max-line-bytes | int    | 1048576                  | 单行日志的最大字节数（用于bufio.Scanner，默认1MiB）                            |
| file-log           | bool   | false                    | 是否启用文件日志（默认false）                                             |
| log-path           | string | ./logs/proc_manager/    | 本地日志文件目录（默认 ./logs/proc_manager/，按进程名创建子目录）               |
| log-life-day       | int    | 7                        | 本地日志文件保存天数（默认7天，新日志写入时会清理过期文件）                                |

### 三、接口文档

> /startup

- 接口说明：启动进程，同时会重置重启次数
- 请求方式：get、post
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
- 请求方式：get、post
- 请求参数
  ```
  password: 密钥（可选params参数）
  ```
- 返回类型：JSON
- 返回示例：
     ```
	{
	"version": "1.1.20260825",
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
- 请求方式：get、post
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
- 开启密码时，接口请求需要在 URL 参数中携带 `password` 参数，例如 `xxx/startup?password=123456`
- 与 OpenHarmony / Android 版的差异:
  - 启动横幅、根路径消息改为 `DontCrack_linux`
  - 不再探测 Android 默认 shell 路径，硬编码 `/bin/sh`
  - 默认日志目录改为相对路径 `./logs/proc_manager/` (而非绝对路径 `/data/...`)
  - 示例改为 `systemd` unit (`example/dontcrack-edgecore.service`)

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