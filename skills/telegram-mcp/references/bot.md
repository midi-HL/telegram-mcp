# Bot 管理与第三方 Bot（bot / botuse 工具）

## 两种完全不同的场景

| | **管理自己的 Bot**（`bot`） | **使用别人的 Bot**（`botuse`） |
|---|---|---|
| 你是 | Bot 的拥有者 | 普通用户 |
| 能做什么 | 改名字/简介、设命令、导出 token、查收入 | 发指令、点按钮、收文件 |
| 底层 | `bots.*` 命名空间 | `messages.*` |

## bot — 管理自己的 Bot

### bot list — 我管理的 Bot

```json
{"action": "list"}
```

返回所有你通过 BotFather 创建的 Bot。

### bot create — 创建 Bot

```json
{"action": "create", "username": "my_test_bot", "name": "我的测试机器人"}
```

**注意**：用户名必须全局唯一且以 `bot` 结尾。创建后会返回 Bot 的 ID。

### bot info — 查看 Bot 信息

```json
{"action": "info", "bot": "@my_test_bot"}
```

### bot set_info — 改 Bot 信息

```json
{"action": "set_info", "bot": "@my_test_bot", "name": "新名字", "about": "新简介"}
```

### bot set_commands — 设置命令菜单

```json
{"action": "set_commands", "bot": "@my_test_bot", "commands": ["start:开始使用", "help:查看帮助", "settings:设置"]}
```

格式：`命令:描述`。命令不带 `/`。

### bot export_token — 导出 token

```json
{"action": "export_token", "bot": "@my_test_bot"}
```

返回 Bot API token。**这个 token 能完全控制 Bot，务必保密。**

**注意**：可能需要 BotFather 授权，失败时会返回明确提示。

### bot check_username — 检查用户名可用

```json
{"action": "check_username", "username": "my_new_bot"}
```

## botuse — 使用别人的 Bot

### 完整链路

```
1. botuse start          → 发 /start，Bot 回复欢迎消息
2. botuse read_replies   → 读 Bot 的消息
3. botuse buttons        → 读消息里的按钮
4. botuse click          → 点按钮
5. 重复 2-4 直到拿到文件
6. media download        → 下载文件
```

### botuse start — 启动 Bot

```json
{"action": "start", "bot": "@some_bot"}
{"action": "start", "bot": "@some_bot", "text": "ref_code"}   // 带参数（推荐链接）
```

### botuse read_replies — 读 Bot 回复

```json
{"action": "read_replies", "bot": "@some_bot", "limit": 10}
```

返回消息列表，`out: true` 是你发的，`out: false` 是 Bot 发的。

### botuse buttons — 读按钮

```json
{"action": "buttons", "bot": "@some_bot", "msg_id": 353}
```

返回：
```json
{
  "buttons": [
    {"i": 0, "text": "🇬🇧 English", "type": "callback", "data": "BAEBAg..."},
    {"i": 1, "text": "🇮🇩 Indonesia", "type": "callback", "data": "BAEBAg..."},
    {"i": 2, "text": "官网", "type": "url", "url": "https://..."}
  ]
}
```

**按钮类型**：
| type | 含义 | 能否点 |
|---|---|---|
| `callback` | 回调按钮 | ✅ `botuse click` |
| `url` | 打开链接 | ❌ 需浏览器 |
| `webview` | 打开 Web App | ❌ 需浏览器 |
| `switch_inline` | 切到内联查询 | ❌ 需客户端 UI |
| `copy` | 复制文本 | ❌ 无副作用 |

### botuse click — 点按钮

**方式 1：按索引（推荐）**
```json
{"action": "click", "bot": "@some_bot", "msg_id": 353, "index": 0}
```

**方式 2：按 data（base64）**
```json
{"action": "click", "bot": "@some_bot", "msg_id": 353, "data": "BAEBAg..."}
```

返回：
```json
{
  "callback": {
    "message": "✅ 签到成功！获得 5 积分",
    "alert": false,
    "url": ""
  }
}
```

### botuse send_text — 给 Bot 发文字

```json
{"action": "send_text", "bot": "@some_bot", "text": "/checkin"}
```

### botuse webapp — 打开 Web App

```json
{"action": "webapp", "bot": "@some_bot"}
```

**限制**：Web App 是浏览器内运行的，MTProto 只能拿到 URL，无法执行里面的 JS。所以 hCaptcha 这类验证**做不了**。

## 典型场景：领福利 Bot

```
用户："用 @gemini_plusv_bot 领一下今天的签到"

模型执行：
1. botuse start @gemini_plusv_bot          → 拿到欢迎消息
2. botuse read_replies @gemini_plusv_bot   → 看到"📋 Daily Check-in"按钮
3. botuse buttons msg_id=353               → 按钮列表
4. botuse click msg_id=353 index=1         → 点"Check-in"
5. botuse read_replies                     → 看到"✅ Check-in Successful! +5 points"
```

## 常见限制（如实告知用户）

| 限制 | 原因 | 能否绕过 |
|---|---|---|
| 需要先订阅频道 | Bot 检查你的订阅状态 | ❌ 只能真的去 join |
| 需要分享手机号 | Telegram 弹确认框 | ❌ 无头环境做不了 |
| hCaptcha 验证 | 浏览器内 JS | ❌ MTProto 碰不到 |
| 图片验证码 | 需要识图 | ✅ `media view_image` + 模型识图 |
| 数学题验证 | 需要计算 | ✅ 模型能算 |
| Bot 每日限次 | Bot 的业务规则 | ❌ 只能等 |

## 常见问题

**Q: `bot create` 失败？**
A: 用户名被占用，或者你的账号没有创建 Bot 的权限。先用 `check_username` 检查。

**Q: `export_token` 失败？**
A: 需要 BotFather 授权。降级方案：让用户去 @BotFather 手动 `/mybots` → 选 Bot → API Token。

**Q: 点按钮没反应？**
A: 检查按钮类型是否是 `callback`。URL 类型点了没意义（只是打开链接）。

**Q: 能自动完成 Web App 里的操作吗？**
A: 不能。Web App 是浏览器环境，MTProto 只能拿到 URL。
