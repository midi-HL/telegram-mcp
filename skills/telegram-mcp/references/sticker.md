# 贴纸与表情（sticker 工具）

## sticker search_sets — 搜索贴纸包

```json
{"action": "search_sets", "query": "cat", "limit": 10}
```

返回贴纸包列表（ID/标题/短名/数量）。

## sticker set_info — 贴纸包详情

```json
{"action": "set_info", "set_id": 123456}
{"action": "set_info", "query": "UtyaDuck"}
```

返回贴纸包信息和所有贴纸（含 emoji 关联）。

## sticker send — 发贴纸

```json
{"action": "send", "chat": "@user", "set_id": 123456, "sticker": "🐱"}
```

`sticker` 是 emoji，用于在贴纸包里找匹配的贴纸。不传则用第一个。

## sticker react — 加表情反应

```json
{"action": "react", "chat": "@user", "msg_id": 123, "emoji": "👍"}
```

**常用 emoji**：👍 ❤️ 🔥 🎉 😁 😢 👏 🤔 💯 🍾

## sticker custom_emoji — 自定义 emoji

```json
{"action": "custom_emoji", "limit": 20}
```

列出你已安装的贴纸包（含自定义 emoji 包）。

## 与 message react 的区别

两个工具都能加反应，区别：
- `message react`：消息工具的一部分，需要 chat + msg_id + emoji
- `sticker react`：功能相同，放在贴纸工具里

**建议**：用 `message react`（更直观）。

## 常见问题

**Q: 贴纸包 ID 怎么拿？**
A: `search_sets` 搜索后返回的 `id` 字段。

**Q: 能发自定义 emoji 到消息里吗？**
A: 自定义 emoji 需要 Premium 账号，且要在文本里用特殊格式。当前版本暂不支持。
