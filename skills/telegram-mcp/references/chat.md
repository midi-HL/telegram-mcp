# 会话管理（chat 工具）

## chat list — 会话列表

```json
{"action": "list", "limit": 50}
```

返回：
```json
{
  "chats": [
    {"id": 123, "title": "群名", "type": "supergroup", "unread": 47, "last": "最后一句", "at": 1730000000}
  ],
  "total": 24
}
```

**type 值**：`user`（私聊）/ `group`（基础群）/ `supergroup`（超级群）/ `channel`（频道）

**token 成本**：50 个会话约 9k tokens（全字段会是 133k）。

## chat info — 会话详情

```json
{"action": "info", "chat": "@username", "expand": ["full"]}
```

返回：
```json
{
  "chat": {
    "id": 123, "title": "群名", "type": "supergroup",
    "about": "群描述", "username": "xxx",
    "members": 1234, "can_send": true,
    "is_admin": false, "is_creator": false,
    "muted": false, "pinned": false
  }
}
```

**先查缓存**：如果最近查过，直接返回缓存（毫秒级）。要强制刷新需重启服务。

## chat unread — 未读概览

```json
{"action": "unread"}
```

返回：
```json
{
  "unread": {
    "total_chats": 5,
    "total_unread": 3939,
    "top": [
      {"id": 1, "title": "Log", "type": "channel", "unread": 3891}
    ]
  }
}
```

**这是省 token 的关键工具**。先看哪个群有未读，再决定读哪个。

## chat mark_read — 标记已读

```json
{"action": "mark_read", "chat": "@username"}
```

## chat folders — 聊天文件夹

```json
{"action": "folders"}
```

返回：
```json
{
  "folders": [
    {"id": 1, "title": "工作", "emoji": "💼", "chats": 12}
  ]
}
```

**说明**：文件夹是 Telegram 的聊天分组功能，类似"标签"。这里只读，不支持创建（阶段 2 再加）。

---

## 典型用法

### 场景 1：看有什么新消息

```
1. chat unread                    → 发现「Log」频道有 3891 条未读
2. message history chat=Log limit=10   → 只看最近 10 条
```

### 场景 2：找某个群

```
1. chat list limit=100            → 列表里找
2. 如果找不到，用 contact search 或 group join 加入
```

### 场景 3：判断能否发言

```
1. chat info chat=@group          → 看 can_send 字段
2. 若 can_send=false，检查 is_admin 或群权限
```

## 常见问题

**Q: 为什么 chat list 里的 last 字段是空的？**
A: 当前版本只返回消息预览的占位。要真实预览用 `message history`。

**Q: 会话数量对不上？**
A: `chat list` 默认只取最近活跃的 N 个。增大 `limit`（最大 200）或分页。

**Q: 怎么知道某个群是不是频道？**
A: 看 `type` 字段。`channel` 是广播频道（只有管理员能发），`supergroup` 是超级群（成员都能发）。
