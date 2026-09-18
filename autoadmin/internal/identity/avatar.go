package identity

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

// 用户头像上传（POST /user/changeAvatar）：文件存 <media>/userAvatar/<时间戳><后缀>，
// 成功后把文件名写回 sys_user.avatar 并删除旧头像，返回 {new_file_name, avatar, avatar_url}。
// avatar_url 是相对地址，静态文件由 router 的 GET /media/* 提供（autoadmin/media）。
const avatarDirectory = "userAvatar"

func (handler *Handler) ChangeAvatar(context *gin.Context) {
	claims, ok := ClaimsFromContext(context)
	if !ok {
		response.Error(context, apperror.ErrTokenInvalid)
		return
	}
	// 5MB 上限：必须在解析 multipart 前限制请求体，否则 FormFile 已经把大文件读进内存/临时盘。
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 5<<20)
	file, err := context.FormFile("avatar")
	if err != nil {
		response.BusinessError(context, 400, "请选择头像文件", nil)
		return
	}
	// 后缀白名单校验，防止任意文件写入 media 目录。
	suffix := strings.ToLower(filepath.Ext(file.Filename))
	switch suffix {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		response.BusinessError(context, 400, "仅支持 png/jpg/jpeg/gif/webp 图片", nil)
		return
	}
	newFileName := time.Now().Format("20060102150405") + suffix
	directory, err := filepath.Abs(filepath.Join("media", avatarDirectory))
	if err != nil {
		response.Error(context, err)
		return
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		response.Error(context, err)
		return
	}
	opened, err := file.Open()
	if err != nil {
		response.BusinessError(context, 400, "读取上传文件失败", nil)
		return
	}
	defer opened.Close()
	destination, err := os.Create(filepath.Join(directory, newFileName))
	if err != nil {
		response.Error(context, err)
		return
	}
	defer destination.Close()
	if _, err = io.Copy(destination, io.LimitReader(opened, 5<<20+1)); err != nil {
		_ = os.Remove(filepath.Join(directory, newFileName))
		response.BusinessError(context, 400, "头像保存失败", nil)
		return
	}
	previous := handler.currentAvatarFileName(context.Request.Context(), claims.UserID)
	if err := handler.service.UpdateAvatar(context.Request.Context(), claims.UserID, newFileName); err != nil {
		_ = os.Remove(filepath.Join(directory, newFileName))
		response.Error(context, apperror.WithCause(ErrUserQueryInternal, err))
		return
	}
	// 删除旧头像文件失败不影响本次上传结论（旧文件可能已被手工清理）。
	if previous != "" && previous != newFileName {
		_ = os.Remove(filepath.Join(directory, filepath.Base(previous)))
	}
	response.Success(context, gin.H{
		"new_file_name": newFileName,
		"avatar":        newFileName,
		"avatar_url":    "/media/" + avatarDirectory + "/" + newFileName,
	})
}

// currentAvatarFileName 取用户当前头像文件名（可能是历史遗留的完整路径），失败时返回空。
func (handler *Handler) currentAvatarFileName(ctx context.Context, userID int32) string {
	user, err := handler.service.GetByID(ctx, userID)
	if err != nil || !user.Avatar.Valid {
		return ""
	}
	return strings.TrimSpace(user.Avatar.String)
}
