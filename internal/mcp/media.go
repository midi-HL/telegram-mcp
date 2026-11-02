package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

// MediaInput 是 media 工具的输入。
type MediaInput struct {
	// Action: send_photo / send_file / send_video / send_voice / send_album / download / view_image
	Action string `json:"action" jsonschema:"send_photo|send_file|send_video|send_voice|send_album|download|view_image"`
	Chat   string `json:"chat,omitempty" jsonschema:"chat id"`
	Path   string `json:"path,omitempty" jsonschema:"local file path to upload"`
	Paths  []string `json:"paths,omitempty" jsonschema:"multiple paths for album"`
	Text   string `json:"text,omitempty" jsonschema:"caption"`
	MsgID  int    `json:"msg_id,omitempty" jsonschema:"message id to download"`
	Out    string `json:"out,omitempty" jsonschema:"output dir for download"`
	// ViewImage 用
	MaxSize int `json:"max_size,omitempty" jsonschema:"max bytes for view_image, default 2MB"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// MediaOutput 是 media 工具的输出。
type MediaOutput struct {
	Action   string          `json:"action"`
	MsgID    int             `json:"msg_id,omitempty"`
	MsgIDs   []int           `json:"msg_ids,omitempty"`
	FilePath string          `json:"file_path,omitempty"`
	Size     int64           `json:"size,omitempty"`
	Mime     string          `json:"mime,omitempty"`
	Width    int             `json:"width,omitempty"`
	Height   int             `json:"height,omitempty"`
	Duration int             `json:"duration,omitempty"`
	Base64   string          `json:"base64,omitempty"`
	MediaType string         `json:"media_type,omitempty"`
	Note     string          `json:"note,omitempty"`
	Meta     *dto.MediaMeta  `json:"meta,omitempty"`
}

// registerMediaTools 注册 media 工具。
func registerMediaTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "media",
		Description: "媒体上传下载。action: send_photo 发图 / send_file 发文件 / send_video 发视频 / send_voice 发语音 / send_album 发相册 / download 下载 / view_image 按需看图（base64）",
		Annotations: &mcp.ToolAnnotations{Title: "媒体", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in MediaInput) (*mcp.CallToolResult, MediaOutput, error) {
		return handleMedia(ctx, deps, in)
	})
}

func handleMedia(ctx context.Context, deps *Deps, in MediaInput) (*mcp.CallToolResult, MediaOutput, error) {
	out := MediaOutput{Action: in.Action}

	switch in.Action {
	case "send_photo":
		return handleSendPhoto(ctx, deps, in, out)
	case "send_file":
		return handleSendFile(ctx, deps, in, out)
	case "send_video":
		return handleSendVideo(ctx, deps, in, out)
	case "send_voice":
		return handleSendVoice(ctx, deps, in, out)
	case "send_album":
		return handleSendAlbum(ctx, deps, in, out)
	case "download":
		return handleDownload(ctx, deps, in, out)
	case "view_image":
		return handleViewImage(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// uploadFile 上传本地文件。
// 用 FromPath 而非 FromReader：FromPath 会自动读取文件大小和名称，
// 这是 Telegram 服务端校验 PHOTO_SAVE_FILE_INVALID 的前提。
func uploadFile(ctx context.Context, api *tg.Client, path string) (tg.InputFileClass, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件信息: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s 是目录", path)
	}
	u := uploader.NewUploader(api).WithThreads(4)
	return u.FromPath(ctx, path)
}

// handleSendPhoto 发送图片。
func handleSendPhoto(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || in.Path == "" {
		out.Note = "send_photo 需要 chat 和 path"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	file, err := uploadFile(ctx, acc.API(), in.Path)
	if err != nil {
		out.Note = fmt.Sprintf("上传失败: %v", err)
		return nil, out, nil
	}

	media := &tg.InputMediaUploadedPhoto{File: file}
	req := &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Media:    media,
		RandomID: randomID(),
	}
	if in.Text != "" {
		req.Message = in.Text
	}
	res, err := acc.API().MessagesSendMedia(ctx, req)
	if err != nil {
		out.Note = fmt.Sprintf("发送失败: %v", err)
		return nil, out, nil
	}
	if id, _ := extractSentMessageID(res); id != 0 {
		out.MsgID = id
	}
	out.Note = "图片已发送"
	return nil, out, nil
}

// handleSendFile 发送文件。
func handleSendFile(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || in.Path == "" {
		out.Note = "send_file 需要 chat 和 path"
		return nil, out, nil
	}
	return sendDocument(ctx, deps, in, out, false, false)
}

// handleSendVideo 发送视频。
func handleSendVideo(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || in.Path == "" {
		out.Note = "send_video 需要 chat 和 path"
		return nil, out, nil
	}
	return sendDocument(ctx, deps, in, out, false, true)
}

// handleSendVoice 发送语音。
func handleSendVoice(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || in.Path == "" {
		out.Note = "send_voice 需要 chat 和 path"
		return nil, out, nil
	}
	return sendDocument(ctx, deps, in, out, true, false)
}

// sendDocument 发送文档类媒体（文件/视频/语音）。
func sendDocument(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput, voice, video bool) (*mcp.CallToolResult, MediaOutput, error) {
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	info, err := os.Stat(in.Path)
	if err != nil {
		out.Note = fmt.Sprintf("读取文件信息: %v", err)
		return nil, out, nil
	}

	file, err := uploadFile(ctx, acc.API(), in.Path)
	if err != nil {
		out.Note = fmt.Sprintf("上传失败: %v", err)
		return nil, out, nil
	}

	mime := guessMime(in.Path)
	attrs := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: filepath.Base(in.Path)},
	}
	if voice {
		attrs = append(attrs, &tg.DocumentAttributeAudio{Voice: true})
	} else if video {
		attrs = append(attrs, &tg.DocumentAttributeVideo{})
	}

	media := &tg.InputMediaUploadedDocument{
		File:       file,
		MimeType:   mime,
		Attributes: attrs,
	}
	req := &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Media:    media,
		RandomID: randomID(),
	}
	if in.Text != "" {
		req.Message = in.Text
	}
	res, err := acc.API().MessagesSendMedia(ctx, req)
	if err != nil {
		out.Note = fmt.Sprintf("发送失败: %v", err)
		return nil, out, nil
	}
	if id, _ := extractSentMessageID(res); id != 0 {
		out.MsgID = id
	}
	out.Size = info.Size()
	out.Mime = mime
	out.Note = "已发送"
	return nil, out, nil
}

// handleSendAlbum 发送相册（多图/多文件）。
func handleSendAlbum(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || len(in.Paths) == 0 {
		out.Note = "send_album 需要 chat 和 paths"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	medias := make([]tg.InputSingleMedia, 0, len(in.Paths))
	for i, p := range in.Paths {
		file, err := uploadFile(ctx, acc.API(), p)
		if err != nil {
			out.Note = fmt.Sprintf("上传 %s 失败: %v", p, err)
			return nil, out, nil
		}
		media := &tg.InputMediaUploadedPhoto{File: file}
		sm := tg.InputSingleMedia{
			Media:    media,
			RandomID: randomID() + int64(i),
		}
		if in.Text != "" && i == 0 {
			sm.Message = in.Text
		}
		medias = append(medias, sm)
	}

	res, err := acc.API().MessagesSendMultiMedia(ctx, &tg.MessagesSendMultiMediaRequest{
		Peer:   peer,
		MultiMedia: medias,
	})
	if err != nil {
		out.Note = fmt.Sprintf("发送相册失败: %v", err)
		return nil, out, nil
	}
	if id, _ := extractSentMessageID(res); id != 0 {
		out.MsgIDs = append(out.MsgIDs, id)
	}
	out.Note = fmt.Sprintf("已发送 %d 个媒体", len(medias))
	return nil, out, nil
}

// handleDownload 下载消息里的媒体到本地。
func handleDownload(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || in.MsgID == 0 {
		out.Note = "download 需要 chat 和 msg_id"
		return nil, out, nil
	}
	acc, resolver, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	peer, err := resolver.Resolve(ctx, in.Chat)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	// 取消息
	msgs, err := acc.API().MessagesGetMessages(ctx, []tg.InputMessageClass{
		&tg.InputMessageID{ID: in.MsgID},
	})
	_ = peer
	if err != nil {
		out.Note = fmt.Sprintf("获取消息失败: %v", err)
		return nil, out, nil
	}
	msgList, _ := extractMessages(msgs)
	if len(msgList) == 0 {
		out.Note = "消息不存在"
		return nil, out, nil
	}

	loc, mime, name, err := mediaLocation(msgList[0])
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	// 输出目录
	dir := in.Out
	if dir == "" {
		dir = filepath.Join(deps.Config.DataDir, "downloads")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		out.Note = fmt.Sprintf("创建目录: %v", err)
		return nil, out, nil
	}
	if name == "" {
		name = fmt.Sprintf("msg_%d_%s", in.MsgID, extFromMime(mime))
	}
	dst := filepath.Join(dir, name)

	_, err = downloader.NewDownloader().Download(acc.API(), loc).ToPath(ctx, dst)
	if err != nil {
		out.Note = fmt.Sprintf("下载失败: %v", err)
		return nil, out, nil
	}

	info, _ := os.Stat(dst)
	out.FilePath = dst
	out.Mime = mime
	if info != nil {
		out.Size = info.Size()
	}
	out.Note = "已下载"
	return nil, out, nil
}

// handleViewImage 按需查看图片（返回 base64）。
func handleViewImage(ctx context.Context, deps *Deps, in MediaInput, out MediaOutput) (*mcp.CallToolResult, MediaOutput, error) {
	if in.Chat == "" || in.MsgID == 0 {
		out.Note = "view_image 需要 chat 和 msg_id"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	msgs, err := acc.API().MessagesGetMessages(ctx, []tg.InputMessageClass{
		&tg.InputMessageID{ID: in.MsgID},
	})
	if err != nil {
		out.Note = fmt.Sprintf("获取消息失败: %v", err)
		return nil, out, nil
	}
	msgList, _ := extractMessages(msgs)
	if len(msgList) == 0 {
		out.Note = "消息不存在"
		return nil, out, nil
	}

	// 只支持图片
	mp, ok := msgList[0].Media.(*tg.MessageMediaPhoto)
	if !ok {
		out.Note = "该消息不是图片（用 download 下载其他类型）"
		return nil, out, nil
	}
	photo, ok := mp.Photo.(*tg.Photo)
	if !ok {
		out.Note = "图片数据不可用"
		return nil, out, nil
	}

	// 选最大尺寸
	size := largestPhotoSize(photo)
	if size == nil {
		out.Note = "没有可用的图片尺寸"
		return nil, out, nil
	}

	maxSize := in.MaxSize
	if maxSize <= 0 {
		maxSize = 2 * 1024 * 1024 // 2MB
	}
	if int(size.Size) > maxSize {
		out.Note = fmt.Sprintf("图片太大（%d 字节，上限 %d），请用 download 下载或用更小尺寸",
			size.Size, maxSize)
		out.Meta = &dto.MediaMeta{Kind: "photo", Size: int64(size.Size), Width: size.W, Height: size.H}
		return nil, out, nil
	}

	// 下载到内存
	loc := &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     size.Type,
	}
	var buf strings.Builder
	_, err = downloader.NewDownloader().Download(acc.API(), loc).Stream(ctx, &buf)
	if err != nil {
		out.Note = fmt.Sprintf("下载图片失败: %v", err)
		return nil, out, nil
	}

	data := []byte(buf.String())
	out.Base64 = base64.StdEncoding.EncodeToString(data)
	out.MediaType = "image/jpeg"
	out.Size = int64(len(data))
	out.Width = size.W
	out.Height = size.H
	out.Note = fmt.Sprintf("图片 %dx%d，%d 字节（base64 编码）", size.W, size.H, len(data))
	return nil, out, nil
}

// largestPhotoSize 找最大的 PhotoSize。
func largestPhotoSize(p *tg.Photo) *tg.PhotoSize {
	var best *tg.PhotoSize
	for _, s := range p.Sizes {
		switch v := s.(type) {
		case *tg.PhotoSize:
			if best == nil || v.Size > best.Size {
				best = v
			}
		case *tg.PhotoSizeProgressive:
			// 取最后一个尺寸（最大）
			if len(v.Sizes) > 0 {
				size := v.Sizes[len(v.Sizes)-1]
				if best == nil || size > best.Size {
					best = &tg.PhotoSize{Type: v.Type, W: v.W, H: v.H, Size: size}
				}
			}
		}
	}
	return best
}

// mediaLocation 从消息提取下载位置、MIME、文件名。
func mediaLocation(m *tg.Message) (tg.InputFileLocationClass, string, string, error) {
	switch media := m.Media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		if !ok {
			return nil, "", "", fmt.Errorf("图片数据不可用")
		}
		size := largestPhotoSize(photo)
		if size == nil {
			return nil, "", "", fmt.Errorf("没有可用的图片尺寸")
		}
		return &tg.InputPhotoFileLocation{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			ThumbSize:     size.Type,
		}, "image/jpeg", "", nil
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return nil, "", "", fmt.Errorf("文档数据不可用")
		}
		name := ""
		for _, attr := range doc.Attributes {
			if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
				name = fn.FileName
				break
			}
		}
		return &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
			ThumbSize:     "",
		}, doc.MimeType, name, nil
	default:
		return nil, "", "", fmt.Errorf("该消息不含可下载的媒体")
	}
}

// guessMime 根据扩展名猜测 MIME。
func guessMime(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

// extFromMime 从 MIME 猜扩展名。
func extFromMime(mime string) string {
	switch {
	case strings.Contains(mime, "jpeg"):
		return "jpg"
	case strings.Contains(mime, "png"):
		return "png"
	case strings.Contains(mime, "gif"):
		return "gif"
	case strings.Contains(mime, "webp"):
		return "webp"
	case strings.Contains(mime, "mp4"):
		return "mp4"
	case strings.Contains(mime, "mpeg"):
		return "mp3"
	case strings.Contains(mime, "ogg"):
		return "ogg"
	case strings.Contains(mime, "pdf"):
		return "pdf"
	default:
		return "bin"
	}
}
