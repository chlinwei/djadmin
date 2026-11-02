package monitor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/api/middleware"
	"autoadmin/internal/identity"
	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func mediaStatusEngine(database *sql.DB, permissions []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(context *gin.Context) {
		context.Set(identity.ClaimsContextKey, &identity.Claims{
			UserID: 7, Username: "operator", Perms: permissions,
		})
	})
	handler := &Handler{db: database}
	engine.PATCH("/monitor/media/:id/", middleware.RequirePermission("monitor:view"),
		middleware.RequirePermission("monitor:update"), handler.UpdateAlertMedia)
	engine.PUT("/monitor/media/:id/", middleware.RequirePermission("monitor:view"),
		middleware.RequirePermission("monitor:update"), handler.UpdateAlertMedia)
	return engine
}

func mediaStatusRows(enabled bool) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{
		"id", "create_time", "update_time", "remark", "name", "media_type", "config", "enabled", "recipients",
	}).AddRow(int64(7), now, now, "unchanged", "smtp-main", "email",
		[]byte(`{"smtpServer":"smtp.example.com","smtpPort":587,"email":"from@example.com","password":"encrypted-secret"}`),
		enabled, []byte(`["to@example.com"]`))
}

func decodeMediaStatus(t *testing.T, recorder *httptest.ResponseRecorder) (int, alertMediaResponse) {
	t.Helper()
	var envelope struct {
		Code int                `json:"code"`
		Data alertMediaResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Code, envelope.Data
}

func TestAlertMediaStatusOnlyPatch(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, affected := range []int64{0, 1} {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			engine := mediaStatusEngine(database, []string{"monitor:view", "monitor:update"})
			mock.ExpectExec(regexp.QuoteMeta("UPDATE monitor_alert_media SET enabled=?,update_time=? WHERE id=?")).
				WithArgs(enabled, sqlmock.AnyArg(), int64(7)).
				WillReturnResult(sqlmock.NewResult(0, affected))
			mock.ExpectQuery("SELECT id, create_time, update_time, remark, name, media_type, config, enabled, recipients").
				WithArgs(int64(7)).WillReturnRows(mediaStatusRows(enabled))
			body := `{"enabled":false}`
			if enabled {
				body = `{"enabled":true}`
			}
			request := httptest.NewRequest(http.MethodPatch, "/monitor/media/7/", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			code, media := decodeMediaStatus(t, recorder)
			if recorder.Code != http.StatusOK || code != 200 || media.Enabled != enabled {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if media.Name != "smtp-main" || media.Remark != "unchanged" || media.Config["password"] != "********" ||
				media.Config["smtpServer"] != "smtp.example.com" {
				t.Fatalf("status update changed configuration or exposed secrets: %+v", media)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			database.Close()
		}
	}
}

func TestAlertMediaStatusPermissionsAndInput(t *testing.T) {
	for _, test := range []struct {
		name        string
		permissions []string
		path        string
		method      string
		body        string
		wantCode    int
	}{
		{name: "view-only", permissions: []string{"monitor:view"}, body: `{"enabled":true}`, wantCode: 403},
		{name: "missing-view", permissions: []string{"monitor:update"}, body: `{"enabled":true}`, wantCode: 403},
		{name: "null", body: `{"enabled":null}`},
		{name: "string", body: `{"enabled":"false"}`},
		{name: "number", body: `{"enabled":1}`},
		{name: "malformed", body: `{"enabled":`},
		{name: "unknown-field", body: `{"enabled":true,"other":1}`},
		{name: "invalid-id", path: "/monitor/media/invalid/", body: `{"enabled":true}`},
		{name: "put-needs-complete-config", method: http.MethodPut, body: `{"enabled":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			permissions := test.permissions
			if permissions == nil {
				permissions = []string{"monitor:view", "monitor:update"}
			}
			path, method := test.path, test.method
			if path == "" {
				path = "/monitor/media/7/"
			}
			if method == "" {
				method = http.MethodPatch
			}
			request := httptest.NewRequest(method, path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			mediaStatusEngine(database, permissions).ServeHTTP(recorder, request)
			code, _ := decodeMediaStatus(t, recorder)
			wantCode := test.wantCode
			if wantCode == 0 {
				wantCode = 400
			}
			if code != wantCode {
				t.Fatalf("expected code %d, got: %s", wantCode, recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAlertMediaStatusWriteFailureAndNotFound(t *testing.T) {
	for _, missing := range []bool{false, true} {
		database, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		update := mock.ExpectExec("UPDATE monitor_alert_media").WithArgs(true, sqlmock.AnyArg(), int64(7))
		if missing {
			update.WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT id, create_time").WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
		} else {
			update.WillReturnError(errors.New("database unavailable"))
		}
		request := httptest.NewRequest(http.MethodPatch, "/monitor/media/7/", strings.NewReader(`{"enabled":true}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		mediaStatusEngine(database, []string{"monitor:view", "monitor:update"}).ServeHTTP(recorder, request)
		code, _ := decodeMediaStatus(t, recorder)
		wantCode := apperror.CodeInternal
		if missing {
			wantCode = 404
		}
		if code != wantCode {
			t.Fatalf("unexpected failure response: %s", recorder.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}

func TestAlertMediaFullPatchPreservesBodyAndPassword(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT config FROM monitor_alert_media").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"config"}).AddRow([]byte(`{"password":"encrypted-secret"}`)))
	mock.ExpectExec("UPDATE monitor_alert_media").
		WithArgs(sqlmock.AnyArg(), "unchanged", "smtp-main", "email",
			[]byte(`{"email":"from@example.com","password":"encrypted-secret","smtpPort":587,"smtpServer":"smtp.example.com"}`),
			false, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, create_time").WithArgs(int64(7)).WillReturnRows(mediaStatusRows(false))
	body := `{"name":"smtp-main","media_type":"email","remark":"unchanged","enabled":false,"config":{"email":"from@example.com","password":"********","smtpPort":587,"smtpServer":"smtp.example.com"}}`
	request := httptest.NewRequest(http.MethodPatch, "/monitor/media/7/", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mediaStatusEngine(database, []string{"monitor:view", "monitor:update"}).ServeHTTP(recorder, request)
	code, _ := decodeMediaStatus(t, recorder)
	if code != 200 {
		t.Fatalf("full edit request was changed: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
