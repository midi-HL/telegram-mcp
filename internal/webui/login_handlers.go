package webui

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/midi-HL/telegram-mcp/internal/config"
)

// handleAPICredentials 读写 API 凭据。
func (s *Server) handleAPICredentials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// 返回是否已配置（不返回 hash 本身，避免泄露）
		accs := make([]map[string]any, 0, len(s.cfg.Accounts))
		for _, a := range s.cfg.Accounts {
			accs = append(accs, map[string]any{
				"name":         a.Name,
				"api_id":       a.APIID,
				"api_hash_set": a.APIHash != "",
				"is_builtin":   config.IsBuiltinCredentials(a.APIID, a.APIHash),
				"phone":        maskPhone(a.Phone),
				"session_dir":  a.SessionDir,
			})
		}
		envOverride := false
		if _, ok := config.EnvCredentialPair(); ok {
			envOverride = true
		}
		cred := s.cfg.EffectiveCredentials(s.cfg.DefaultAccount)
		writeJSON(w, http.StatusOK, map[string]any{
			"accounts":          accs,
			"builtin_api_id":    config.BuiltinAPIID,
			"builtin_api_hash":  config.BuiltinAPIHash,
			"effective_api_id":  cred.APIID,
			"credential_source": cred.Source,
			"env_override":      envOverride,
		})

	case http.MethodPost:
		var req struct {
			Account string `json:"account"`
			APIID   int    `json:"api_id"`
			APIHash string `json:"api_hash"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
			return
		}
		if req.APIID < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "api_id 不能为负数"})
			return
		}
		name := req.Account
		if name == "" {
			name = "main"
		}

		// 留空即用公开内置默认凭据；填写则替换。
		// 环境变量只在运行时覆盖，不写进 config.json。
		fill := config.Account{Name: name, APIID: req.APIID, APIHash: req.APIHash}
		config.FillBuiltinCredentials(&fill)

		// 保留已有的 phone/session 配置
		existing := s.cfg.Account(name)
		newAcc := config.Account{
			Name:    name,
			APIID:   fill.APIID,
			APIHash: fill.APIHash,
		}
		if existing != nil {
			newAcc.Phone = existing.Phone
			newAcc.SessionDir = existing.SessionDir
		}
		s.cfg.UpsertAccount(newAcc)

		if err := s.cfg.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		effective := s.cfg.EffectiveCredentials(name)
		s.logger.Info("API 凭据已更新", zap.String("account", name), zap.Int("api_id", fill.APIID))
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":           true,
			"account":      name,
			"api_id":       effective.APIID,
			"source":       effective.Source,
			"is_builtin":   config.IsBuiltinCredentials(fill.APIID, fill.APIHash),
			"env_override": effective.Source == "env",
		})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
	}
}

// maskPhone 隐藏手机号中间部分。
func maskPhone(p string) string {
	if len(p) <= 6 {
		return p
	}
	return p[:4] + "****" + p[len(p)-2:]
}

// handleAPILoginQR 启动扫码登录。
func (s *Server) handleAPILoginQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	if s.loginMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "登录管理器未初始化"})
		return
	}
	var req struct {
		Account string `json:"account"`
		APIID   int    `json:"api_id"`
		APIHash string `json:"api_hash"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}

	cred := s.resolveLoginCredentials(req.Account, req.APIID, req.APIHash)
	req.APIID, req.APIHash = cred.APIID, cred.APIHash

	sess, err := s.loginMgr.StartQR(r.Context(), req.Account, req.APIID, req.APIHash)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sess.Snap())
}

// handleAPILoginPhone 启动手机号登录。
func (s *Server) handleAPILoginPhone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	if s.loginMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "登录管理器未初始化"})
		return
	}
	var req struct {
		Account string `json:"account"`
		APIID   int    `json:"api_id"`
		APIHash string `json:"api_hash"`
		Phone   string `json:"phone"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	cred := s.resolveLoginCredentials(req.Account, req.APIID, req.APIHash)
	req.APIID, req.APIHash = cred.APIID, cred.APIHash

	sess, err := s.loginMgr.StartPhone(r.Context(), req.Account, req.APIID, req.APIHash, req.Phone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sess.Snap())
}

// handleAPILoginBot 用 Bot Token 登录。
func (s *Server) handleAPILoginBot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	var req struct {
		Account  string `json:"account"`
		APIID    int    `json:"api_id"`
		APIHash  string `json:"api_hash"`
		BotToken string `json:"bot_token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if req.BotToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "需要 bot_token"})
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"error": "Bot Token 登录需要 Bot API 模式，当前为 MTProto 模式。" +
			"请用 @BotFather 创建 Bot 后用二维码/手机号登录主账号，再用 bot 工具管理 Bot。",
	})
}

// handleAPILoginStatus 查询登录状态。
func (s *Server) handleAPILoginStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "需要 id"})
		return
	}
	if s.loginMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "登录管理器未初始化"})
		return
	}
	sess, ok := s.loginMgr.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "会话不存在或已过期"})
		return
	}
	writeJSON(w, http.StatusOK, sess.Snap())
}

// handleAPILoginCode 提交验证码。
func (s *Server) handleAPILoginCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	var req struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.loginMgr.SubmitCode(req.ID, req.Code); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAPILoginPassword 提交两步验证密码。
func (s *Server) handleAPILoginPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	var req struct {
		ID       string `json:"id"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.loginMgr.SubmitPassword(req.ID, req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAPILoginCancel 取消登录。
func (s *Server) handleAPILoginCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "方法不允许"})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求格式错误"})
		return
	}
	if err := s.loginMgr.Cancel(req.ID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// resolveLoginCredentials 按「请求参数 → 已保存配置 → 内置凭据」解析，
// 环境变量始终由 config.ResolveCredentials 保持最高优先级。
func (s *Server) resolveLoginCredentials(account string, apiID int, apiHash string) config.Credentials {
	if acc := s.cfg.Account(defaultIfEmpty(account, s.cfg.DefaultAccount)); acc != nil {
		if apiID == 0 {
			apiID = acc.APIID
		}
		if apiHash == "" {
			apiHash = acc.APIHash
		}
	}
	return config.ResolveCredentials(apiID, apiHash)
}

// defaultIfEmpty 空则返回默认值。
func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

var _ = fmt.Sprintf
