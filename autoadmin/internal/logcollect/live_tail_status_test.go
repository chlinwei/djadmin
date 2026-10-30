package logcollect

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestLiveTailStatusOwnershipCapacityAndRelease(t *testing.T) {
	state := &liveTailState{slots: map[string]liveSlot{}}
	for i := 1; i <= 5; i++ {
		if err := state.reserve(fmt.Sprint(i), liveSlot{
			user: int32(i), host: "1", hostIP: "10.0.0.1", path: "/app.log",
		}); err != nil {
			t.Fatal(err)
		}
	}
	status := state.status(1)
	if status.MaxFiles != 5 || status.MaxViewers != 5 || status.UserFiles != 1 || len(status.Files) != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Files[0] != (liveFileStatus{HostIP: "10.0.0.1", Path: "/app.log", Viewers: 5, CurrentUser: true}) {
		t.Fatalf("own reservation must remain visible at capacity: %+v", status)
	}
	if state.status(6).Files[0].CurrentUser || state.status(6).UserFiles != 0 {
		t.Fatal("another user inherited ownership")
	}
	if err := state.reserve("6", liveSlot{user: 6, host: "1", path: "/app.log"}); err == nil || !strings.Contains(err.Error(), "该文件跟踪已满") {
		t.Fatalf("expected file-specific capacity error, got %v", err)
	}
	state.release("1")
	if status = state.status(1); status.UserFiles != 0 || status.Files[0].Viewers != 4 || status.Files[0].CurrentUser {
		t.Fatalf("release did not update occupancy: %+v", status)
	}
	for i := 2; i <= 5; i++ {
		state.release(fmt.Sprint(i))
	}
	if status = state.status(1); status.Files == nil || len(status.Files) != 0 {
		t.Fatalf("empty snapshot must serialize as an array: %+v", status)
	}
}

func TestLiveTailStatusPhysicalAliasesRotationAndHostIsolation(t *testing.T) {
	state := &liveTailState{slots: map[string]liveSlot{}}
	slots := []liveSlot{
		{user: 1, host: "1", hostIP: "10.0.0.1", path: "/logical.log", identity: "old"},
		{user: 2, host: "1", hostIP: "10.0.0.1", path: "/logical.log", identity: "new"},
		{user: 3, host: "1", hostIP: "10.0.0.1", path: "/alias.log", identity: "new"},
		{user: 4, host: "2", hostIP: "10.0.0.2", path: "/logical.log", identity: "new"},
	}
	for i, slot := range slots {
		if err := state.reserve(fmt.Sprint(i), slot); err != nil {
			t.Fatal(err)
		}
	}
	status := state.status(1)
	if len(status.Files) != 3 {
		t.Fatalf("unexpected paths: %+v", status)
	}
	for _, file := range status.Files {
		if file.HostIP == "10.0.0.1" {
			if file.Viewers != 3 || !file.CurrentUser {
				t.Fatalf("known aliases/rotation must share occupancy: %+v", file)
			}
		} else if file.Viewers != 1 || file.CurrentUser {
			t.Fatalf("physical identities must be scoped by host: %+v", file)
		}
	}
}

func TestLiveTailStatusAndAdmissionUseSameRotationGroup(t *testing.T) {
	state := &liveTailState{slots: map[string]liveSlot{}}
	slots := []liveSlot{
		{user: 1, host: "1", path: "/logical.log", identity: "old"},
		{user: 2, host: "1", path: "/logical.log", identity: "new"},
		{user: 3, host: "1", path: "/alias.log", identity: "new"},
		{user: 4, host: "1", path: "/alias.log", identity: "new"},
		{user: 5, host: "1", path: "/alias.log", identity: "new"},
	}
	for i, slot := range slots {
		if err := state.reserve(fmt.Sprint(i), slot); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.reserve("six", liveSlot{user: 6, host: "1", path: "/old-alias.log", identity: "old"}); err == nil {
		t.Fatal("old inode alias bypassed the full logical file group")
	}
	if err := state.reserve("duplicate", liveSlot{user: 3, host: "1", path: "/old-alias.log", identity: "old"}); err == nil || !strings.Contains(err.Error(), "其他窗口") {
		t.Fatalf("same user must be rejected before the full-file state: %v", err)
	}
	state.release("4")
	if err := state.reserve("six", liveSlot{user: 6, host: "1", path: "/old-alias.log", identity: "old"}); err != nil {
		t.Fatalf("released group slot was not reusable: %v", err)
	}
}

func TestLiveTailStatusHTTPAuthorizationAndPrivacy(t *testing.T) {
	for _, test := range []struct {
		name        string
		username    string
		status      int
		permissions []string
		mode        string
		wantCode    int
	}{
		{name: "unauthenticated", wantCode: 301},
		{name: "admin", username: "admin", status: 1, wantCode: 200},
		{name: "viewer", username: "viewer", status: 1, permissions: []string{"monitor:view"}, wantCode: 200},
		{name: "disabled account", username: "viewer", status: 0, wantCode: 403},
		{name: "revoked grant", username: "viewer", status: 1, permissions: []string{"assets:hosts:view"}, wantCode: 403},
		{name: "feature disabled", username: "admin", status: 1, mode: "disabled", wantCode: 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LOG_LIVE_TAIL_MODE", test.mode)
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			handler := &Handler{db: database}
			handler.ConfigureLiveTail(nil, nil)
			handler.liveTail.slots["private-reader-id"] = liveSlot{
				user: 99, host: "42", hostIP: "10.0.0.1", path: "/app.log", identity: "private-inode",
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/monitor/log-tail/status/", nil)
			if test.username != "" {
				tokens := identity.NewTokenManager("test-secret", time.Hour)
				token, err := tokens.Issue(1, test.username, []string{"monitor:view"})
				if err != nil {
					t.Fatal(err)
				}
				claims, err := tokens.Parse(token)
				if err != nil {
					t.Fatal(err)
				}
				c.Set(identity.ClaimsContextKey, claims)
				mock.ExpectQuery("FROM sys_user WHERE id").WithArgs(int32(1)).WillReturnRows(liveUserRows(test.username, test.status))
				if test.username != "admin" && test.status != 0 {
					rows := sqlmock.NewRows([]string{"perms"})
					for _, permission := range test.permissions {
						rows.AddRow(permission)
					}
					mock.ExpectQuery("SELECT DISTINCT m.perms").WillReturnRows(rows)
				}
			}
			handler.LiveTailStatus(c)
			var body struct {
				Code int            `json:"code"`
				Data liveTailStatus `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != 200 || body.Code != test.wantCode || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected response: HTTP %d %s", recorder.Code, recorder.Body)
			}
			if body.Code == 200 && (len(body.Data.Files) != 1 || body.Data.Files[0].CurrentUser || body.Data.Files[0].Viewers != 1) {
				t.Fatalf("unexpected snapshot: %+v", body.Data)
			}
			for _, private := range []string{"private-reader-id", "private-inode", `"user":99`, `"host":"42"`} {
				if strings.Contains(recorder.Body.String(), private) {
					t.Fatalf("snapshot exposed %s", private)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
