package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSelfProfileUpdates(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		phone      string
		timezone   string
		writePhone bool
		writeZone  bool
		failZone   bool
		code       int
	}{
		{"both fields and forged identity", `{"id":99,"user_id":99,"status":0,"phonenumber":" 256 ","timezone":" Asia/Hong_Kong "}`, "256", "Asia/Hong_Kong", true, true, false, 200},
		{"timezone only", `{"timezone":"Asia/Kolkata"}`, "old-phone", "Asia/Kolkata", false, true, false, 200},
		{"phone only", `{"phonenumber":"256"}`, "256", "Asia/Shanghai", true, false, false, 200},
		{"unknown timezone", `{"timezone":"Unknown/Zone"}`, "", "", false, false, false, 400},
		{"server local timezone", `{"timezone":"Local"}`, "", "", false, false, false, 400},
		{"empty timezone", `{"timezone":""}`, "", "", false, false, false, 400},
		{"invalid JSON", `{`, "", "", false, false, false, 400},
		{"no supported fields", `{"id":99,"status":0}`, "", "", false, false, false, 400},
		{"atomic failure", `{"phonenumber":"256","timezone":"UTC"}`, "256", "UTC", true, true, true, 600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			tokens := identity.NewTokenManager("test-secret", time.Hour)
			engine, err := New(database, tokens, []string{"*"}, nil, "", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			token, err := tokens.Issue(7, "limited-user", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.writePhone || tt.writeZone {
				mock.ExpectBegin()
				if tt.writePhone {
					mock.ExpectExec("UPDATE sys_user SET phonenumber").
						WithArgs(tt.phone, sqlmock.AnyArg(), 7).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				if tt.writeZone {
					query := mock.ExpectExec("UPDATE sys_user SET timezone").WithArgs(tt.timezone, sqlmock.AnyArg(), 7)
					if tt.failZone {
						query.WillReturnError(errors.New("database unavailable"))
					} else {
						query.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				if tt.failZone {
					mock.ExpectRollback()
				} else {
					now := time.Now().UTC()
					mock.ExpectQuery("SELECT .* FROM sys_user WHERE id").
						WithArgs(7).
						WillReturnRows(sqlmock.NewRows([]string{
							"id", "username", "password", "avatar", "phonenumber", "login_date",
							"status", "create_time", "update_time", "remark", "timezone",
						}).AddRow(7, "limited-user", "secret-hash", "avatar.png", tt.phone, nil, 1, now, now, "kept", tt.timezone))
					mock.ExpectCommit()
				}
			}
			mock.ExpectExec("INSERT INTO audit_operation_log").WillReturnResult(sqlmock.NewResult(1, 1))
			request := httptest.NewRequest(http.MethodPost, "/sys/usercenter/updateUserInfo/", strings.NewReader(tt.body))
			request.Header.Set("Authorization", token)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			var result struct {
				Code int `json:"code"`
				Data struct {
					User struct {
						ID          int32  `json:"id"`
						Phonenumber string `json:"phonenumber"`
						Timezone    string `json:"timezone"`
						Status      int16  `json:"status"`
					} `json:"user"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			wantStatus := http.StatusOK
			if tt.failZone {
				wantStatus = http.StatusInternalServerError
			}
			if recorder.Code != wantStatus || result.Code != tt.code {
				t.Fatalf("unexpected response: HTTP %d %s", recorder.Code, recorder.Body.String())
			}
			if tt.code == 200 {
				if result.Data.User.ID != 7 || result.Data.User.Phonenumber != tt.phone || result.Data.User.Timezone != tt.timezone || result.Data.User.Status != 1 {
					t.Fatalf("unexpected saved profile: %s", recorder.Body.String())
				}
				if strings.Contains(recorder.Body.String(), "password") || strings.Contains(recorder.Body.String(), "secret-hash") {
					t.Fatal("password returned in profile")
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSelfProfileRequiresLogin(t *testing.T) {
	engine, err := New(nil, identity.NewTokenManager("test-secret", time.Hour), []string{"*"}, nil, "", "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/sys/usercenter/updateUserInfo/", strings.NewReader(`{"timezone":"UTC"}`)))
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.Code != 301 {
		t.Fatalf("unauthenticated response: %s, error: %v", recorder.Body.String(), err)
	}
}
