package webui

import (
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

// handleAPIWake 读取/保存 Agent 唤醒配置。
func (s *Server) handleAPIWake(w http.ResponseWriter, r *http.Request) {
	if s.wakeEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "唤醒引擎未初始化"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": s.wakeEngine.Config(),
			"stats":  s.wakeEngine.Stats(),
		})
	case http.MethodPost:
		var req config.Wake
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		config.NormalizeWake(&req)
		if req.Mode == "webhook" || req.Mode == "all" {
			if err := validateWebhook(req.WebhookURL); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
		}
		s.wakeEngine.UpdateConfig(req)
		s.cfg.Wake = req
		if err := s.cfg.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "保存配置失败: " + err.Error()})
			return
		}
		s.logger.Info("Agent 唤醒配置已更新",
			zap.Bool("enabled", req.Enabled), zap.String("mode", req.Mode))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": req, "stats": s.wakeEngine.Stats()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
	}
}

// handleAPIWakeToggle 只切换开关并持久化。
func (s *Server) handleAPIWakeToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	if s.wakeEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "唤醒引擎未初始化"})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	cfg := s.wakeEngine.Config()
	cfg.Enabled = req.Enabled
	s.wakeEngine.UpdateConfig(cfg)
	s.cfg.Wake = cfg
	if err := s.cfg.Save(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "保存配置失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": req.Enabled})
}

// handleAPIWakeTest 投递一条合成事件验证链路。
func (s *Server) handleAPIWakeTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	if s.wakeEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "唤醒引擎未初始化"})
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &req); err != nil && r.ContentLength > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	note := s.wakeEngine.Test(r.Context(), req.Text)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "notification": note, "stats": s.wakeEngine.Stats()})
}

// handleAPIWakePending 返回待读队列。
func (s *Server) handleAPIWakePending(w http.ResponseWriter, r *http.Request) {
	if s.wakeEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "唤醒引擎未初始化"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "notifications": s.wakeEngine.Pending()})
}

// handleAPIWakeAck 清空待读队列。
func (s *Server) handleAPIWakeAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	if s.wakeEngine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "唤醒引擎未初始化"})
		return
	}
	n := s.wakeEngine.Ack()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": n})
}

func validateWebhook(raw string) error {
	if raw == "" {
		return &webhookError{"webhook/all 模式需要填写 Webhook URL"}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(raw, " \n\r\t") {
		return &webhookError{"Webhook URL 必须是完整的 http:// 或 https:// 地址"}
	}
	return nil
}

type webhookError struct{ msg string }

func (e *webhookError) Error() string { return e.msg }
