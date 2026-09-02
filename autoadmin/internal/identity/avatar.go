package identity

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 用户头像上传（POST /user/changeAvatar），行为与 Django ChangeAvatarView 一致：
// 文件存 <MEDIA_ROOT>/userAvatar/<时间戳><后缀>，只返回文件名，不更新用户记录。
// MEDIA_ROOT 与 monitor 模块共用（backend/djadmin/media）。
func (handler *Handler) ChangeAvatar(context *gin.Context) {
	file, err := context.FormFile("avatar")
	if err != nil {
		response.BusinessError(context, 400, "请选择头像文件", nil)
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 5<<20)
	// 后缀白名单校验（Django 未校验，这里收紧防止任意文件写入 media 目录）。
	suffix := strings.ToLower(filepath.Ext(file.Filename))
	switch suffix {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		response.BusinessError(context, 400, "仅支持 png/jpg/jpeg/gif/webp 图片", nil)
		return
	}
	newFileName := time.Now().Format("20060102150405") + suffix
	directory, err := filepath.Abs(filepath.Join("..", "backend", "djadmin", "media", "userAvatar"))
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
	response.Success(context, gin.H{"new_file_name": newFileName})
}
