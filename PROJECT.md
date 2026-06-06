---
name: dscli-project
description: "dscli v1.1.0 — Claude Code multi-backend proxy project overview, architecture, and key decisions"
metadata: 
  node_type: memory
  type: project
  originSessionId: 5ecb4139-38f3-419e-af32-d12b921fe3ad
---

# dscli — Claude Code 多后端代理

**仓库**: https://github.com/wdw8276/dscli
**版本**: v1.1.0
**语言**: Go (单二进制，跨平台，~7MB)

## 核心原理

完全参考 deepclaude 的思路，通过设置 `ANTHROPIC_BASE_URL` 劫持 Claude Code CLI 的 API 调用。
Claude Code 启动时 `settings.json env block > OS process environment`，用 `CCOverride` 临时 patch settings.json 强制指向代理。

## 启动方式选型

| 命令 | 代理 | Claude Code | settings.json patch | 场景 |
|------|------|-------------|---------------------|------|
| `dscli` (裸命令) | ✅ | ✅ | ✅ | 完整 workflow |
| `dscli launch` | ✅ | ✅ | ✅ | 同上 |
| `dscli proxy` | ✅ | ❌ | ❌ | 仅代理，给 cc/curl 用 |
| `dscli cc` / `dscli agent` | ❌ | ✅ | ✅ | 附加到已有代理 |

**Why**: 裸 `dscli` 默认为 `launch` 是最终决策——之前默认为 `proxy`，但用户体验更好的是直接启动 full workflow。

## 配置优先级链

```
settings.json env block (最高)
    ↓ 覆盖
进程环境变量 (os.Environ)
    ↓ 回退
dscli ResolveAPIKey 回退到 settings.json (仅 dscli 内部)
```

## 多 agent 同步方案

dscli cc 启动时先检测 settings.json 是否已被另一个 dscli patch——如果是，跳过 CCOverride (secondary 实例 no-op)。
第一个 dscli 负责写 bakPath 备份，最后退出时 restore 原始值。

## settings.json 备份/恢复

| 退出方式 | 恢复来源 | 机制 |
|---------|---------|------|
| 正常退出 | 内存 backup → defer restore() | CCOverride |
| kill -9 | 文件 settings.json.dscli-backup → recoverFromBackup() | 下次启动检测 |

**Why**: Go 进程无法捕获 SIGKILL，用磁盘备份兜底。

## 关键实现细节

- **端口冲突**: StartServerInBackground 用 net.ListenConfig{}.Listen() 预获取端口——比 ListenAndServe 早检测冲突
- **cost 持久化**: 30s 定时 batch flush，避免子 agent 并发时高频 disk IO
- **日志轮转**: 每次 output() 比较日期，自动切文件
- **模型名映射**: `[1m]` 后缀已去掉，雷火网关不支持

## 目录结构

~/.dscli/
├── config.yaml            # 配置文件（从 ~/.dscli.yaml 迁移）
├── .cost.json             # 成本持久化（跨重启累计）
├── .statusline-command    # 原始 statusLine 命令
└── logs/
    └── dscli-2026-06-05.log  # 按天轮转日志

## 配置文件示例

```yaml
active_backend: claude
proxy_port: 18799
backends:
  claude:
    type: anthropic
    base_url: https://ai.leihuo.netease.com
    api_key: sk-xxx
    pricing: { input: 3.0, output: 15.0 }
    models: { opus: claude-opus-4-8, sonnet: claude-sonnet-4-6, haiku: claude-haiku-4-5-20251001 }
  deepseek:
    type: deepseek
    base_url: https://ai.leihuo.netease.com
    api_key: sk-xxx
    pricing: { input: 0.44, output: 0.87 }
    models: { opus: deepseek-v4-pro, sonnet: deepseek-v4-pro, haiku: deepseek-v4-flash }
```

## 技术选型记录

- **Go** 而非 Rust/Node.js：单二进制 + 零运行时 + 极简交叉编译
- **goreleaser** 而非手动脚本：自动 5 平台构建 + GitHub Release
- **标准库 net/http** 而非 gin/echo：无框架依赖，~7MB 二进制
- **按天日志轮转** 而非按大小：更适合代理类长期运行


## Architecture

```
dscli/
├── cmd/dscli/
│   ├── main.go         # CLI: launch / proxy / agent / status / statusline / version
│   └── http.go         # HTTP helpers
├── pkg/
│   ├── config/
│   │   └── config.go   # YAML config + API key resolution + settings.json patch
│   ├── logx/
│   │   └── logx.go     # Leveled logging (debug/info/warn/error) + file output
│   └── proxy/
│       ├── server.go   # HTTP server + ProxyState (routing + hot-switch)
│       ├── backend.go  # Backend interface + Claude / DeepSeek implementations
│       ├── control.go  # /_proxy/status|mode|cost endpoints
│       ├── cost.go     # Per-backend token usage and cost tracking
│       └── sse.go      # SSE stream processing + usage normalization + thinking strip
├── Makefile
├── install.sh
└── .goreleaser.yaml
```

### Launch flow

```
dscli launch --backend deepseek
  ├── 1. Load ~/.dscli/config.yaml + ~/.claude/settings.json
  ├── 2. Resolve ${ANTHROPIC_API_KEY} (env → settings.json fallback)
  ├── 3. Patch ~/.claude/settings.json (inject dscli's ANTHROPIC_BASE_URL etc.)
  ├── 4. Start proxy → 127.0.0.1:18799
  ├── 5. Launch claude child process (with correct env vars)
  ├── 6. On exit: restore ~/.claude/settings.json to original
  └── 7. Shut down proxy
```
