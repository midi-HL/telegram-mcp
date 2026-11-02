package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/asr"
)

// ASRInput 是 asr 工具的输入。
type ASRInput struct {
	// Action: transcribe / backend
	Action  string `json:"action" jsonschema:"transcribe|backend"`
	Chat    string `json:"chat,omitempty" jsonschema:"chat id"`
	MsgID   int    `json:"msg_id,omitempty" jsonschema:"message id with voice/audio"`
	Path    string `json:"path,omitempty" jsonschema:"local audio file path"`
	Backend string `json:"backend,omitempty" jsonschema:"override backend: sherpa|openai|native|none"`
	Account string `json:"account,omitempty" jsonschema:"account name"`
}

// ASROutput 是 asr 工具的输出。
type ASROutput struct {
	Action      string `json:"action"`
	Text        string `json:"text,omitempty"`
	Backend     string `json:"backend,omitempty"`
	Language    string `json:"language,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	AudioBase64 string `json:"audio_base64,omitempty"`
	Note        string `json:"note,omitempty"`
}

// registerASRTools 注册 asr 工具。
func registerASRTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "asr",
		Description: "语音转文字。action: transcribe 转写语音消息 / backend 查看当前后端",
		Annotations: &mcp.ToolAnnotations{Title: "语音转写", ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ASRInput) (*mcp.CallToolResult, ASROutput, error) {
		return handleASR(ctx, deps, in)
	})
}

func handleASR(ctx context.Context, deps *Deps, in ASRInput) (*mcp.CallToolResult, ASROutput, error) {
	out := ASROutput{Action: in.Action}

	switch in.Action {
	case "", "transcribe":
		out.Action = "transcribe"
		return handleTranscribe(ctx, deps, in, out)
	case "backend":
		out.Action = "backend"
		out.Backend = deps.Config.ASR.Backend
		modelDir := deps.Config.ASR.ResolveModelDir(deps.Config.DataDir)
		cliPath := deps.Config.ASR.ResolveCLIPath()
		out.Note = fmt.Sprintf("后端: %s | 模型目录: %s | CLI: %s",
			deps.Config.ASR.Backend, modelDir, cliPath)
		return nil, out, nil
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleTranscribe 转写语音。
func handleTranscribe(ctx context.Context, deps *Deps, in ASRInput, out ASROutput) (*mcp.CallToolResult, ASROutput, error) {
	var audio []byte
	var mime string

	// 来源 1：本地文件
	if in.Path != "" {
		data, err := os.ReadFile(in.Path)
		if err != nil {
			out.Note = fmt.Sprintf("读取文件失败: %v", err)
			return nil, out, nil
		}
		audio = data
		mime = guessMime(in.Path)
	} else if in.Chat != "" && in.MsgID != 0 {
		// 来源 2：Telegram 语音消息
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
		data, m, err := downloadVoice(ctx, acc.API(), msgList[0], deps.Config.DataDir)
		if err != nil {
			out.Note = err.Error()
			return nil, out, nil
		}
		audio = data
		mime = m
	} else {
		out.Note = "transcribe 需要 chat+msg_id 或 path"
		return nil, out, nil
	}

	if len(audio) == 0 {
		out.Note = "音频为空"
		return nil, out, nil
	}

	// 选择后端
	backendName := in.Backend
	if backendName == "" {
		backendName = deps.Config.ASR.Backend
	}
	// 路径按优先级解析（环境变量 > 配置 > 默认）
	tr, err := asr.New(asr.Config{
		Backend:  backendName,
		CLIPath:  deps.Config.ASR.ResolveCLIPath(),
		ModelDir: deps.Config.ASR.ResolveModelDir(deps.Config.DataDir),
		APIKey:   deps.Config.ASR.APIKey,
		Endpoint:   deps.Config.ASR.Endpoint,
		Language:   deps.Config.ASR.Language,
		FFmpegPath: deps.Config.ASR.FFmpegPath,
		NumThreads: deps.Config.ASR.NumThreads,
	})
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}

	res, err := tr.Transcribe(ctx, audio, mime)
	if err != nil {
		out.Note = fmt.Sprintf("转写失败: %v", err)
		return nil, out, nil
	}

	out.Text = res.Text
	out.Backend = res.Backend
	out.Language = res.Language
	out.Duration = res.Duration
	out.AudioBase64 = res.AudioBase64
	out.Note = res.Note
	if out.Text == "" && out.Note == "" {
		out.Note = fmt.Sprintf("已用 %s 转写（%d 字节音频）", res.Backend, len(audio))
	}
	return nil, out, nil
}

// downloadVoice 下载语音消息的音频。
func downloadVoice(ctx context.Context, api *tg.Client, m *tg.Message, dataDir string) ([]byte, string, error) {
	media, ok := m.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, "", fmt.Errorf("该消息不是语音/音频")
	}
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return nil, "", fmt.Errorf("文档数据不可用")
	}

	// 检查是否是语音
	isVoice := false
	for _, attr := range doc.Attributes {
		if a, ok := attr.(*tg.DocumentAttributeAudio); ok && a.Voice {
			isVoice = true
			break
		}
	}
	if !isVoice {
		// 也允许普通音频
		for _, attr := range doc.Attributes {
			if _, ok := attr.(*tg.DocumentAttributeAudio); ok {
				isVoice = true
				break
			}
		}
	}
	if !isVoice {
		return nil, "", fmt.Errorf("该消息不是语音或音频")
	}

	// 下载到临时文件再读
	dir := filepath.Join(dataDir, "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", err
	}
	dst := filepath.Join(dir, fmt.Sprintf("voice_%d.ogg", m.ID))

	loc := &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		ThumbSize:     "",
	}
	if _, err := downloader.NewDownloader().Download(api, loc).ToPath(ctx, dst); err != nil {
		return nil, "", fmt.Errorf("下载语音失败: %w", err)
	}
	defer func() { _ = os.Remove(dst) }()

	data, err := os.ReadFile(dst)
	if err != nil {
		return nil, "", err
	}
	return data, doc.MimeType, nil
}
