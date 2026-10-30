package automation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestListTasksIncludesActualTemplateFormat(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM automation_task a").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	now := time.Now()
	mock.ExpectQuery("SELECT a.id, a.create_time").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "create_time", "update_time", "remark", "name", "env_vars", "enabled", "inventory_id",
			"default_limit", "execution_timeout_seconds", "playbook_template_id", "run_as_user", "run_as_group",
			"work_directory", "raw_template_name", "inventory_name", "template_content_format",
		}).
			AddRow(7, now, now, nil, "playbook-task", []byte(`{}`), true, nil, "", 600, 70, "", "", "", "deploy", "", "playbook").
			AddRow(8, now, now, nil, "shell-task", []byte(`{}`), true, nil, "", 600, 80, "", "", "", "uptime", "", "shell").
			AddRow(9, now, now, nil, "missing-template-task", []byte(`{}`), true, nil, "", 600, nil, "", "", "", "", "", nil))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/sys/automation/tasks/", nil)
	handler := &Handler{db: pool}
	handler.ListTasks(ctx)

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Results []struct {
				ID                    int64   `json:"id"`
				TemplateName          string  `json:"template_name"`
				TemplateContentFormat *string `json:"template_content_format"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid task list: %s: %v", recorder.Body.String(), err)
	}
	if recorder.Code != http.StatusOK || envelope.Code != 200 || len(envelope.Data.Results) != 3 {
		t.Fatalf("unexpected task list: %s", recorder.Body.String())
	}
	playbook, shell, missing := envelope.Data.Results[0], envelope.Data.Results[1], envelope.Data.Results[2]
	if playbook.ID != 7 || playbook.TemplateName != "[Playbook] deploy" ||
		playbook.TemplateContentFormat == nil || *playbook.TemplateContentFormat != "playbook" ||
		shell.ID != 8 || shell.TemplateName != "[Playbook] uptime" ||
		shell.TemplateContentFormat == nil || *shell.TemplateContentFormat != "shell" ||
		missing.TemplateName != "" || missing.TemplateContentFormat != nil {
		t.Fatalf("actual format or existing name contract lost: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
