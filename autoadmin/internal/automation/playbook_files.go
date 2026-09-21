package automation

import (
	"database/sql"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// Playbook 模板文件上传/下载，对应 Django PlaybookTemplateManage.upload_file/download_file。
// 内容仍存 automation_playbook_template.content 列，不落文件系统。

var playbookFilenamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// UploadFile 处理 multipart 上传（字段名 file），校验后缀/UTF-8/YAML 后覆盖模板内容。
func (handler *Handler) UploadFile(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil {
		response.BusinessError(context, 400, "invalid playbook id", nil)
		return
	}
	file, err := context.FormFile("file")
	if err != nil {
		response.BusinessError(context, 400, "请选择上传文件", nil)
		return
	}
	filename := strings.ToLower(file.Filename)
	if !strings.HasSuffix(filename, ".yml") && !strings.HasSuffix(filename, ".yaml") {
		response.BusinessError(context, 400, "仅支持上传 .yml / .yaml 文件", nil)
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 10<<20+1)
	opened, err := file.Open()
	if err != nil {
		response.BusinessError(context, 400, "读取上传文件失败", nil)
		return
	}
	defer opened.Close()
	raw, err := io.ReadAll(io.LimitReader(opened, 10<<20+1))
	if err != nil {
		response.BusinessError(context, 400, "读取上传文件失败", nil)
		return
	}
	if len(raw) > 10<<20 {
		response.BusinessError(context, 400, "文件过大", nil)
		return
	}
	// Django：UTF-8 解码失败直接 400，避免把乱码写进模板。
	if !utf8.Valid(raw) {
		response.BusinessError(context, 400, "Template file must be UTF-8 encoded", nil)
		return
	}
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	// .yml/.yaml 上传只适用于 Playbook 形态：shell 模板的 content 是裸脚本，用 YAML 覆盖会让
	// 形态与内容对不上（校验按 format 分流，format 不随上传改变）。前端已对 shell 隐藏上传入口，
	// 这里再挡一次，避免绕过 UI 造成不一致。
	if row, rowErr := db.New(handler.db).GetAutomationPlaybook(context, id); rowErr == nil && row.ContentFormat == playbookFormatShell {
		response.BusinessError(context, 400, "Shell 类模板不支持 YAML 文件覆盖，请直接在编辑框修改脚本内容", nil)
		return
	}
	if err := validatePlaybook(content); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	affected, err := db.New(handler.db).UpdateAutomationPlaybookContent(context, db.UpdateAutomationPlaybookContentParams{
		Content: content, UpdateTime: time.Now().UTC(), ID: id,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	if affected == 0 {
		response.BusinessError(context, 404, "playbook not found", nil)
		return
	}
	handler.GetByID(context, id)
}

// DownloadFile 以 text/yaml 附件形式下发模板内容，文件名由模板 name 清洗而来。
func (handler *Handler) DownloadFile(context *gin.Context) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil {
		response.BusinessError(context, 400, "invalid playbook id", nil)
		return
	}
	playbook, err := db.New(handler.db).GetAutomationPlaybook(context, id)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "playbook not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	filename := playbookFilenamePattern.ReplaceAllString(playbook.Name, "-") + ".yml"
	context.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	context.Data(http.StatusOK, "text/yaml; charset=utf-8", []byte(playbook.Content))
}
