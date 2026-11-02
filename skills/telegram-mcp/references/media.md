# 媒体（media 工具）

## 核心原则：图片默认不给内容

一张图片约 **1500 tokens**，20 张就是 30k。所以：

```
message history → 只返回 media:"photo"（元数据）
                    ↓ 模型判断需要看
media view_image → 才下载并转 base64（约 1500 tokens/张）
```

## media send_photo — 发图片

```json
{"action": "send_photo", "chat": "@user", "path": "/path/to/pic.jpg", "text": "配文"}
```

支持 JPG/PNG/WebP。Telegram 会自动压缩（3MB → 约 500KB）。

## media send_file — 发文件

```json
{"action": "send_file", "chat": "@user", "path": "/path/to/doc.pdf", "text": "说明"}
```

文件不会被压缩，原样发送。

## media send_video — 发视频

```json
{"action": "send_video", "chat": "@user", "path": "/path/to/video.mp4"}
```

## media send_voice — 发语音

```json
{"action": "send_voice", "chat": "@user", "path": "/path/to/voice.ogg"}
```

语音消息会显示波形，用 OGG/Opus 格式效果最好。

## media send_album — 发相册（多图一次发）

```json
{"action": "send_album", "chat": "@user", "paths": ["/a.jpg", "/b.jpg", "/c.jpg"], "text": "配文"}
```

一次最多 10 个媒体。

## media download — 下载媒体

```json
{"action": "download", "chat": "@user", "msg_id": 123, "out": "/downloads"}
```

不传 `out` 时默认存到 `{data_dir}/downloads/`。

返回本地文件路径。

## media view_image — 按需看图

```json
{"action": "view_image", "chat": "@user", "msg_id": 123, "max_size": 2000000}
```

返回 base64 编码的图片。`max_size` 默认 2MB，超过会拒绝并提示用 download。

**注意**：返回的 base64 很大（500KB 图 ≈ 700KB base64 ≈ 200k tokens！）。所以：

- 只在**确实需要看图片内容**时调用
- 不要循环调用（一次一张）
- 如果只需要知道"有没有图"，用 `message history` 的 media 字段即可

## 媒体类型对照

| `media` 字段 | 类型 | 默认给什么 | 按需操作 |
|---|---|---|---|
| `photo` | 图片 | 无内容 | `view_image` 或 `download` |
| `video` | 视频 | 无内容 | `download` |
| `document` | 文件 | 文件名/大小 | `download` |
| `voice` | 语音 | 无内容 | `asr transcribe` 或 `download` |
| `audio` | 音乐 | 标题/歌手 | `download` |
| `sticker` | 贴纸 | emoji | `sticker send` |
| `poll` | 投票 | 选项/票数 | — |
| `location` | 位置 | 经纬度 | — |
| `webpage` | 链接预览 | 标题/描述 | — |
| `""` | 纯文字 | 文本本身 | — |

## 常见问题

**Q: 为什么发图片报 `PHOTO_SAVE_FILE_INVALID`？**
A: 上传时没传文件大小。已修复（用 `FromPath` 而非 `FromReader`）。

**Q: 图片被压缩了怎么办？**
A: 用 `send_file` 而不是 `send_photo`，文件不会被压缩。

**Q: view_image 返回的 base64 太大怎么办？**
A: 调小 `max_size`，或者先用 `download` 存到本地再用其他工具处理。

**Q: 能发 GIF 吗？**
A: 用 `send_file` 发 .mp4 格式的 GIF（Telegram 会自动识别为动画）。
