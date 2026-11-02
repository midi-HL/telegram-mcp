# 群组管理（group 工具）

## group members — 成员列表

```json
{"action": "members", "chat": "@group", "limit": 50}
{"action": "members", "chat": "@group", "query": "张三"}   // 搜索
```

返回：
```json
{
  "members": [
    {"id": 123, "name": "张三", "username": "zhangsan", "role": "member", "joined": 1730000000},
    {"id": 456, "name": "李四", "role": "admin"},
    {"id": 789, "name": "王五", "role": "creator"}
  ],
  "count": 3
}
```

**role 值**：`creator`（群主）/ `admin`（管理员）/ `member`（成员）/ `banned`（被封禁）

## group invite — 拉人进群

```json
{"action": "invite", "chat": "@group", "user": "@zhangsan"}
{"action": "invite", "chat": "@group", "users": ["@a", "@b", "@c"]}
```

**限制**：
- 需要 `invite_users` 管理员权限
- 对方设了"谁能拉我进群"隐私限制时会失败
- 目标必须是用户，不能是群/频道

## group kick — 踢人

```json
{"action": "kick", "chat": "@group", "user": "@zhangsan"}
```

**说明**：调用 `channels.editBanned`，效果是"封禁并移除"。对方可以重新通过邀请链接加入（除非设为永久封禁）。

## group promote — 设/撤管理员

```json
{
  "action": "promote",
  "chat": "@group",
  "user": "@zhangsan",
  "rights": ["change_info", "delete_messages", "ban_users", "invite_users", "pin_messages"]
}
{"action": "promote", "chat": "@group", "user": "@zhangsan", "role": "member"}  // 撤销
```

**可用权限**：
| 值 | 含义 |
|---|---|
| `change_info` | 改群名/头像/描述 |
| `delete_messages` | 删他人消息 |
| `ban_users` | 踢人/封禁 |
| `invite_users` | 拉人 |
| `pin_messages` | 置顶消息 |
| `add_admins` | 添加其他管理员 |
| `manage_topics` | 管理话题（论坛群） |
| `post_messages` | 频道发帖 |
| `edit_messages` | 编辑他人消息 |

## group permissions — 查看权限

```json
{"action": "permissions", "chat": "@group"}
```

返回 `chat` 对象，含 `is_admin` / `is_creator` / `can_send`。

## group approve — 审批入群请求

```json
{"action": "approve", "chat": "@group", "user": "@zhangsan"}
{"action": "approve", "chat": "@group", "user": "@zhangsan", "role": "reject"}  // 拒绝
```

**前提**：群开了"需要管理员审批"模式（`ChatInvite.RequestNeeded = true`）。

## group join — 加入群/频道

```json
{"action": "join", "chat": "@publicgroup"}
{"action": "join", "link": "https://t.me/+abcdefg"}
```

**三种情况**：
1. **公开群** → 直接加入
2. **需要审批** → 返回"已提交申请"，等管理员批
3. **需要 Bot 验证** → 加入后被踢，需处理验证（见下）

**返回 Web App 验证**：
```json
{"note": "需要 Web App 验证（Bot ID 123），请用 botuse 打开"}
```

## group leave — 退出群/频道

```json
{"action": "leave", "chat": "@group"}
```

**注意**：退出后聊天记录会从列表消失（除非重新加入）。

## group invites — 邀请链接

```json
{"action": "invites", "chat": "@group"}
```

返回：
```json
{"invites": [{"link": "https://t.me/+abc", "permanent": true}]}
```

## group comments — 频道评论区

**读评论**：
```json
{"action": "comments", "chat": "@channel", "msg_id": 123, "limit": 20}
```

**发评论**：
```json
{"action": "comments", "chat": "@channel", "msg_id": 123, "text": "说点什么"}
```

---

## Bot 人机验证怎么处理

很多群有验证 Bot。完整流程：

```
1. group join → 加入
2. 立即被踢（可能返回"已加入"但实际被移除）
3. 检查是否有验证 Bot 私聊你：
   chat list → 找新出现的 Bot 对话
4. message history → 读 Bot 发来的消息
5. 看消息里的按钮（reply_markup）
6. botuse buttons → 列出按钮
7. botuse click → 点"我是人类"
8. group join → 重新加入
```

**验证类型**：
| 类型 | 能否自动 |
|---|---|
| 按钮验证 | ✅ botuse click |
| 数学题 | ✅ 模型能算 |
| 图片验证码 | ✅ media view_image + 模型识图 |
| Web App（hCaptcha） | ❌ 浏览器内交互，无法自动 |

**Web App 验证的处理**：返回明确提示，让用户手动完成。

## 常见问题

**Q: 为什么 members 返回空？**
A: 可能是基础群（非超级群）。当前版本只支持超级群/频道。或者你没有查看成员权限。

**Q: kick 之后对方还能回来吗？**
A: 能，除非设置永久封禁。当前实现是永久封禁。

**Q: 能一次踢多人吗？**
A: 当前逐个人调用。批量需要多次调用。

**Q: 频道评论区和群有什么区别？**
A: 评论区本质是频道关联的讨论组。`comments` 会自动处理这个关联，你只需传频道的 msg_id。
