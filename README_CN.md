[English](README.md) | 中文

# hi

**对 Claude Code 零侵入的多后端代理。** 不修改 Claude Code、不需要 fork、不依赖插件。hi 位于 Claude Code 和网络之间——仅拦截 API 调用——退出时恢复一切原样。你的 Claude Code 始终保持原生状态。

可连接**任意** Anthropic 兼容 API：Claude、DeepSeek、OpenRouter、内部网关和自定义端点。在同一个 session 中热切换所有后端，无需重启。

名称由来：每一次打开 Claude Code 或其他 AI agent 工具，第一句话都是从一句 **hi** 开始。AI 回复"你好，有什么可以帮助你的？"——然后工作就开始了。简单、好记、贴合 AI 时代。如有更好的名称也会更换。

## 快速开始

### 前置条件

- [Claude Code](https://code.claude.com/docs/en/quickstart#step-1-install-claude-code) 已安装并登录
- **hi 二进制文件** — 推荐从 [Releases](https://github.com/mars-base/hi/releases) 下载，或[从源码构建](https://github.com/mars-base/hi#building-from-source)
- **API key** — hi 兼容任意 Anthropic 兼容端点，你需要以下任一 API key：
  - [Anthropic Console](https://console.anthropic.com/) — Claude API key
  - [DeepSeek Platform](https://platform.deepseek.com/api_keys) — DeepSeek API key
  - 你所在组织的内部 API 网关 key

### 安装（Linux / macOS）

```bash
curl -fsSL https://raw.githubusercontent.com/mars-base/hi/main/install.sh | sh
```

以上命令会下载最新 release 二进制文件并安装到 `/usr/local/bin/hi`
（如果 `/usr/local/bin` 需要 `sudo`，则安装到 `~/.local/bin/hi`）。

### 安装（Windows）

**PowerShell：**

```powershell
irm https://raw.githubusercontent.com/mars-base/hi/main/install-windows.ps1 | iex
```

**CMD：**

```batch
curl -fsSL https://raw.githubusercontent.com/mars-base/hi/main/install-windows.cmd -o install.cmd && install.cmd && del install.cmd
```

安装到 `%USERPROFILE%\.local\bin\hi.exe` 并自动添加到用户 PATH。安装后重启终端即可使用。

### 运行

```bash
# 1. 初始化配置（自动检测 settings.json）
hi init-config

# 2. 编辑配置文件（Linux/macOS: ~/.hi/config.yaml, Windows: %USERPROFILE%/.hi/config.yaml）
#    自动检测的默认值够用的话可以跳过

# 3. 启动代理 + Claude Code（输入 /deepseek 或 /claude 即时切换后端）
hi

# 或不写配置文件，通过环境变量传入 API key
ANTHROPIC_API_KEY=sk-xxx DEEPSEEK_API_KEY=sk-xxx hi

# Windows PowerShell
$env:ANTHROPIC_API_KEY="sk-xxx"; $env:DEEPSEEK_API_KEY="sk-xxx"; hi

# Windows CMD
set ANTHROPIC_API_KEY=sk-xxx && set DEEPSEEK_API_KEY=sk-xxx && hi

# 将额外的 agent 附加到已运行的代理
hi cc

# 同上，显式指定
hi launch --backend deepseek

# 仅启动代理（不启动 Claude Code，不修改 settings.json）
hi proxy --log-file /tmp/hi.log

# 代理后台运行（Linux / macOS）
nohup hi proxy > /dev/null 2>&1 &

# 查看配置和状态
hi status
```

### 命令参考

| 命令 | 代理 | Claude Code | settings.json patch | Slash 命令 | 用途 |
|------|--------|--------|---------|--------|------|
| `hi` | ✅ | ✅ | ✅ | ✅ | 完整工作流（推荐） |
| `hi launch` | ✅ | ✅ | ✅ | ✅ | 同上 |
| `hi proxy` | ✅ | ❌ | ❌ | ✅ | 仅代理 |
| `hi agent` / `hi cc` | ❌ | ✅ | ✅ | ✅ | 附加到已有代理 |
| `hi status` | ❌ | ❌ | ❌ | ❌ | 查看配置 |
| `hi statusline` | ❌ | ❌ | ❌ | ❌ | Claude Code 状态栏 |

### CLI 选项

| 选项 | 默认值 | 说明 |
|------|--------|------|
| ``-b, --backend <name>`` | — | 后端: `claude` / `deepseek` |
| ``--log-file <path>`` | `~/.hi/logs/hi.log` | 日志文件路径（按天自动轮转） |
| ``--log-level <level>`` | `info` | 日志级别: `debug` / `info` / `warn` / `error` |
| `--preserve-statusline` | — | 保留已有的 statusLine 命令（不替换为 hi） |

日志文件按日期自动轮转，文件名在扩展名前加上当天日期：

```
~/.hi/logs/hi.log  →  ~/.hi/logs/hi-2026-06-05.log
/tmp/hi.log      →  /tmp/hi-2026-06-05.log
```

午夜时分日志自动切换到新文件——无需信号或重启。旧文件不会自动删除，需手动管理或通过 cron 清理。

### 查看日志

```bash
tail -f /tmp/hi-$(date +%F).log | grep -E "#[0-9]|Control:|backend |env:"
```

文中所有 `~/.hi/` 路径会自动解析为操作系统的主目录：

| 平台 | 路径 |
|------|------|
| Linux | `/home/user/.hi/` |
| macOS | `/Users/user/.hi/` |
| Windows | `C:\Users\user\.hi\` |

## 配置

首次运行 `hi status` 会自动生成 `~/.hi/config.yaml`：

```yaml
active_backend: deepseek
proxy_port: 18799

backends:
  claude:
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: "${ANTHROPIC_API_KEY}"
    pricing:
      input: 3.00
      output: 15.00
    models:
      opus: claude-opus-4-8
      sonnet: claude-sonnet-4-6
      haiku: claude-haiku-4-5-20251001

  deepseek:
    type: deepseek
    base_url: https://api.deepseek.com/anthropic
    api_key: "${DEEPSEEK_API_KEY}"
    strip_thinking: true     # 移除顶层 thinking 配置
    pricing:
      input: 0.42
      output: 0.83
    models:
      opus: deepseek-v4-pro
      sonnet: deepseek-v4-pro
      haiku: deepseek-v4-flash
```

`type: anthropic` 后端兼容任意 Anthropic 兼容 API——OpenRouter、OneAPI、内部网关、Azure 等。只需修改 `base_url` 和 `api_key`。详见[添加自定义后端](#添加自定义后端)。

## API Key 解析优先级

hi 在展开 `~/.hi/config.yaml` 中的 `${VAR}` 引用时，按以下顺序查找：

1. 操作系统环境变量 (`os.Getenv`)
2. `~/.claude/settings.json` 的 `env` 块（回退）
3. 非 `${...}` 格式 — 原样使用

> 这是 hi 自身的解析顺序。Claude Code 启动时的优先级正好相反——
> `settings.json env > OS process env`——所以 hi 必须 patch settings.json
> 才能将 API 流量重定向到代理。

启动日志会显示解析来源：

```
[hi] DEBUG env: ANTHROPIC_API_KEY=...KoKG (from OS env)
[hi] DEBUG env: ANTHROPIC_API_KEY=...KoKG (from ~/.claude/settings.json)
[hi] DEBUG env: DEEPSEEK_API_KEY=<not set>
```

### 模型名称映射

Claude Code 使用 Anthropic 原生的模型名称。hi 自动将其重映射：

| Claude 模型 | deepseek 后端 | claude 后端 |
|-------------|---------------|-------------|
| `claude-opus-4-8` | `deepseek-v4-pro` | `claude-opus-4-8` |
| `claude-sonnet-4-6` | `deepseek-v4-pro` | `claude-sonnet-4-6` |
| `claude-haiku-4-5-20251001` | `deepseek-v4-flash` | `claude-haiku-4-5-20251001` |

### 添加自定义后端

可以在 `~/.hi/config.yaml` 的 `backends` 下添加任意 Anthropic 兼容 API 端点，然后重启 hi：

```yaml
backends:
  claude:
    type: anthropic
    base_url: https://api.anthropic.com
    api_key: "${ANTHROPIC_API_KEY}"
    pricing: { input: 3.00, output: 15.00 }
    models:
      opus: claude-opus-4-8
      sonnet: claude-sonnet-4-6
      haiku: claude-haiku-4-5-20251001

  deepseek:
    type: deepseek
    base_url: https://api.deepseek.com/anthropic
    api_key: "${DEEPSEEK_API_KEY}"
    strip_thinking: true
    pricing: { input: 0.42, output: 0.83 }
    models:
      opus: deepseek-v4-pro
      sonnet: deepseek-v4-pro
      haiku: deepseek-v4-flash

  # 示例：通过内部网关自定义后端
  internal:
    type: anthropic
    base_url: https://llm.internal.example.com
    api_key: "${INTERNAL_API_KEY}"
    strip_thinking: true      # 如果网关强制 thinking 一致性，开启此项
    pricing: { input: 0.50, output: 1.00 }
    models:
      opus: claude-opus-4-8
      sonnet: claude-sonnet-4-6
      haiku: claude-haiku-4-5-20251001
```

要点：
- `type: anthropic` — 适用于 Anthropic 兼容的 API 端点
- `type: deepseek` — 转发前剥离顶层 `thinking` 配置（避免 `reasoning_effort` 兼容性错误），保留 content 级 thinking 块
- `strip_thinking` — `true` / `false`，覆盖按类型的默认值（`deepseek` 默认 `true`，`anthropic` 默认 `false`）
- `api_key` — 支持 `${ENV_VAR}` 展开或直接填写 key
- `models.opus/sonnet/haiku` — 将 Claude 模型名映射到后端实际模型 ID
- `pricing` — 每百万 token 的 USD 价格，用于成本追踪

> **DeepSeek 1M 上下文窗口**：使用 DeepSeek 官方 API 时，在模型名称后追加
> ``[1m]`` 即可解锁 1M token 上下文窗口。在 `models` 块中写入
> ``deepseek-v4-pro[1m]`` 和 ``deepseek-v4-flash[1m]``。如果你的 API 网关
> 拒绝了 ``[1m]`` 后缀，换用不带后缀的名称（``deepseek-v4-pro``、
> ``deepseek-v4-flash``）即可。

## 工作原理

hi 在 Claude Code 和 API 端点之间运行一个本地代理，拦截所有请求并路由到配置的后端：

```
Claude Code CLI（工具、文件编辑、bash — 完全不变）
  └── 所有 API 调用 → hi 代理 (localhost:18799)
                        ├── 注入后端认证
                        ├── 自动重映射模型名
                        └── 路由 → 任意 Anthropic 兼容后端（Claude、DeepSeek、OpenRouter、自定义……）
```

启动时，hi 临时 patch `~/.claude/settings.json` 将 Claude Code 指向本地代理。文件在退出时恢复——完全透明。

在 `~/.hi/config.yaml` 中定义任意数量的后端——每个后端都可通过 `/_proxy/mode` 或 slash 命令热切换。

环境变量优先级（Claude Code 启动时）：

```
settings.json env block > OS process environment
```

**崩溃恢复**：如果 hi 被强制终止（`kill -9`、断电），被 patch 的 `settings.json` 会残留。下次启动时，hi 检测到 `~/.claude/settings.json.hi-backup` 中的过期备份，自动恢复原始文件。无需手动修复。

**端口冲突**：同一时间只能有一个 hi 代理在端口 `18799` 上运行。如果启动第二个实例，它会打印清晰的错误并退出：

```
hi: Proxy startup failed: port :18799 already in use —
another hi proxy is running. Use 'hi cc' or 'hi agent' to attach instead
```

使用 `hi cc` 将额外的 Claude Code 会话附加到已有的代理。

### 独立代理模式

运行 `hi proxy` 仅启动代理服务器——不启动 Claude Code。适用于测试后端连通性或为多个 agent 提供服务：

```bash
# 终端 1：启动代理
hi proxy

# 终端 2：附加 agent
hi cc
hi cc --backend claude

# 或直接用 curl 测试
curl -s http://127.0.0.1:18799/_proxy/status | python3 -m json.tool
```

## 热切换后端

在会话中切换后端，无需重启 Claude Code。切换仅影响下一次 API 调用。

在 Claude Code 中输入：

```
切换到 DeepSeek：
! curl -sX POST http://127.0.0.1:18799/_proxy/mode -d 'backend=deepseek'

切换到 Claude：
! curl -sX POST http://127.0.0.1:18799/_proxy/mode -d 'backend=claude'
```

或创建 slash 命令 `~/.claude/commands/deepseek.md`：

```markdown
将代理切换到 DeepSeek。静默执行并报告结果：
curl -sX POST http://127.0.0.1:18799/_proxy/mode -d 'backend=deepseek'
如果返回 HTTP 200，说"已切换到 DeepSeek"。否则报告错误。
```

然后在任意 Claude Code 会话中输入 `/deepseek` 即时切换。

### 切换日志输出

```
[hi] INFO  Control: backend switched deepseek → claude
[hi] INFO  New backend env:
[hi] INFO    ANTHROPIC_MODEL                = claude-sonnet-4-6
[hi] INFO    ANTHROPIC_DEFAULT_OPUS_MODEL   = claude-opus-4-8
[hi] INFO    ANTHROPIC_DEFAULT_SONNET_MODEL = claude-sonnet-4-6
[hi] INFO    ANTHROPIC_DEFAULT_HAIKU_MODEL  = claude-haiku-4-5-20251001
```

## 多 agent 工作流

单个 `hi proxy` 实例可同时为多个 Claude Code agent 提供服务。每个 agent 的 API 调用经过同一个代理，共享成本追踪和后端切换。

有两种方式运行多 agent：

#### 方式一：`hi proxy` + `hi cc`（推荐）

先启动代理，然后用零配置方式附加 agent：

```bash
# 终端 1：启动代理
hi proxy

# 终端 2+：附加 agent
hi cc
hi cc --backend claude
```

#### 方式二：`hi launch` + 裸 `claude`

`hi launch` 在启动时 patch `settings.json`。一旦 patch 完成，任何裸 `claude` 命令都会自动拾取代理地址：

```bash
# 终端 1：完整启动（代理 + agent + settings patch）
hi launch --backend deepseek

# 终端 2+：直接 claude — settings.json 已指向代理
claude
claude
```

**注意**：`hi launch` 在其 Claude Code 退出时会关闭代理，导致所有共享的 agent 断连。如果需要代理独立存活，或需要以任意顺序启动 agent，请使用方式一。

所有 agent 共享同一个热切换端点——通过 `/_proxy/mode` 切换后端会立即影响所有连接的 agent。成本追踪跨所有会话汇总到 `~/.hi/.cost.json`。

## 状态行实时模型更新

hi 自动发现 `~/.claude/settings.json` 中已有的 `statusLine` 配置，并替换为内置的 `hi statusline` 命令。切换后端后，状态栏中的模型名会在 120s 内自动更新。无需手动配置。

传递 `--preserve-statusline` 可跳过此替换，保留原有的 statusLine 命令：

```bash
hi launch --preserve-statusline
hi cc --preserve-statusline
```

### 工作原理

```
hi launch
  ├── 发现 settings.json statusLine.command
  ├── 保存原始命令到 ~/.hi/.statusline-command
  └── 替换为 hi statusline

Claude Code 状态栏刷新（每 120s）
  ├── 运行 hi statusline
  ├── 查询代理 → 获取当前后端模型
  ├── 替换 stdin JSON 中的 model 字段
  └── 委托原始脚本渲染其余部分（费用、上下文、缓存统计）
```

### 效果

| 后端 | 状态栏显示 |
|------|------|
| deepseek | `🤖 deepseek-v4-pro` |
| claude | `🤖 claude-sonnet-4-6` |

### 手动测试

```bash
echo '{"model":{},"workspace":{},"context_window":{},"cost":{}}' | hi statusline
# 📁 tmp | 🤖 deepseek-v4-pro | 🧠 ctx:-- | 💰 $0.010
```

## 控制端点

```bash
# 状态查询
curl -s http://127.0.0.1:18799/_proxy/status | python3 -m json.tool
# {
#   "active_backend": "deepseek",
#   "backends": ["claude", "deepseek"],
#   "requests": 47,
#   "uptime_seconds": 3600
# }

# 成本追踪
curl -s http://127.0.0.1:18799/_proxy/cost | python3 -m json.tool
# {
#   "backends": {
#     "deepseek": {
#       "input_tokens": 23223,
#       "output_tokens": 166,
#       "cost": 0.0104,
#       "anthropic_equivalent": 0.0722
#     }
#   },
#   "total_cost": 0.0104,
#   "anthropic_equivalent": 0.0722,
#   "savings": 0.0618
# }

# 切换后端
curl -sX POST http://127.0.0.1:18799/_proxy/mode -d 'backend=deepseek'
# {"mode":"deepseek","previous":"claude"}
```

### 成本持久化

成本数据在代理重启后仍然保留。token 用量在内存中累积，按固定频率落盘：

| 触发条件 | 行为 |
|------|------|
| 每次请求 `Record()` | 仅更新内存（不写磁盘） |
| 后台 goroutine | 每 30s 落盘到 `~/.hi/.cost.json` |
| 代理关闭 | 通过 `Close()` 最终落盘 |
| 下次启动 | 从文件加载累积数据 |

这种设计避免了大量子 agent 并发工作时造成过度的磁盘 I/O，同时确保数据在进程重启和系统重启后不丢失。

```bash
# 查看跨所有会话的累计成本
cat ~/.hi/.cost.json | python3 -m json.tool
```

## 调试

### 日志级别

| 级别 | 显示内容 | 用途 |
|------|------|----------|
| `debug` | 全部（env 解析、model remap、upstream URL） | 排查问题 |
| `info` | 请求追踪、后端切换、成本统计（默认） | 日常使用 |
| `warn` | 警告 + 错误 | 关注异常 |
| `error` | 仅错误 | 极简模式 |

### 请求日志示例（debug 级别）

```
[hi] INFO  15:07:03 #2 POST deepseek /v1/messages 200 634ms
[hi] DEBUG 15:07:03   -> upstream: POST https://api.anthropic.com/v1/messages
[hi] DEBUG 15:07:03   <- status=200 content-type=application/json
[hi] DEBUG 15:07:03 Model remap: claude-sonnet-4-6 → deepseek-v4-pro
```

### hi status 输出

```
Config:  /home/fish/.hi.yaml
Claude:  detected at /home/fish/.claude/settings.json
Proxy:   http://127.0.0.1:18799
Active:  deepseek
Log:     /tmp/hi.log (level=info)
Backends:
    claude
  * deepseek
```

## 从源码构建

需要 [Go 1.25+](https://go.dev/dl/)。

```bash
git clone https://github.com/mars-base/hi.git
cd hi
make build
make install
```

## 成本对比

| 后端 | 输入/M | 输出/M |
|------|--------|--------|
| Claude Opus | $3.00 | $15.00 |
| DeepSeek V4 | $0.42 | $0.83 |

重度使用（25天/月）：$30-80 vs $200——节省 60-85%。

代理实时追踪并报告节省金额：

```
total_cost: 0.0104              ← 实际花费
anthropic_equivalent: 0.0722    ← 等量 Anthropic 花费
savings: 0.0618                 ← 节省 85.6%
```

## License

MIT
