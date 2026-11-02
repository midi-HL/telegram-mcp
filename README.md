# Telegram MCP

通过 [MCP](https://modelcontextprotocol.io) 让 AI 操作真实的 Telegram 账号——收发消息、管理群组频道、查看用户资料、操作 Bot、语音转写。

不是 Bot API，是**完整的 MTProto 客户端**：能读个人聊天历史、能管理 Bot、能做 Bot API 做不到的事。

## 特点

| 特性 | 说明 |
|---|---|
| **纯 Go，零 cgo** | 单个可执行文件，Windows / Linux / macOS / 安卓 Termux 通吃 |
| **token 高效** | 12 个聚合工具（8k tokens）而非 568 个细粒度工具（24k+） |
| **返回体投影** | `chat list(50)` 仅 458 tokens（全字段是 133k，省 99.7%） |
| **引用自动展开** | 被引用的图片/文件/语音自动标注，可直接下载 |
| **信任分级** | 管理员消息不设防，外部内容包 `<untrusted_content>` |
| **Web UI** | 局域网可访问，纯密码登录，内嵌无依赖 |

## 快速开始

### 1. 获取 Telegram API 凭据

访问 https://my.telegram.org → API development tools → 创建应用，拿到 `api_id` 和 `api_hash`。

### 2. 下载

从 [Releases](../../releases) 下载对应平台：

| 平台 | 文件 |
|---|---|
| Windows | `tgmcp-windows-amd64.exe` |
| Linux | `tgmcp-linux-amd64` |
| Linux ARM64（Termux/proot） | `tgmcp-linux-arm64` |
| macOS Apple Silicon | `tgmcp-darwin-arm64` |
| macOS Intel | `tgmcp-darwin-amd64` |

### 3. 登录

```bash
# 扫码登录（推荐）
./tgmcp auth --account main --qr

# 或手机号登录
./tgmcp auth --account main --phone +8613800138000
```

### 4. 配置 MCP 客户端

**Claude Code**（`~/.claude.json` 或项目 `.mcp.json`）：

```json
{
  "mcpServers": {
    "telegram": {
      "command": "/path/to/tgmcp",
      "args": ["serve"],
      "env": {
        "TGMCP_DATA_DIR": "/path/to/data"
      }
    }
  }
}
```

**Cursor**（`~/.cursor/mcp.json`）：同上格式。

**VS Code**（`.vscode/mcp.json`）：

```json
{
  "servers": {
    "telegram": {
      "type": "stdio",
      "command": "/path/to/tgmcp",
      "args": ["serve"]
    }
  }
}
```

### 5. 使用

在 AI 对话里直接说：

- "列出我所有群，显示未读数"
- "读一下 XX 群最近 20 条消息"
- "把这条消息转发到另一个群"
- "给 @someone 发条消息说你好"
- "把 @Eddie 的资料页给我看看"
- "用 @some_bot 领一下今天的签到"

## Web UI

服务启动后访问 `http://<你的IP>:8080/ui`。

| 页面 | 功能 |
|---|---|
| 账号 | 查看账号状态、缓存统计 |
| 聊天 | 会话列表 + 消息 + 手动发送 |
| 自动回复 | 规则配置 + 开关 + 日志 |
| 管理员 | 添加/删除管理员（存数字 ID） |
| 设置 | 修改密码、服务信息 |

**默认密码 `Admin`**，登录后请立即在设置页修改。

## 命令

```bash
tgmcp auth      登录（--qr 扫码 / --phone 手机号）
tgmcp serve     启动 MCP 服务（--http 用 HTTP 传输）
tgmcp doctor    环境自检
tgmcp asr       语音转写配置（setup / status / config）
tgmcp version   版本
```

## 12 个工具

| 工具 | 能力 |
|---|---|
| `system` | 服务状态、账号列表、环境自检 |
| `chat` | 会话列表、详情、未读、已读、文件夹 |
| `message` | 收发/编辑/删除/转发/搜索/历史/反应/置顶 + **引用展开** |
| `group` | 成员、拉人踢人、管理员、权限、审批、加入退出、评论区 |
| `contact` | 用户资料页、联系人、搜索、添加、拉黑 |
| `media` | 发图/文件/视频/语音/相册、下载、**按需看图** |
| `sticker` | 贴纸包、发贴纸、自定义 emoji、反应 |
| `account` | 自己资料、改名简介头像用户名、登录设备 |
| `bot` | 管理自己的 Bot、内联查询 |
| `botuse` | 使用别人的 Bot（读按钮、点回调、WebApp） |
| `asr` | 语音转文字（4 种后端） |
| `stats` | 群统计、会话统计、缓存统计 |

## Skill（给模型看的 API 文档）

`skills/telegram-mcp/` 是 Claude Code Skill，让模型按需加载 API 细节：

```
SKILL.md（~2k tokens，常驻路由表）
references/
  ├── message.md    发消息、引用、历史
  ├── chat.md       会话管理
  ├── group.md      群管理、Bot 验证
  ├── channel.md    频道、评论区
  ├── contact.md    用户资料页
  ├── media.md      媒体收发
  ├── sticker.md    贴纸
  ├── bot.md        Bot 管理
  └── asr.md        语音转写、统计
```

安装：`cp -r skills/telegram-mcp ~/.claude/skills/`

## Termux / proot 安装

```bash
# 方式 1：proot 容器（推荐）
proot-distro login ubuntu
apt update && apt install -y wget
wget https://github.com/<owner>/telegram-mcp/releases/latest/download/tgmcp-linux-arm64
chmod +x tgmcp-linux-arm64
mv tgmcp-linux-arm64 /usr/local/bin/tgmcp
tgmcp doctor

# 方式 2：原生 Termux（静态二进制直接跑）
pkg install wget
wget https://github.com/<owner>/telegram-mcp/releases/latest/download/tgmcp-linux-arm64
chmod +x tgmcp-linux-arm64
./tgmcp-linux-arm64 doctor
```

**注意**：安卓上跑 Telegram 客户端会触发风控，建议用小号。

## 平台支持

| OS | 架构 | 状态 |
|---|---|---|
| Linux | x86_64, i386, armv6, armv7, arm64, loong64, riscv64, mipsle, mips64, mips64le, s390x | ✅ 完整支持 |
| Windows | x86_64, i386, arm64 | ✅ 完整支持 |
| macOS | x86_64, arm64 | ✅ 完整支持 |
| FreeBSD | x86_64, i386, arm64 | ✅ 完整支持 |
| NetBSD | x86_64, arm64 | ✅ 完整支持 |
| OpenBSD | x86_64 | ✅ 完整支持 |
| **linux/mips**（大端 32 位） | — | ❌ **不支持**：`modernc.org/libc` 无该架构实现 |
| **linux/ppc64**（大端 64 位） | — | ⚠️ **待验证**：同样依赖 `modernc.org/libc` 的大端支持 |

> 依赖 `modernc.org/sqlite`（纯 Go SQLite），它通过 `modernc.org/libc` 实现 C 库。
> 该项目**只实现了小端架构**，因此所有大端平台（mips、ppc64）都无法编译。
> 这是依赖的硬限制，不是本项目的问题。

## 语音转写（可选）

```bash
# 下载模型（74MB）
tgmcp asr setup

# 下载 sherpa-onnx CLI 后指定路径
tgmcp asr config --cli /path/to/sherpa-onnx
```

或使用云端 API：

```bash
tgmcp asr config --backend openai --api-key sk-xxx
```

## 配置

数据目录默认 `~/.tgmcp`，可用 `TGMCP_DATA_DIR` 环境变量覆盖。

```
~/.tgmcp/
├── config.json          # 配置
├── cache.db             # SQLite 缓存（聊天/消息/用户/管理员）
├── sessions/
│   └── main/
│       └── session.json # Telegram session（★ 含 auth key，勿泄露）
├── models/              # ASR 模型
└── downloads/           # 媒体下载
```

## 安全说明

- **Session 文件是明文 JSON**（含 auth key）。泄露等于账号失守。
- **管理员按数字 UserID 识别**——用户名可被修改/抢注，所以不存用户名做主键。
- **Web UI 默认密码 `Admin`**，绑 `0.0.0.0` 意味着局域网内谁都能访问，请立即改密码。
- **管理员消息不做注入防御**——管理员账号被盗等于模型被操控。

## 开发

```bash
go vet ./...
go test ./...
go build -o bin/tgmcp ./cmd/tgmcp
```

编译产物全部由 GitHub Actions 生成（`CGO_ENABLED=0` 交叉编译）。

## 许可证

MIT
