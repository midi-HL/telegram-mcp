# 用户资料与联系人（contact 工具）

## contact info — 用户资料页

对应 Telegram 客户端点开某人看到的内容。

```json
{"action": "info", "user": "@username"}
```

返回：
```json
{
  "profile": {
    "id": 123456789,
    "first_name": "辰星",
    "last_name": "（Edwin 2J)",
    "username": "example_user",
    "usernames": ["example_user"],
    "phone": "60123456789",
    "bio": "The stars in the sky are not as bright as you.",
    "status": "最近上线 2026-11-02 11:20",
    "last_seen": 1730000000,
    "is_bot": false,
    "is_premium": true,
    "is_verified": false,
    "is_contact": true,
    "is_mutual": true,
    "is_blocked": false,
    "birthday": "5月21日",
    "common_chats": 2,
    "has_photo": true,
    "photo_id": 123456,
    "channel": {
      "id": 789,
      "title": "Music Collection音乐收藏",
      "username": "musiccollection",
      "subs": 8
    }
  }
}
```

**字段说明**：

| 字段 | 说明 | 何时为空 |
|---|---|---|
| `phone` | 手机号 | 对方隐私设置不允许，且不在通讯录 |
| `bio` | 简介 | 对方没填 |
| `status` | 在线状态 | 对方隐藏了上线时间 |
| `birthday` | 生日 | 对方没填，或隐私设置不允许 |
| `channel` | 个人频道 | 对方没设 |
| `common_chats` | 共同群组数 | 没有共同群 |

**token 成本**：约 300 tokens（相比全字段 3.7k）。

## contact list — 联系人列表

```json
{"action": "list", "limit": 50}
```

返回精简资料（id/name/username/phone）。

## contact search — 搜索联系人

```json
{"action": "search", "query": "张三", "limit": 20}
```

**只搜自己的通讯录**，不是全局搜索用户。

## contact add — 添加联系人

**方式 1：按用户名**
```json
{"action": "add", "user": "@zhangsan", "first_name": "张三"}
```

**方式 2：按手机号**
```json
{"action": "add", "phone": "+8613800138000", "first_name": "张三"}
```

**重要用途**：绕过"陌生人消息隐私限制"。

```
1. message send → USER_PRIVACY_RESTRICTED
2. contact add phone=+86xxx → 静默添加（对方无感知）
3. message send → 成功
```

**Telegram 通讯录是单向的**：你加对方，对方手机不会出现你，也不会收到通知。

## contact block — 拉黑/取消拉黑

```json
{"action": "block", "user": "@spammer"}
{"action": "block", "user": "@spammer", "query": "unblock"}
```

## contact resolve — 只解析用户名

```json
{"action": "resolve", "user": "@username"}
```

只返回 ID，不拉完整资料。适合只想确认 ID 的场景（省 token）。

---

## 用户 ID 的三种形态

| 场景 | 示例 | 说明 |
|---|---|---|
| 用户 | `123456789` | 正数 |
| 基础群 | `-123456789` | 负数 |
| 超级群/频道 | `-1001234567890` | `-100` 前缀 |

**注意**：`contact info` 只支持用户。群/频道用 `chat info`。

## 隐私限制的完整说明

Telegram 每个账号有"谁能给我发私信"设置：

| 设置 | 直接发消息 | 加联系人后 |
|---|---|---|
| 所有人 | ✅ | ✅ |
| 我的联系人（默认） | ❌ | ✅ |
| 没有人 | ❌ | ❌ |

**绕过限制的前提**：
1. 你得知道对方手机号（只有 @用户名不够）
2. 对方设置不能是"没有人"

## 常见问题

**Q: 为什么看不到对方手机号？**
A: 对方隐私设置不允许。只有①对方设为"所有人可见"②你已在对方通讯录里，才能看到。

**Q: 生日为什么有的是"5月21日"有的是"2001-05-21"？**
A: 看对方是否公开了出生年份。不公开年份时只显示月日。

**Q: 共同群组数怎么算的？**
A: Telegram 返回的 `common_chats_count`，只算双方都在的群。

**Q: 个人频道的订阅数为什么是 0？**
A: 当前版本从 `channels.getChannels` 拿，可能需要额外拉 `channels.getFullChannel` 才有准确订阅数。已知限制。
