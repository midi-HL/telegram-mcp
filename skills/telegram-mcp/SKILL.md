---
name: telegram-mcp
description: 通过 MCP 操作 Telegram 个人账号——收发消息、管理群组频道、查看用户资料、管理 Bot。当用户要求发 Telegram 消息、读聊天记录、管理群成员、查用户资料、加入频道、操作 Telegram Bot 时使用。
---

# Telegram MCP

通过 MCP 服务操作一个真实的 Telegram 账号（非 Bot API，是完整客户端能力）。

## 能力域（需要细节时读对应文件）

| 你想做什么 | 读哪个文件 |
|---|---|
| 发消息、改消息、删消息、转发、搜索、看历史 | `references/message.md` |
| 会话列表、未读数、标记已读、聊天文件夹 | `references/chat.md` |
| 群成员、拉人、踢人、设管理员、邀请链接、入群审批 | `references/group.md` |
| 加入频道/群、退出、评论区读写 | `references/channel.md` |
| 用户资料页（头像/简介/生日/状态/个人频道） | `references/contact.md` |
| 发图片/文件/视频/语音、下载媒体 | `references/media.md` |
| 贴纸、表情反应、自定义 emoji | `references/sticker.md` |
| 管理自己的 Bot、使用别人的 Bot | `references/bot.md` |
| 语音转文字、群统计 | `references/asr.md` |

## 通用约定（始终有效）

### 聊天标识 `chat` 参数

以下形式都支持：
- 数字 ID：`123456789`（用户）、`-1001234567890`（频道/超级群）
- 用户名：`@username`（不带 @ 也行）
- 链接：`t.me/username`、`https://t.me/+invitehash`
- 特殊值：`me` / `self`（自己）

### 返回体默认精简

所有查询默认只返回关键字段。要完整数据需显式传：

```json
{"expand": ["full"]}
```

`chat list` 返回 5 字段（id/title/type/unread/last），全字段会多 30 倍 token。

### 两级查询（省 token 的关键）

**不要**一次拉所有消息。正确做法：

```
1. chat list          → 看哪个群有未读（约 2k tokens）
2. message history    → 只读那个群（约 1.5k tokens）
```

### 常见错误码

| 错误 | 含义 | 处理 |
|---|---|---|
| `FLOOD_WAIT_N` | 请求太频繁，等 N 秒 | 服务已自动等待，若仍失败则告知用户 |
| `CHANNEL_PRIVATE` | 未加入该频道 | 先用 `group join` 加入 |
| `USER_PRIVACY_RESTRICTED` | 对方隐私设置限制 | 用 `contact add` 加联系人后再发 |
| `MESSAGE_ID_INVALID` | 消息 ID 不存在 | 检查 msg_id 是否正确 |
| `CHAT_WRITE_FORBIDDEN` | 无权在该群发言 | 检查群权限或被禁言 |
| `PEER_ID_INVALID` | 找不到该聊天 | 用 `@username` 或先拉一次 `chat list` 建立缓存 |

### 账号参数

多账号时，每次调用可传 `account` 指定账号；不传则用默认账号。

```json
{"account": "work", "action": "list"}
```

## 工具清单（12 个）

| 工具 | 一句话 |
|---|---|
| `system` | 服务状态、账号列表、环境自检 |
| `chat` | 会话列表、详情、未读、已读、文件夹 |
| `message` | 收发编辑删除转发搜索历史反应置顶 |
| `group` | 群成员、拉人踢人、管理员、权限、审批、加入退出、评论区 |
| `contact` | 用户资料、联系人、搜索、添加、拉黑 |
| `media` | 上传下载、图片按需查看 |
| `sticker` | 贴纸包、发贴纸、自定义 emoji、反应 |
| `account` | 自己资料、改名字简介头像用户名、登录设备 |
| `bot` | 管理自己的 Bot、内联查询 |
| `botuse` | 使用别人的 Bot（读按钮、点回调、Web App） |
| `asr` | 语音转文字 |
| `stats` | 群统计 |

## 使用原则

1. **先查后改**：不确定 chat ID 时先 `chat list`
2. **精简优先**：默认不加 `expand`，除非确实需要完整字段
3. **批量优于循环**：`group invite` 支持 `users` 数组，不要一次一个
4. **危险操作确认**：踢人、删消息、退出群前向用户确认
5. **不要猜测 ID**：用 `@username` 或先查列表，不要凭记忆填数字
