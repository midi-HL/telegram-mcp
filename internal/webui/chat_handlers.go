package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/autoreply"
	"github.com/midi-HL/telegram-mcp/internal/dto"
	tgclient "github.com/midi-HL/telegram-mcp/internal/telegram"
)

// handleChat 聊天面板页。
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	s.render(w, "chat.html", pageData{
		Title:        "聊天",
		Active:       "chat",
		IsDefaultPwd: s.store.IsDefaultPassword(r.Context()),
	})
}

// handleAutoReply 自动回复配置页。
func (s *Server) handleAutoReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	rs := autoreply.NewRuleStore(s.store)
	rules, err := rs.List(r.Context())
	if err != nil {
		s.render(w, "autoreply.html", pageData{
			Title: "自动回复", Active: "autoreply", Error: err.Error(),
		})
		return
	}
	logs, _ := rs.RecentLogs(r.Context(), 20)
	s.render(w, "autoreply.html", pageData{
		Title:        "自动回复",
		Active:       "autoreply",
		IsDefaultPwd: s.store.IsDefaultPassword(r.Context()),
		Data: map[string]any{
			"Rules":   rules,
			"Logs":    logs,
			"Enabled": s.autoReplyEnabled(),
		},
	})
}

// autoReplyEnabled 返回自动回复开关状态。
func (s *Server) autoReplyEnabled() bool {
	if s.autoreply == nil {
		return false
	}
	return s.autoreply.Enabled()
}

// handleAPIChats 聊天列表 API（Web UI 用）。
func (s *Server) handleAPIChats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc, _, err := s.firstReadyAccount()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}

	res, err := acc.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      50,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var dialogs []tg.DialogClass
	var users []tg.UserClass
	var chats []tg.ChatClass
	switch v := res.(type) {
	case *tg.MessagesDialogs:
		dialogs, users, chats = v.Dialogs, v.Users, v.Chats
	case *tg.MessagesDialogsSlice:
		dialogs, users, chats = v.Dialogs, v.Users, v.Chats
	}

	userMap := make(map[int64]*tg.User)
	for _, uc := range users {
		if u, ok := uc.(*tg.User); ok {
			userMap[u.ID] = u
		}
	}
	chatMap := make(map[int64]tg.ChatClass)
	for _, c := range chats {
		switch ch := c.(type) {
		case *tg.Channel:
			chatMap[ch.ID] = ch
		case *tg.Chat:
			chatMap[ch.ID] = ch
		}
	}

	list := make([]dto.ChatBrief, 0, len(dialogs))
	for _, d := range dialogs {
		dialog, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}
		brief := dto.ChatBrief{Unread: dialog.UnreadCount}
		switch p := dialog.Peer.(type) {
		case *tg.PeerUser:
			if u := userMap[p.UserID]; u != nil {
				brief.ID = u.ID
				brief.Title = displayNameUI(u)
				brief.Type = dto.ChatTypeUser
			}
		case *tg.PeerChat:
			if c, ok := chatMap[p.ChatID].(*tg.Chat); ok {
				brief.ID = c.ID
				brief.Title = c.Title
				brief.Type = dto.ChatTypeBasicGroup
			}
		case *tg.PeerChannel:
			if ch, ok := chatMap[p.ChannelID].(*tg.Channel); ok {
				brief.ID = ch.ID
				brief.Title = ch.Title
				if ch.Megagroup {
					brief.Type = dto.ChatTypeSupergroup
				} else {
					brief.Type = dto.ChatTypeChannel
				}
			}
		}
		if brief.ID != 0 {
			list = append(list, brief)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": list})
}

// handleAPIMessages 消息列表 API。
func (s *Server) handleAPIMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	chatID := parseI64(r.URL.Query().Get("chat_id"))
	if chatID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "需要 chat_id"})
		return
	}
	limit := parseInt(r.URL.Query().Get("limit"), 30)

	acc, resolver, err := s.firstReadyAccountWithResolver()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	peer, err := resolver.Resolve(ctx, fmt.Sprintf("%d", chatID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	res, err := acc.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	msgs, users, _ := extractMessagesUI(res)
	out := make([]dto.MessageBrief, 0, len(msgs))
	for _, m := range msgs {
		b := dto.MessageBrief{
			ID:   m.ID,
			Date: int64(m.Date),
			Text: m.Message,
			Out:  m.Out,
		}
		if m.FromID != nil {
			b.SenderID = tgclient.PeerIDFromClass(m.FromID)
			if u := users[b.SenderID]; u != nil {
				b.Sender = displayNameUI(u)
			}
		}
		out = append(out, b)
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

// handleAPISend 发送消息 API。
func (s *Server) handleAPISend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	ctx := r.Context()
	var req struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if req.ChatID == 0 || req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "需要 chat_id 和 text"})
		return
	}

	acc, resolver, err := s.firstReadyAccountWithResolver()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	peer, err := resolver.Resolve(ctx, fmt.Sprintf("%d", req.ChatID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	res, err := acc.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  req.Text,
		RandomID: time.Now().UnixNano(),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": fmt.Sprintf("%T", res)})
}

// --- 自动回复 API ---

// handleAPIAutoReply 自动回复规则 CRUD。
func (s *Server) handleAPIAutoReply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rs := autoreply.NewRuleStore(s.store)

	switch r.Method {
	case http.MethodGet:
		rules, err := rs.List(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		logs, _ := rs.RecentLogs(ctx, 50)
		writeJSON(w, http.StatusOK, map[string]any{
			"rules":   rules,
			"logs":    logs,
			"enabled": s.autoReplyEnabled(),
		})

	case http.MethodPost:
		var rule autoreply.Rule
		if err := decodeJSON(r, &rule); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		id, err := rs.Save(ctx, &rule)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})

	case http.MethodDelete:
		var req struct {
			ID int64 `json:"id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		if err := rs.Delete(ctx, req.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
	}
}

// handleAPIAutoReplyToggle 开关自动回复。
func (s *Server) handleAPIAutoReplyToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if s.autoreply == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "自动回复引擎未初始化"})
		return
	}
	s.autoreply.SetEnabled(req.Enabled)
	s.logger.Info("自动回复开关变更", zap.Bool("enabled", req.Enabled))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": req.Enabled})
}

// --- 辅助 ---

// firstReadyAccount 返回第一个就绪的账号。
func (s *Server) firstReadyAccount() (*tgclient.Account, string, error) {
	for _, name := range s.pool.Names() {
		acc, ok := s.pool.Get(name)
		if ok && acc != nil && acc.API() != nil && acc.Self() != nil {
			return acc, name, nil
		}
	}
	return nil, "", fmt.Errorf("没有就绪的账号")
}

// firstReadyAccountWithResolver 返回账号和 resolver。
func (s *Server) firstReadyAccountWithResolver() (*tgclient.Account, *tgclient.PeerResolver, error) {
	acc, _, err := s.firstReadyAccount()
	if err != nil {
		return nil, nil, err
	}
	return acc, tgclient.NewPeerResolver(acc.API()), nil
}

// displayNameUI 生成显示名。
func displayNameUI(u *tg.User) string {
	name := u.FirstName
	if u.LastName != "" {
		if name != "" {
			name += " "
		}
		name += u.LastName
	}
	if name == "" {
		if un, ok := u.GetUsername(); ok {
			name = un
		}
	}
	if name == "" {
		name = fmt.Sprintf("用户%d", u.ID)
	}
	return name
}

// extractMessagesUI 提取消息与用户。
func extractMessagesUI(res tg.MessagesMessagesClass) ([]*tg.Message, map[int64]*tg.User, []tg.ChatClass) {
	var msgClasses []tg.MessageClass
	var userClasses []tg.UserClass
	var chats []tg.ChatClass
	switch v := res.(type) {
	case *tg.MessagesMessages:
		msgClasses, userClasses, chats = v.Messages, v.Users, v.Chats
	case *tg.MessagesMessagesSlice:
		msgClasses, userClasses, chats = v.Messages, v.Users, v.Chats
	case *tg.MessagesChannelMessages:
		msgClasses, userClasses, chats = v.Messages, v.Users, v.Chats
	}
	users := make(map[int64]*tg.User)
	for _, uc := range userClasses {
		if u, ok := uc.(*tg.User); ok {
			users[u.ID] = u
		}
	}
	msgs := make([]*tg.Message, 0, len(msgClasses))
	for _, mc := range msgClasses {
		if m, ok := mc.(*tg.Message); ok {
			msgs = append(msgs, m)
		}
	}
	return msgs, users, chats
}

// parseI64 解析 int64。
func parseI64(s string) int64 {
	var n int64
	neg := false
	for i, c := range s {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n
}

// parseInt 解析 int。
func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}

var _ = context.Background

// handleASRPage ASR 配置页。
func (s *Server) handleASRPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	modelDir := s.cfg.ASR.ResolveModelDir(s.cfg.DataDir)
	cliPath := s.cfg.ASR.ResolveCLIPath()

	s.render(w, "asr.html", pageData{
		Title:        "语音转写",
		Active:       "asr",
		IsDefaultPwd: s.store.IsDefaultPassword(ctx),
		Data: map[string]any{
			"Backend":    s.cfg.ASR.Backend,
			"ModelDir":   modelDir,
			"CLIPath":    cliPath,
			"ModelFound": findModelFileUI(modelDir) != "",
			"CLIFound":   cliPath != "" && fileExistsUI(cliPath),
		},
	})
}

// handleAPIASR ASR 配置 API。
func (s *Server) handleAPIASR(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		modelDir := s.cfg.ASR.ResolveModelDir(s.cfg.DataDir)
		cliPath := s.cfg.ASR.ResolveCLIPath()
		writeJSON(w, http.StatusOK, map[string]any{
			"backend":     s.cfg.ASR.Backend,
			"model_dir":   modelDir,
			"cli_path":    cliPath,
			"api_key_set": s.cfg.ASR.APIKey != "",
			"endpoint":    s.cfg.ASR.Endpoint,
			"language":    s.cfg.ASR.Language,
			"model_found": findModelFileUI(modelDir) != "",
			"cli_found":   cliPath != "" && fileExistsUI(cliPath),
		})

	case http.MethodPost:
		var req struct {
			Backend  string `json:"backend"`
			ModelDir string `json:"model_dir"`
			CLIPath  string `json:"cli_path"`
			APIKey   string `json:"api_key"`
			Endpoint string `json:"endpoint"`
			Language string `json:"language"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}

		// 校验后端
		switch req.Backend {
		case "none", "sherpa", "openai", "native":
		case "":
			req.Backend = s.cfg.ASR.Backend
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "未知后端，支持 none/sherpa/openai/native",
			})
			return
		}

		// sherpa 校验路径
		if req.Backend == "sherpa" {
			if req.ModelDir != "" && findModelFileUI(req.ModelDir) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "模型目录里找不到模型文件（model.onnx / encoder.onnx 等）",
				})
				return
			}
			if req.CLIPath != "" && !fileExistsUI(req.CLIPath) {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "CLI 路径不存在：" + req.CLIPath,
				})
				return
			}
		}

		s.cfg.ASR.Backend = req.Backend
		if req.ModelDir != "" {
			s.cfg.ASR.ModelDir = req.ModelDir
		}
		if req.CLIPath != "" {
			s.cfg.ASR.CLIPath = req.CLIPath
		}
		if req.APIKey != "" {
			s.cfg.ASR.APIKey = req.APIKey
		}
		if req.Endpoint != "" {
			s.cfg.ASR.Endpoint = req.Endpoint
		}
		if req.Language != "" {
			s.cfg.ASR.Language = req.Language
		}

		if err := s.cfg.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.logger.Info("ASR 配置已更新",
			zap.String("backend", req.Backend),
			zap.String("model_dir", req.ModelDir),
			zap.String("cli", req.CLIPath))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
	}
}

// findModelFileUI 查找模型文件。
func findModelFileUI(dir string) string {
	if dir == "" {
		return ""
	}
	for _, name := range []string{"model.onnx", "model.int8.onnx", "encoder.onnx", "encoder.int8.onnx", "paraformer.onnx"} {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// fileExistsUI 检查文件存在。
func fileExistsUI(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
