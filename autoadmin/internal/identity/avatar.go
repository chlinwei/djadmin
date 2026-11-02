package identity

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	"golang.org/x/image/riff"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
	_ "golang.org/x/image/webp"
)

const (
	avatarDirectory   = "userAvatar"
	avatarUploadLimit = 5 << 20
	avatarStoredLimit = 1 << 20
	avatarMaxSide     = 1024
	avatarMaxPixels   = 32 << 20
)

// processAvatar checks dimensions before decoding to bound memory even for compressed image bombs.
// PNG re-encoding strips metadata and preserves transparency; GIF images use their first frame.
func processAvatar(source []byte) ([]byte, error) {
	if len(source) == 0 || len(source) > avatarUploadLimit {
		return nil, fmt.Errorf("头像原文件不得为空或超过 5 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("无法识别头像图片：%w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 ||
		int64(config.Width)*int64(config.Height) > avatarMaxPixels {
		return nil, fmt.Errorf("头像尺寸不得超过 8192×8192，且总像素不得超过 33554432")
	}
	if format == "webp" {
		if err := validateAvatarWebP(source, config); err != nil {
			return nil, err
		}
	}
	original, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("头像图片损坏：%w", err)
	}
	width, height := config.Width, config.Height
	if width > avatarMaxSide || height > avatarMaxSide {
		if width >= height {
			height = max(1, height*avatarMaxSide/width)
			width = avatarMaxSide
		} else {
			width = max(1, width*avatarMaxSide/height)
			height = avatarMaxSide
		}
	}
	for {
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), original, original.Bounds(), draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, resized); err != nil {
			return nil, fmt.Errorf("头像压缩失败：%w", err)
		}
		if encoded.Len() <= avatarStoredLimit {
			return encoded.Bytes(), nil
		}
		// Check encoded bytes, not uploaded bytes; lossless PNG may exceed 1 MiB even at 1024px.
		width, height = max(1, width*3/4), max(1, height*3/4)
	}
}

// WebP's optional canvas header can disagree with the actual frame; validate both before allocation.
func validateAvatarWebP(source []byte, canvas image.Config) error {
	_, chunks, err := riff.NewReader(bytes.NewReader(source))
	if err != nil {
		return fmt.Errorf("头像 WebP 容器无效：%w", err)
	}
	for {
		id, size, data, err := chunks.Next()
		if err != nil {
			return fmt.Errorf("头像 WebP 帧无效：%w", err)
		}
		if size > uint32(len(source)) {
			return fmt.Errorf("头像 WebP 帧长度无效")
		}
		var width, height int
		switch string(id[:]) {
		case "VP8X":
			var header [10]byte
			if _, err := io.ReadFull(data, header[:]); err != nil {
				return fmt.Errorf("头像 WebP 头无效：%w", err)
			}
			if header[0]&2 != 0 {
				return fmt.Errorf("暂不支持 WebP 动画，请上传静态图片")
			}
			continue
		case "VP8 ":
			decoder := vp8.NewDecoder()
			decoder.Init(data, int(size))
			frame, err := decoder.DecodeFrameHeader()
			if err != nil {
				return fmt.Errorf("头像 WebP 帧无效：%w", err)
			}
			width, height = frame.Width, frame.Height
		case "VP8L":
			frame, err := vp8l.DecodeConfig(data)
			if err != nil {
				return fmt.Errorf("头像 WebP 帧无效：%w", err)
			}
			width, height = frame.Width, frame.Height
		default:
			continue
		}
		if width != canvas.Width || height != canvas.Height {
			return fmt.Errorf("头像 WebP 画布与图片帧尺寸不一致")
		}
		return nil
	}
}

func newAvatarFilename() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]) + ".png", nil
}

func (handler *Handler) ChangeAvatar(c *gin.Context) {
	claims, ok := ClaimsFromContext(c)
	if !ok {
		response.Error(c, apperror.ErrTokenInvalid)
		return
	}
	// Multipart framing has a separate budget, so a valid 5 MiB image is not rejected for its headers.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, avatarUploadLimit+(64<<10))
	file, err := c.FormFile("avatar")
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		response.BusinessError(c, 400, "请选择头像文件，原文件不得超过 5 MiB", nil)
		return
	}
	switch strings.ToLower(filepath.Ext(file.Filename)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
	default:
		response.BusinessError(c, 400, "仅支持 png/jpg/jpeg/gif/webp 图片", nil)
		return
	}
	opened, err := file.Open()
	if err != nil {
		response.Error(c, fmt.Errorf("读取头像上传文件：%w", err))
		return
	}
	defer opened.Close()
	source, err := io.ReadAll(io.LimitReader(opened, avatarUploadLimit+1))
	if err != nil {
		response.Error(c, fmt.Errorf("读取头像内容：%w", err))
		return
	}
	content, err := processAvatar(source)
	if err != nil {
		response.BusinessError(c, 400, err.Error(), nil)
		return
	}
	filename, err := newAvatarFilename()
	if err != nil {
		response.Error(c, err)
		return
	}
	if _, err := handler.service.repository.saveAvatar(c.Request.Context(), claims.UserID, filename, content, nil); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{
		"new_file_name": filename,
		"avatar":        avatarDirectory + "/" + filename,
		"avatar_url":    "/media/" + avatarDirectory + "/" + filename,
	})
}

// ServeMedia keeps existing public media URLs, but avatar bytes have only one source: the database.
func (handler *Handler) ServeMedia(mediaRoot string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clean := path.Clean("/" + c.Param("filepath"))
		if !strings.HasPrefix(clean, "/"+avatarDirectory+"/") && clean != "/"+avatarDirectory {
			c.FileFromFS(clean, http.Dir(mediaRoot))
			return
		}
		filename := strings.TrimPrefix(clean, "/"+avatarDirectory+"/")
		if filename == "" || strings.ContainsAny(filename, `/\`) {
			c.Status(http.StatusNotFound)
			return
		}
		avatar, err := handler.service.repository.queries.GetUserAvatarContent(c.Request.Context(), filename)
		if errors.Is(err, sql.ErrNoRows) {
			c.Status(http.StatusNotFound)
			return
		}
		if err != nil {
			response.Error(c, err)
			return
		}
		c.Header("Content-Type", "image/png")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "public, max-age=3600")
		c.Header("ETag", `"`+filename+`"`)
		http.ServeContent(c.Writer, c.Request, filename, avatar.UpdateTime, bytes.NewReader(avatar.Content))
	}
}
