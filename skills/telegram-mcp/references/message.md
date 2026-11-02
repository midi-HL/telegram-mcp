# 消息操作（message 工具）

## message send — 发送消息

```json
{
  "action": "send",
  "chat": "@username 或 数字ID",
  "text": "消息内容",
  "reply_to": 123,        // 可选：回复哪条消息
  "silent": false         // 可选：静默发送（不通知对方）
}
```

返回：`{"msg_id": 351, "note": "已发送"}`

**注意**：
- 文本支持 Markdown（`**粗体**`、`__斜体__`、`` `代码` ``）
- 如果对方设了隐私限制，会返回 `USER_PRIVACY_RESTRICTED`，此时需先 `contact add`

## message edit — 编辑已发消息

```json
{
  "action": "edit",
  "chat": "...",
  "msg_id": 351,
  "text": "新的内容"
}
```

**限制**：只能编辑自己发的消息；频道消息需管理员权限。

## message delete — 删除消息

```json
{
  "action": "delete",
  "chat": "...",
  "msg_id": 351
}
```

**说明**：`revoke: true` 会同时从对方那里删除（Telegram 的"为所有人删除"）。
群管理场景删除他人消息需要 `delete_messages` 管理员权限。

## message forward — 转发

```json
{
  "action": "forward",
  "chat": "@来源聊天",
  "to": "@目标聊天",
  "msg_id": 351
}
```

## message search — 搜索消息

```json
{
  "action": "search",
  "query": "关键词",
  "chat": "@可选，限定在某个聊天里搜",
  "limit": 20
}
```

不传 `chat` 则全局搜索（只搜自己加入的聊天）。

## message history — 读历史

```json
{
  "action": "history",
  "chat": "...",
  "limit": 20,
  "before_id": 0,          // 可选：从这条消息往前翻
  "expand": ["full"]       // 可选：返回完整字段
}
```

返回按时间**倒序**（最新在前）。

**默认返回 6 字段**：`id / date / sender / sender_name / text / media / out`
`expand: ["full"]` 追加：`reply_to / forward / media_meta / views / reactions`

## message react — 加表情反应

```json
{
  "action": "react",
  "chat": "...",
  "msg_id": 351,
  "emoji": "👍"
}
```

常用 emoji：👍 ❤️ 🔥 🎉 😁 😢 👏 🤔 💯

## 引用消息（Reply）的处理

### 自动展开

`message history` 和 `message search` 返回时，**自动展开**被引用消息的摘要：

```json
{
  "id": 360,
  "text": "这张图不错",
  "reply_to": 359,
  "reply": {
    "msg_id": 359,
    "sender": "若叶睦 | Edwin 3J | Mutsumi",
    "text": "[引用测试] 原图",
    "media": "photo",
    "media_meta": {"kind": "photo", "w": 1536, "h": 1872, "size": 526964},
    "deleted": false
  }
}
```

**不需要额外调用**——模型一眼就知道引用的是什么。

### 被引用的是媒体时

`reply.media` 会标出类型，模型可以直接下载（用 `reply.msg_id`）：

| reply.media | 操作 |
|---|---|
| `photo` | `media view_image` 或 `media download` |
| `video` | `media download` |
| `document` | `media download` |
| `voice` | `asr transcribe` 或 `media download` |
| `sticker` | `sticker send` |
| `""` | 纯文字，text 字段就是内容 |

**示例**：用户问"这条回复引用的是什么图？"

```
1. message history → 看到 reply.media = "photo"，reply.msg_id = 359
2. media view_image chat=... msg_id=359 → 拿到图片 base64
```

### 原消息已删除

```json
{"reply": {"msg_id": 359, "deleted": true}}
```

### 部分引用（Quote）

```json
{"reply": {"msg_id": 359, "text": "完整原文...", "quote": "被引用的那一段"}}
```

### 发带引用的消息

```json
{"action": "send", "chat": "@user", "text": "回复内容", "reply_to": 359}
```

### 三种"引用"的区别

| 类型 | 字段 | 能否拿到原文 | 能否下载媒体 |
|---|---|---|---|
| **回复（Reply）** | `reply_to` + `reply` | ✅ 自动展开 | ✅ |
| **转发（Forward）** | `forward` | ⚠️ 只有原聊天/发送者信息 | ⚠️ 需跳到原聊天 |
| **引用片段（Quote）** | `reply.quote` | ✅ 引用文本 | — |

### 常见问题

**Q: 为什么 reply 字段有时没有？**
A: 消息本身不是回复。只有 `reply_to` 存在的消息才有 `reply`。

**Q: 批量读 100 条消息时，引用展开会不会很慢？**
A: 不会。用一次 `messages.getMessages` 批量拉取所有被引用的消息，不是 N+1。

**Q: 被引用的图片很大，会占多少 token？**
A: 只返回元数据（约 30 tokens）。要看图才调 `view_image`（约 1500 tokens）。

## message pin — 置顶/取消置顶

```json
{"action": "pin", "chat": "...", "msg_id": 351}
{"action": "pin", "chat": "...", "msg_id": 351, "text": "unpin"}
```

---

## 媒体消息的识别

`message history` 返回的 `media` 字段值：

| 值 | 含义 | 如何看内容 |
|---|---|---|
| `photo` | 图片 | `media view_image`（按需，约 1500 tokens/张） |
| `video` | 视频 | 只给封面帧和时长，不提供视频流 |
| `document` | 文件 | 给文件名/大小，`media download` 下载 |
| `voice` | 语音 | `asr transcribe` 转文字 |
| `audio` | 音乐 | 给标题/歌手/时长 |
| `sticker` | 贴纸 | 给 emoji 关联 |
| `poll` | 投票 | 给选项和票数 |
| `location` | 位置 | 给经纬度 |
| `webpage` | 链接预览 | 给标题/描述 |

**重要**：默认不给图片内容，因为一张图约 1500 tokens。模型应先用元数据判断是否需要看图。

## 常见问题

**Q: 发的消息对方没收到？**
A: 检查是否 `USER_PRIVACY_RESTRICTED`。若是，先 `contact add` 加联系人。

**Q: 消息 ID 是什么？**
A: 每个聊天内自增的整数，不是全局唯一。所以 `(chat, msg_id)` 才唯一。

**Q: 能发带格式的消息吗？**
A: 支持 Markdown。复杂格式（表格、代码块）用 `text` 里的 Markdown 语法。

**Q: 一次发多条？**
A: 目前逐条发。相册（多图一次发）用 `media send_album`。
