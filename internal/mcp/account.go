package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/midi-HL/telegram-mcp/internal/dto"
)

// AccountInput 是 account 工具的输入。
type AccountInput struct {
	// Action: profile / set_name / set_bio / set_username / set_photo / sessions / delete_photo
	Action    string `json:"action" jsonschema:"profile|set_name|set_bio|set_username|set_photo|delete_photo|sessions"`
	FirstName string `json:"first_name,omitempty" jsonschema:"first name"`
	LastName  string `json:"last_name,omitempty" jsonschema:"last name"`
	Bio       string `json:"bio,omitempty" jsonschema:"bio"`
	Username  string `json:"username,omitempty" jsonschema:"username without @"`
	PhotoPath string `json:"photo_path,omitempty" jsonschema:"local photo path"`
	Account   string `json:"account,omitempty" jsonschema:"account name"`
}

// SessionInfo 是登录设备信息。
type SessionInfo struct {
	Hash      int64  `json:"hash"`
	Device    string `json:"device"`
	Platform  string `json:"platform"`
	AppName   string `json:"app,omitempty"`
	IP        string `json:"ip,omitempty"`
	Country   string `json:"country,omitempty"`
	Created   int64  `json:"created"`
	Active    int64  `json:"active,omitempty"`
	Current   bool   `json:"current,omitempty"`
	Official  bool   `json:"official,omitempty"`
}

// AccountOutput 是 account 工具的输出。
type AccountOutput struct {
	Action   string        `json:"action"`
	Profile  *dto.UserProfile `json:"profile,omitempty"`
	Sessions []SessionInfo `json:"sessions,omitempty"`
	Count    int           `json:"count,omitempty"`
	Note     string        `json:"note,omitempty"`
}

// registerAccountTools 注册 account 工具。
func registerAccountTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "account",
		Description: "自己的账号。action: profile 资料 / set_name 改名 / set_bio 改简介 / set_username 改用户名 / set_photo 改头像 / delete_photo 删头像 / sessions 登录设备",
		Annotations: &mcp.ToolAnnotations{Title: "账号", ReadOnlyHint: false},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in AccountInput) (*mcp.CallToolResult, AccountOutput, error) {
		return handleAccount(ctx, deps, in)
	})
}

func handleAccount(ctx context.Context, deps *Deps, in AccountInput) (*mcp.CallToolResult, AccountOutput, error) {
	out := AccountOutput{Action: in.Action}

	switch in.Action {
	case "", "profile":
		out.Action = "profile"
		return handleAccountProfile(ctx, deps, in, out)
	case "set_name":
		return handleSetName(ctx, deps, in, out)
	case "set_bio":
		return handleSetBio(ctx, deps, in, out)
	case "set_username":
		return handleSetUsername(ctx, deps, in, out)
	case "set_photo":
		return handleSetPhoto(ctx, deps, in, out)
	case "delete_photo":
		return handleDeletePhoto(ctx, deps, in, out)
	case "sessions":
		return handleSessions(ctx, deps, in, out)
	default:
		out.Note = fmt.Sprintf("未知 action %q", in.Action)
		return nil, out, nil
	}
}

// handleAccountProfile 查看自己的资料。
func handleAccountProfile(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	self, err := acc.API().UsersGetFullUser(ctx, &tg.InputUserSelf{})
	if err != nil {
		out.Note = fmt.Sprintf("获取资料失败: %v", err)
		return nil, out, nil
	}
	var u *tg.User
	for _, uc := range self.Users {
		if user, ok := uc.(*tg.User); ok {
			u = user
			break
		}
	}
	if u == nil {
		out.Note = "资料为空"
		return nil, out, nil
	}
	out.Profile = projectUserProfile(u, &self.FullUser)
	out.Note = "ok"
	return nil, out, nil
}

// handleSetName 改名字。
func handleSetName(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	if in.FirstName == "" {
		out.Note = "set_name 需要 first_name"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	_, err = acc.API().AccountUpdateProfile(ctx, &tg.AccountUpdateProfileRequest{
		FirstName: in.FirstName,
		LastName:  in.LastName,
	})
	if err != nil {
		out.Note = fmt.Sprintf("改名失败: %v", err)
		return nil, out, nil
	}
	out.Note = fmt.Sprintf("已改名为「%s %s」", in.FirstName, in.LastName)
	return nil, out, nil
}

// handleSetBio 改简介。
func handleSetBio(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	_, err = acc.API().AccountUpdateProfile(ctx, &tg.AccountUpdateProfileRequest{
		About: in.Bio,
	})
	if err != nil {
		out.Note = fmt.Sprintf("改简介失败: %v", err)
		return nil, out, nil
	}
	out.Note = "简介已更新"
	return nil, out, nil
}

// handleSetUsername 改用户名。
func handleSetUsername(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	if in.Username == "" {
		out.Note = "set_username 需要 username"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	_, err = acc.API().AccountUpdateUsername(ctx, in.Username)
	if err != nil {
		out.Note = fmt.Sprintf("改用户名失败: %v（可能已被占用）", err)
		return nil, out, nil
	}
	out.Note = fmt.Sprintf("用户名已改为 @%s", in.Username)
	return nil, out, nil
}

// handleSetPhoto 改头像。
func handleSetPhoto(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	if in.PhotoPath == "" {
		out.Note = "set_photo 需要 photo_path"
		return nil, out, nil
	}
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	f, err := os.Open(in.PhotoPath)
	if err != nil {
		out.Note = fmt.Sprintf("打开图片: %v", err)
		return nil, out, nil
	}
	defer func() { _ = f.Close() }()

	up := uploader.NewUploader(acc.API()).WithThreads(2)
	file, err := up.FromReader(ctx, "photo.jpg", f)
	if err != nil {
		out.Note = fmt.Sprintf("上传失败: %v", err)
		return nil, out, nil
	}
	_, err = acc.API().PhotosUploadProfilePhoto(ctx, &tg.PhotosUploadProfilePhotoRequest{
		File: file,
	})
	if err != nil {
		out.Note = fmt.Sprintf("设置头像失败: %v", err)
		return nil, out, nil
	}
	out.Note = "头像已更新"
	return nil, out, nil
}

// handleDeletePhoto 删除头像。
func handleDeletePhoto(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	// 先拿到当前头像
	self, err := acc.API().UsersGetFullUser(ctx, &tg.InputUserSelf{})
	if err != nil {
		out.Note = fmt.Sprintf("获取资料: %v", err)
		return nil, out, nil
	}
	var u *tg.User
	for _, uc := range self.Users {
		if user, ok := uc.(*tg.User); ok {
			u = user
			break
		}
	}
	if u == nil {
		out.Note = "资料为空"
		return nil, out, nil
	}
	photo, ok := u.Photo.(*tg.UserProfilePhoto)
	if !ok {
		out.Note = "当前没有头像"
		return nil, out, nil
	}
	_, err = acc.API().PhotosDeletePhotos(ctx, []tg.InputPhotoClass{
		&tg.InputPhoto{
			ID:            photo.PhotoID,
			AccessHash:    int64(photo.DCID),
			FileReference: photo.StrippedThumb,
		},
	})
	if err != nil {
		out.Note = fmt.Sprintf("删除头像失败: %v", err)
		return nil, out, nil
	}
	out.Note = "头像已删除"
	return nil, out, nil
}

// handleSessions 列出登录设备。
func handleSessions(ctx context.Context, deps *Deps, in AccountInput, out AccountOutput) (*mcp.CallToolResult, AccountOutput, error) {
	acc, _, _, err := deps.withAccount(ctx, in.Account)
	if err != nil {
		out.Note = err.Error()
		return nil, out, nil
	}
	res, err := acc.API().AccountGetAuthorizations(ctx)
	if err != nil {
		out.Note = fmt.Sprintf("获取设备列表失败: %v", err)
		return nil, out, nil
	}
	for _, a := range res.Authorizations {
		out.Sessions = append(out.Sessions, SessionInfo{
			Hash:     a.Hash,
			Device:   a.DeviceModel,
			Platform: a.Platform,
			AppName:  a.AppName,
			IP:       a.IP,
			Country:  a.Country,
			Created:  int64(a.DateCreated),
			Active:   int64(a.DateActive),
			Current:  a.Current,
			Official: a.OfficialApp,
		})
	}
	out.Count = len(out.Sessions)
	out.Note = fmt.Sprintf("共 %d 个登录设备", out.Count)
	return nil, out, nil
}
