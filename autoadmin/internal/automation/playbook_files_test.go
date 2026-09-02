package automation

import (
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func newPlaybookFileServer(t *testing.T, database *sql.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.POST("/playbooks/:id/upload/", handler.UploadFile)
	engine.GET("/playbooks/:id/download/", handler.DownloadFile)
	return engine
}

func multipartBody(t *testing.T, fieldName, fileName, content string) (*strings.Reader, string) {
	t.Helper()
	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(body.String()), writer.FormDataContentType()
}

// 上传回归：合法 YAML 覆盖 content 并返回模板对象；非 .yml/.yaml 直接 400 不碰库。
func TestPlaybookUploadFile(t *testing.T) {
	updateQuery := regexp.QuoteMeta(`UPDATE automation_playbook_template SET content=?,update_time=? WHERE id=?`)
	selectQuery := regexp.QuoteMeta(`SELECT id,create_time,update_time,remark,name,description,content,category FROM automation_playbook_template WHERE id=?`)
	now := time.Now().UTC()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	engine := newPlaybookFileServer(t, database)

	body, contentType := multipartBody(t, "file", "site.yml", "- hosts: all\n  tasks:\n    - name: ping\n      ping:\n")
	mock.ExpectExec(updateQuery).WithArgs("- hosts: all\n  tasks:\n    - name: ping\n      ping:\n", sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(selectQuery).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "create_time", "update_time", "remark", "name", "description", "content", "category"}).
			AddRow(int64(7), now, now, sql.NullString{}, "site", "", "- hosts: all", "general"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/playbooks/7/upload/", body)
	request.Header.Set("Content-Type", contentType)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"code":200`) {
		t.Fatalf("upload: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "- hosts: all") {
		t.Fatalf("response content missing: %s", recorder.Body.String())
	}

	// 非 YAML 后缀：400 且无数据库交互
	recorder = httptest.NewRecorder()
	body, contentType = multipartBody(t, "file", "site.txt", "- hosts: all")
	request = httptest.NewRequest(http.MethodPost, "/playbooks/7/upload/", body)
	request.Header.Set("Content-Type", contentType)
	engine.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), `.yml / .yaml`) {
		t.Fatalf("invalid ext: body=%s", recorder.Body.String())
	}

	// 非 UTF-8 内容：400（Django 行为）
	recorder = httptest.NewRecorder()
	body, contentType = multipartBody(t, "file", "site.yml", "\xff\xfe\x00invalid")
	request = httptest.NewRequest(http.MethodPost, "/playbooks/7/upload/", body)
	request.Header.Set("Content-Type", contentType)
	engine.ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "UTF-8") {
		t.Fatalf("invalid utf8: body=%s", recorder.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// 下载回归：text/yaml 附件、filename* 按 RFC 5987 编码。
func TestPlaybookDownloadFile(t *testing.T) {
	selectQuery := regexp.QuoteMeta(`SELECT name,content FROM automation_playbook_template WHERE id=?`)
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()
	engine := newPlaybookFileServer(t, database)

	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "content"}).AddRow("deploy main v2", "- hosts: all\n"))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/playbooks/3/download/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("download status=%d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/yaml") {
		t.Fatalf("content-type=%q", got)
	}
	disposition := recorder.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment; filename*=UTF-8''") {
		t.Fatalf("disposition=%q", disposition)
	}
	expected := url.PathEscape("deploy-main-v2.yml")
	if !strings.Contains(disposition, expected) {
		t.Fatalf("filename missing %q in %q", expected, disposition)
	}
	if !strings.Contains(recorder.Body.String(), "- hosts: all") {
		t.Fatalf("content missing: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
