# 频道与评论区

频道（Channel）和超级群（Supergroup）在 Telegram 内部是同一种结构，区别只在 `Megagroup` 标志位。

## 频道 vs 超级群

| | 频道（Channel） | 超级群（Supergroup） |
|---|---|---|
| `type` | `channel` | `supergroup` |
| 谁能发言 | 只有管理员 | 所有成员 |
| 人数上限 | 无限 | 20 万 |
| 有评论区 | ✅（可选） | ❌（本身就是群） |
| 成员叫 | 订阅者（subscribers） | 成员（members） |

## 加入频道/群

```json
{"action": "join", "chat": "@publicchannel"}
{"action": "join", "link": "https://t.me/+abcdefg"}
```

**三种结果**：

1. **直接加入** → `{"note": "已加入"}`
2. **需要审批** → `{"note": "该群需要管理员审批（群名：xxx，1234 人），已提交申请"}`
3. **需要 Web App 验证** → `{"note": "需要 Web App 验证（Bot ID 123），请用 botuse 打开"}`

## 退出频道/群

```json
{"action": "leave", "chat": "@channel"}
```

**注意**：退出后该聊天从列表消失，历史记录也不再可见（除非重新加入）。

## 评论区（频道专属）

### 什么是评论区

频道开通"讨论"后，Telegram 会自动创建一个关联的**讨论组**（LinkedChat）。
你在频道帖子下的"评论"，实际是发到那个讨论组里，并 `reply_to` 指向原帖。

### 读评论

```json
{"action": "comments", "chat": "@channel", "msg_id": 123, "limit": 20}
```

返回评论列表，按时间倒序。

### 发评论

```json
{"action": "comments", "chat": "@channel", "msg_id": 123, "text": "说点什么"}
```

### 什么时候没有评论区

- 频道没开讨论功能
- 频道是"禁止评论"模式
- 该帖子被单独关闭评论

此时调用会返回空列表或错误。

## 频道管理

频道管理大部分复用 `group` 工具：

| 操作 | 调用 |
|---|---|
| 看订阅者 | `group members chat=@channel` |
| 加管理员 | `group promote chat=@channel user=@x rights=[...]` |
| 改频道名 | 阶段 2 实现 |
| 发帖 | `message send chat=@channel text=...` |
| 删帖 | `message delete chat=@channel msg_id=...` |
| 置顶 | `message pin chat=@channel msg_id=...` |

## 查找频道

### 按用户名

```json
{"action": "join", "chat": "@channelusername"}
```

### 按邀请链接

```json
{"action": "join", "link": "https://t.me/+abcdefg"}
```

### 从用户资料页找个人频道

```
1. contact info user=@someone
2. 返回里的 channel 字段就是他的个人频道
3. group join chat=@他的频道用户名
```

## 常见错误

| 错误 | 含义 | 处理 |
|---|---|---|
| `CHANNEL_PRIVATE` | 未加入或频道私有 | 先 join |
| `CHANNEL_INVALID` | 频道不存在或已删除 | 检查用户名 |
| `CHANNELS_TOO_MUCH` | 加入的频道太多（Telegram 限制约 500） | 退出一些不用的 |
| `INVITE_REQUEST_SENT` | 已提交审批申请 | 等待管理员批准 |
| `INVITE_HASH_EXPIRED` | 邀请链接已过期 | 让对方重新生成 |
| `USER_ALREADY_PARTICIPANT` | 已在群里 | 无需操作 |

## 常见问题

**Q: 怎么知道频道有没有评论区？**
A: 先 `chat info chat=@channel`，如果返回里有 `linked_chat` 就说明有。当前版本简化处理，直接调 `comments` 即可。

**Q: 能发带图片的帖子吗？**
A: 可以，用 `media` 工具，`chat` 填频道。

**Q: 频道能设置慢速模式吗？**
A: 阶段 2 实现。

**Q: 为什么 join 之后马上又被踢了？**
A: 群里有验证 Bot。按 `group.md` 里的"Bot 人机验证怎么处理"流程走。
