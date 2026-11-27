package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHomeOverviewPermissionAndPartialFailure(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	tokens := identity.NewTokenManager("home-test-secret", time.Hour)
	engine, err := New(database, tokens, []string{"*"}, nil, "", "home-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, token string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/sys/home/overview/", nil)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) (int, map[string]json.RawMessage) {
		t.Helper()
		var payload struct {
			Code int                        `json:"code"`
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Code, payload.Data
	}
	issue := func(perms ...string) string {
		t.Helper()
		token, err := tokens.Issue(7, "operator", perms)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}

	if code, _ := decode(request(http.MethodGet, "")); code != 301 {
		t.Fatalf("anonymous request code = %d, want 301", code)
	}
	noAccess := issue("automation:tasks:view")
	if code, data := decode(request(http.MethodGet, noAccess)); code != 200 || len(data) != 1 || data["generated_at"] == nil {
		t.Fatalf("unprivileged overview = %d %v", code, data)
	}
	if rec := request(http.MethodPost, noAccess); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST overview status = %d", rec.Code)
	}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets_host`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM inspection_execution`).WillReturnError(errors.New("inspection query failed"))
	code, data := decode(request(http.MethodGet, issue("assets:hosts:view", "inspection:view")))
	if code != 200 || len(data) != 3 || string(data["hosts"]) != `{"status":"ok","count":0}` ||
		string(data["inspections"]) != `{"status":"unavailable"}` {
		t.Fatalf("partial overview = %d %v", code, data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
