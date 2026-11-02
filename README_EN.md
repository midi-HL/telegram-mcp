# Telegram MCP

[中文](README.md) | [English](README_EN.md) | [日本語](README_JA.md) | [Русский](README_RU.md)

Give AI agents full control of a real Telegram account via [MCP](https://modelcontextprotocol.io) — send messages, manage groups and channels, view user profiles, operate bots, transcribe voice.

Not the Bot API — a **complete MTProto client**: read personal chat history, manage bots, do things the Bot API cannot.

## Features

| Feature | Description |
|---|---|
| **Pure Go, zero cgo** | Single binary for Windows / Linux / macOS / Termux |
| **Token-efficient** | 12 aggregated tools (8k tokens) instead of 568 fine-grained tools (24k+) |
| **Response projection** | `chat list(50)` costs 458 tokens (full fields would be 133k, 99.7% savings) |
| **Auto reply expansion** | Quoted images/files/voice are annotated and downloadable |
| **Trust levels** | Admin messages unwrapped; external content wrapped in `<untrusted_content>` |
| **Web UI** | LAN-accessible, password-only login, embedded with zero dependencies |

## Quick Start

### 1. Get Telegram API credentials

Visit https://my.telegram.org → API development tools → create an app. You'll get `api_id` and `api_hash`.

### 2. Download

From [Releases](../../releases), pick your platform:

| Platform | File |
|---|---|
| Windows x64 | `tgmcp-windows-x86_64.zip` |
| Windows ARM64 | `tgmcp-windows-arm64.zip` |
| Linux x64 | `tgmcp-linux-x86_64.tar.gz` |
| Linux ARM64 | `tgmcp-linux-arm64.tar.gz` |
| Linux ARMv6/v7 | `tgmcp-linux-armv6.tar.gz` / `armv7` |
| Linux RISC-V 64 | `tgmcp-linux-riscv64.tar.gz` |
| Linux LoongArch 64 | `tgmcp-linux-loong64.tar.gz` |
| macOS Apple Silicon | `tgmcp-darwin-arm64.tar.gz` |
| macOS Intel | `tgmcp-darwin-x86_64.tar.gz` |
| FreeBSD | `tgmcp-freebsd-x86_64.tar.gz` / `arm64` |

Debian/Ubuntu: `.deb` packages available. RHEL/Fedora: `.rpm` packages available.

### 3. Log in

```bash
# QR scan (recommended)
./tgmcp auth --account main --qr

# Or phone number
./tgmcp auth --account main --phone +8613800138000
```

### 4. Configure MCP client

**Claude Code** (`~/.claude.json` or project `.mcp.json`):

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

**Cursor** (`~/.cursor/mcp.json`): same format.

**VS Code** (`.vscode/mcp.json`):

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

### 5. Use

Just talk to your AI:

- "List all my groups with unread counts"
- "Read the last 20 messages in group X"
- "Forward this message to another group"
- "Send a message to @someone saying hi"
- "Show me @Eddie's profile"
- "Use @some_bot to claim today's check-in"

## Web UI

Access `http://<your-ip>:8080/ui` after starting the service.

| Page | Function |
|---|---|
| Accounts | View account status, cache stats |
| Chat | Conversation list + messages + manual send |
| Auto-reply | Rule configuration + toggle + logs |
| Admins | Add/remove admins (stores numeric ID) |
| Settings | Change password, service info |

**Default password `Admin`** — change it immediately after first login.

## Commands

```bash
tgmcp auth      Login (--qr scan / --phone)
tgmcp serve     Start MCP service (--http for HTTP transport)
tgmcp doctor    Environment check
tgmcp asr       Voice transcription config (setup / status / config)
tgmcp version   Version
```

## 12 Tools

| Tool | Capability |
|---|---|
| `system` | Service status, account list, environment check |
| `chat` | Conversation list, details, unread, mark read, folders |
| `message` | Send/edit/delete/forward/search/history/react/pin + **quote expansion** |
| `group` | Members, invite/kick, admins, permissions, approval, join/leave, comments |
| `contact` | User profile page, contacts, search, add, block |
| `media` | Send photo/file/video/voice/album, download, **on-demand image view** |
| `sticker` | Sticker sets, send stickers, custom emoji, reactions |
| `account` | Own profile, change name/bio/avatar/username, sessions |
| `bot` | Manage own bots, inline queries |
| `botuse` | Use others' bots (read buttons, click callbacks, WebApp) |
| `asr` | Voice-to-text (4 backends) |
| `stats` | Group stats, chat stats, cache stats |

## Skill (API docs for the model)

`skills/telegram-mcp/` is a Claude Code Skill that lets the model load API details on demand:

```
SKILL.md (~2k tokens, always-on routing table)
references/
  ├── message.md    Send, quotes, history
  ├── chat.md       Conversation management
  ├── group.md      Group management, bot verification
  ├── channel.md    Channels, comments
  ├── contact.md    User profile page
  ├── media.md      Media send/receive
  ├── sticker.md    Stickers
  ├── bot.md        Bot management
  └── asr.md        Voice transcription, stats
```

Install: `cp -r skills/telegram-mcp ~/.claude/skills/`

## Custom ASR Paths

If you already have sherpa-onnx installed, point to your existing paths:

```bash
# Environment variables (highest priority)
export TGMCP_ASR_MODEL_DIR=/opt/sherpa/models/paraformer-zh-small
export TGMCP_ASR_CLI=/usr/local/bin/sherpa-onnx-offline

# Or via config
tgmcp asr config --cli /usr/local/bin/sherpa-onnx --model-dir /opt/sherpa/models/...

# Or persistent config
tgmcp asr setup   # skips download if model already exists
tgmcp asr status  # shows resolved paths
```

**Priority**: environment variables > `config.json` > default `{DataDir}/models/...`

Model files recognized: `model.onnx`, `model.int8.onnx`, `encoder.onnx`, `encoder.int8.onnx`, `paraformer.onnx`.

## Configuration

Data directory defaults to `~/.tgmcp`, override with `TGMCP_DATA_DIR`.

```
~/.tgmcp/
├── config.json          # Config
├── cache.db             # SQLite cache (chats/messages/users/admins)
├── sessions/
│   └── main/
│       └── session.json # Telegram session (★ contains auth key, keep secret)
├── models/              # ASR models
└── downloads/           # Media downloads
```

## Security Notes

- **Session file is plaintext JSON** (contains auth key). Leak = account compromise.
- **Admins identified by numeric UserID** — usernames can be changed/squatted, so never use them as the primary key.
- **Web UI default password `Admin`** binds to `0.0.0.0`, meaning anyone on the LAN can access it. Change the password immediately.
- **Admin messages bypass injection defense** — if an admin account is compromised, the model is compromised.

## Development

```bash
go vet ./...
go test ./...
go build -o bin/tgmcp ./cmd/tgmcp
```

All release binaries are built by GitHub Actions (`CGO_ENABLED=0` cross-compilation).

## Platform Support

| OS | Architectures | Notes |
|---|---|---|
| Linux | x86_64, i386, arm64, armv6, armv7, riscv64, loong64, mipsle, mips64, mips64le, ppc64, ppc64le, s390x | Full support |
| Windows | x86_64, i386, arm64 | Full support |
| macOS | x86_64, arm64 | Full support |
| FreeBSD | x86_64, i386, arm64 | Full support |
| NetBSD | x86_64, arm64 | Full support |
| OpenBSD | x86_64 | Full support |
| **linux/mips (big-endian 32-bit)** | — | ❌ **Not supported** (`modernc.org/libc` has no mips build) |

## License

**PolyForm Noncommercial 1.0.0** with additional terms:

- ✅ Personal use (any purpose, including commercial use of your own account)
- ✅ Internal use in noncommercial organizations
- ✅ Fork and modify for your own use
- ✅ Share unmodified copies free of charge
- ✅ Include in free noncommercial open-source projects
- ❌ **Sell, rent, or charge fees** for the software or as a service
- ❌ **Remove attribution** (author name + original repo URL must be retained)

See [LICENSE](LICENSE) for full terms. For commercial licensing, contact the author.

---

*Copyright 2026 Edwin (辰星) — https://github.com/midi-HL/telegram-mcp*
