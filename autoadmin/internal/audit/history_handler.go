package audit

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListLoginLogs(c *gin.Context) {
	page, err := auditPage(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	filter := LoginFilter{Keyword: strings.TrimSpace(c.Query("keyword")), Status: strings.ToLower(c.Query("status")), From: parseAuditTime(c.Query("login_time_from")), To: parseAuditTime(c.Query("login_time_to"))}
	rows, count, err := h.repository.ListLoginLogs(c.Request.Context(), filter, page)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrQueryInternal, err))
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) ListWebSSHSessions(c *gin.Context) {
	page, err := auditPage(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	filter := webSSHFilter(c)
	rows, count, err := h.repository.ListWebSSHSessions(c.Request.Context(), filter, page)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrQueryInternal, err))
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) WebSSHContent(c *gin.Context) {
	id, ok := auditID(c)
	if !ok {
		return
	}
	content, err := h.repository.GetWebSSHContent(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, apperror.ErrResourceNotFound)
		return
	}
	if err != nil {
		response.Error(c, apperror.WithCause(ErrQueryInternal, err))
		return
	}
	response.Success(c, content)
}
func (h *Handler) DownloadWebSSH(c *gin.Context) {
	id, ok := auditID(c)
	if !ok {
		return
	}
	content, err := h.repository.GetWebSSHContent(c.Request.Context(), id)
	if err != nil {
		response.Error(c, apperror.ErrResourceNotFound)
		return
	}
	writeTextDownload(c, sessionFilename(content), sessionText(content))
}
func (h *Handler) DownloadWebSSHMany(c *gin.Context) {
	ids := parseIDs(c.QueryArray("ids"))
	contents := []WebSSHContent{}
	for _, id := range ids {
		content, err := h.repository.GetWebSSHContent(c.Request.Context(), id)
		if err == nil {
			contents = append(contents, content)
		}
	}
	if len(contents) == 0 {
		writeTextDownload(c, "webssh-sessions.log", "Web SSH Session Logs\nTotal: 0\n")
		return
	}
	if len(contents) == 1 {
		writeTextDownload(c, sessionFilename(contents[0]), sessionText(contents[0]))
		return
	}
	buffer := bytes.NewBuffer(nil)
	writer := zip.NewWriter(buffer)
	for _, content := range contents {
		file, _ := writer.Create(sessionFilename(content))
		_, _ = file.Write([]byte(sessionText(content)))
	}
	_ = writer.Close()
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''webssh-sessions.zip")
	c.Data(http.StatusOK, "application/zip", buffer.Bytes())
}
func webSSHFilter(c *gin.Context) WebSSHFilter {
	return WebSSHFilter{Status: strings.ToLower(c.Query("status")), Username: strings.TrimSpace(c.Query("username")), Keyword: strings.TrimSpace(c.Query("keyword")), OutputKeyword: strings.TrimSpace(c.Query("output_keyword")), From: parseAuditTime(c.Query("start_time_from")), To: parseAuditTime(c.Query("start_time_to"))}
}
func auditID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.Error(c, apperror.ErrIDInvalid)
		return 0, false
	}
	return id, true
}
func parseIDs(values []string) []int64 {
	result := []int64{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && id > 0 {
				result = append(result, id)
			}
		}
	}
	return result
}
func sessionFilename(content WebSSHContent) string {
	name := "host"
	if content.HostName != nil {
		name = *content.HostName
	}
	name = strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(name)
	return fmt.Sprintf("webssh-%d-%s.log", content.ID, name)
}
func sessionText(content WebSSHContent) string {
	return fmt.Sprintf("Web SSH Session\nID: %d\nHost: %s\nUser: %s\nStart: %s\nStatus: %s\n\n%s\n", content.ID, stringValue(content.HostName), content.Username, content.StartTime, content.Status, content.OutputContent)
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func writeTextDownload(c *gin.Context, filename, content string) {
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(content))
}
