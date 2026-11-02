# 语音转写与统计（asr / stats 工具）

## asr — 语音转文字

### 转写 Telegram 语音消息

```json
{"action": "transcribe", "chat": "@user", "msg_id": 123}
```

### 转写本地音频文件

```json
{"action": "transcribe", "path": "/path/to/audio.ogg"}
```

### 查看当前后端

```json
{"action": "backend"}
```

## 五种后端

| 后端 | 优点 | 缺点 | 配置 |
|---|---|---|---|
| `sherpa` | 完全离线、免费 | 需下载 74MB 模型 | `tgmcp asr setup` |
| `openai` | 质量最好、多语言 | 要 API key、要钱 | `tgmcp asr config --backend openai --api-key sk-xxx` |
| `native` | 零 ASR 成本 | 模型需支持音频输入（Claude 不支持） | `tgmcp asr config --backend native` |
| `none` | 最省 | 不转写 | 默认 |

### 后端选择建议

- **日常用**：`sherpa`（离线、免费、中文效果好）
- **高质量需求**：`openai`（Whisper API）
- **用 Gemini 等支持音频的模型**：`native`（直接给音频，模型自己理解）
- **只要元数据**：`none`

## 安装 sherpa

```bash
# 1. 下载模型（74MB）
tgmcp asr setup

# 2. 下载 CLI（三选一）
# Windows: sherpa-onnx-non-streaming-asr-x64-v2.0.1.exe
# Linux:   sherpa-onnx-v2.0.1-linux-x64-shared.tar.bz2
# Termux:  sherpa-onnx-v2.0.1-android-aarch64-termux-shared.tar.bz2

# 3. 指定 CLI 路径
tgmcp asr config --cli /path/to/sherpa-onnx
```

## 工作方式

sherpa 后端用**子进程**调用 CLI，不是 cgo 链接。好处：

- 主程序保持纯 Go（Termux 可运行）
- ASR 崩溃不影响主程序
- 不用时不占内存

## stats — 统计信息

### stats cache — 本地缓存统计

```json
{"action": "cache"}
```

返回：
```json
{
  "chats": 24,
  "messages": 49,
  "users": 2,
  "db_size_mb": 0.1
}
```

### stats group — 群统计

```json
{"action": "group", "chat": "@group"}
```

返回：
```json
{
  "members": 37,
  "online": 1,
  "admins": 0,
  "kicked": 0,
  "banned": 0,
  "stats_note": "无统计权限（需要群主或管理员）"
}
```

**限制**：详细统计（成员增长曲线、消息量、活跃时段）需要：
1. 你是群主或管理员
2. 群人数足够（Telegram 要求通常 500+）

不满足时只返回基础数字。

### stats chat — 单会话统计

```json
{"action": "chat", "chat": "@user"}
```

从**本地缓存**统计（不查服务器）：
```json
{
  "chat_id": 123,
  "title": "群名",
  "type": "supergroup",
  "unread": 0,
  "members": 37,
  "cached_messages": 18
}
```

## 常见问题

**Q: 转写 30 秒语音要多久？**
A: sherpa + paraformer-zh-small 在手机上约 2-5 秒，电脑上 1-2 秒。

**Q: 支持哪些音频格式？**
A: OGG/Opus（Telegram 语音默认）、MP3、M4A、WAV、WebM。其他格式需先转换。

**Q: 模型太大，能换小的吗？**
A: 当前用的就是 small 版（74MB）。还有 int8 量化版（218MB）质量更好但更大。

**Q: 群统计为什么没有数据？**
A: 需要群主/管理员权限，且群人数要足够（Telegram 的门槛）。小群或个人群拿不到详细数据。
