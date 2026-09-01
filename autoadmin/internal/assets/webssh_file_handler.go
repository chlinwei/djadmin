package assets

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

const webSSHMaxUploadBytes int64 = 500 * 1024 * 1024
const webSSHMultipartOverheadBytes int64 = 1024 * 1024

func (handler *Handler) webSSHFileAgent(context *gin.Context) (string, bool) {
	id, ok := resourceID(context)
	if !ok {
		return "", false
	}
	host, err := handler.service.GetHost(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, err)
		return "", false
	}
	if host.AgentID == nil || strings.TrimSpace(*host.AgentID) == "" {
		context.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "主机未绑定 Agent", "data": nil})
		return "", false
	}
	return strings.TrimSpace(*host.AgentID), true
}

func (handler *Handler) ListWebSSHFiles(context *gin.Context) {
	agentID, ok := handler.webSSHFileAgent(context)
	if !ok {
		return
	}
	result, err := handler.gateway.ListFiles(context.Request.Context(), agentID, strings.TrimSpace(context.Query("path")))
	if err != nil {
		responseFileError(context, err)
		return
	}
	if result == nil || result.Error != "" {
		responseFileError(context, fileOperationError(result.GetError()))
		return
	}
	entries := make([]gin.H, 0, len(result.Entries))
	for _, entry := range result.Entries {
		var size any = entry.Size
		if entry.IsDir {
			size = nil
		}
		entries = append(entries, gin.H{
			"name": entry.Name, "path": path.Join(result.CurrentPath, entry.Name),
			"size": size, "is_dir": entry.IsDir, "mtime": entry.Mtime,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		leftDir, _ := entries[left]["is_dir"].(bool)
		rightDir, _ := entries[right]["is_dir"].(bool)
		if leftDir != rightDir {
			return leftDir
		}
		return strings.ToLower(entries[left]["name"].(string)) < strings.ToLower(entries[right]["name"].(string))
	})
	var parentPath any = path.Dir(result.CurrentPath)
	if result.CurrentPath == "/" {
		parentPath = nil
	}
	respond(context, gin.H{"current_path": result.CurrentPath, "parent_path": parentPath, "entries": entries}, nil)
}

func (handler *Handler) RenameWebSSHFile(context *gin.Context) {
	agentID, ok := handler.webSSHFileAgent(context)
	if !ok {
		return
	}
	var input struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Path) == "" || strings.TrimSpace(input.NewName) == "" {
		responseFileError(context, fileOperationError("path 和 new_name 不能为空"))
		return
	}
	result, err := handler.gateway.RenameFile(context.Request.Context(), agentID, input.Path, input.NewName)
	if err != nil {
		responseFileError(context, err)
		return
	}
	if result == nil || result.Error != "" {
		responseFileError(context, fileOperationError(result.GetError()))
		return
	}
	respond(context, gin.H{"path": result.Path, "name": result.Name}, nil)
}

func (handler *Handler) DeleteWebSSHFile(context *gin.Context) {
	agentID, ok := handler.webSSHFileAgent(context)
	if !ok {
		return
	}
	var input struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Path) == "" {
		responseFileError(context, fileOperationError("path 不能为空"))
		return
	}
	result, err := handler.gateway.DeleteFile(context.Request.Context(), agentID, input.Path, input.Recursive)
	if err != nil {
		responseFileError(context, err)
		return
	}
	if result == nil || result.Error != "" {
		responseFileError(context, fileOperationError(result.GetError()))
		return
	}
	respond(context, gin.H{"path": result.Path}, nil)
}

func (handler *Handler) CreateWebSSHDirectory(context *gin.Context) {
	agentID, ok := handler.webSSHFileAgent(context)
	if !ok {
		return
	}
	var input struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Name) == "" {
		responseFileError(context, fileOperationError("name 不能为空"))
		return
	}
	result, err := handler.gateway.MakeDirectory(context.Request.Context(), agentID, input.Path, input.Name)
	if err != nil {
		responseFileError(context, err)
		return
	}
	if result == nil || result.Error != "" {
		responseFileError(context, fileOperationError(result.GetError()))
		return
	}
	respond(context, gin.H{"path": result.Path, "name": result.Name}, nil)
}

func (handler *Handler) UploadWebSSHFile(context *gin.Context) {
	agentID, ok := handler.webSSHFileAgent(context)
	if !ok {
		return
	}
	if context.Request.ContentLength > webSSHMaxUploadBytes+webSSHMultipartOverheadBytes {
		responseFileError(context, fileOperationError("上传文件不能超过 500 MiB"))
		return
	}
	context.Request.Body = http.MaxBytesReader(
		context.Writer,
		context.Request.Body,
		webSSHMaxUploadBytes+webSSHMultipartOverheadBytes,
	)
	upload, header, err := context.Request.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			responseFileError(context, fileOperationError("上传文件不能超过 500 MiB"))
			return
		}
		responseFileError(context, fileOperationError("缺少上传文件"))
		return
	}
	defer upload.Close()
	if header.Size > webSSHMaxUploadBytes {
		responseFileError(context, fileOperationError("上传文件不能超过 500 MiB"))
		return
	}
	targetPath := strings.TrimSpace(context.PostForm("path"))
	if targetPath == "" {
		targetPath = "."
	}
	fileName := strings.TrimSpace(context.PostForm("filename"))
	if fileName == "" || strings.Contains(fileName, "/") {
		responseFileError(context, fileOperationError("filename 无效"))
		return
	}
	result, err := handler.gateway.WriteFile(context.Request.Context(), agentID, targetPath, fileName, upload)
	if err != nil {
		responseFileError(context, err)
		return
	}
	respond(context, gin.H{"done": true, "path": result.Path, "name": fileName, "size": header.Size}, nil)
}

func (handler *Handler) DownloadWebSSHFile(context *gin.Context) {
	agentID, ok := handler.webSSHFileAgent(context)
	if !ok {
		return
	}
	remotePath := strings.TrimSpace(context.Query("path"))
	if remotePath == "" {
		responseFileError(context, fileOperationError("path 不能为空"))
		return
	}
	stat, err := handler.gateway.StatFile(context.Request.Context(), agentID, remotePath)
	if err != nil {
		responseFileError(context, err)
		return
	}
	if stat == nil || stat.Error != "" {
		responseFileError(context, fileOperationError(stat.GetError()))
		return
	}
	if stat.IsDir {
		responseFileError(context, fileOperationError("目录下载功能已关闭，请改为逐个下载文件"))
		return
	}
	context.Header("Content-Type", "application/octet-stream")
	context.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", path.Base(stat.NormalizedPath)))
	context.Header("Content-Length", fmt.Sprintf("%d", stat.Size))
	context.Header("Accept-Ranges", "bytes")
	context.Status(http.StatusOK)
	for offset := int64(0); offset < stat.Size; {
		length := int64(1024 * 1024)
		if remaining := stat.Size - offset; remaining < length {
			length = remaining
		}
		chunk, readErr := handler.gateway.ReadFileChunk(context.Request.Context(), agentID, stat.NormalizedPath, offset, length)
		if readErr != nil || chunk == nil || chunk.Error != "" || len(chunk.Data) == 0 {
			return
		}
		if _, writeErr := context.Writer.Write(chunk.Data); writeErr != nil {
			return
		}
		offset += int64(len(chunk.Data))
	}
}

type fileOperationError string

func (err fileOperationError) Error() string { return string(err) }
func responseFileError(context *gin.Context, err error) {
	context.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error(), "data": nil})
}
