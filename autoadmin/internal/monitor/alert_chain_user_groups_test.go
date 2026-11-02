package monitor

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
)

func groupPolicyRows(groups any) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{
		"id", "parent_id", "name", "position", "remark", "matchers", "media_ids", "user_group_ids",
		"notify_on_firing", "notify_on_resolved", "create_time", "update_time",
	}).AddRow(int64(1), int64(0), "默认策略", int32(0), "", []byte(`[]`), "[2]", groups, true, true, now, now)
}

func expectUserChainBinding(mock sqlmock.Sqlmock, userID int32) {
	mock.ExpectQuery("SELECT username FROM sys_user").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("target"))
	mock.ExpectQuery("FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "media_id", "media_name", "recipients", "enabled", "media_type", "media_enabled",
		}).AddRow(int64(7), int64(2), "smtp-main", []byte(`["to@example.com"]`), true, "email", true))
}

func decodeUserChain(t *testing.T, recorder *httptest.ResponseRecorder) (int, alertChainUserChain) {
	t.Helper()
	var envelope struct {
		Code int                 `json:"code"`
		Data alertChainUserChain `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Code, envelope.Data
}

func TestUserAlertChainFiltersUnrelatedGroups(t *testing.T) {
	for _, test := range []struct {
		name   string
		userID int32
		path   string
	}{
		{name: "current-user", userID: 5, path: "/monitor/alert-notification/user-chain/"},
		{name: "target-user", userID: 9, path: "/monitor/alert-notification/user-chain/?user_id=9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			expectUserChainBinding(mock, test.userID)
			now := time.Now().UTC()
			rows := groupPolicyRows(nil).
				AddRow(int64(2), int64(1), "kul通知", int32(0), "", []byte(`[]`), nil, "[10]", true, true, now, now).
				AddRow(int64(3), int64(1), "nkg通知", int32(0), "", []byte(`[]`), nil, "[20]", false, true, now, now).
				AddRow(int64(4), int64(3), "继承ops组", int32(0), "", []byte(`[]`), nil, nil, false, true, now, now).
				AddRow(int64(5), int64(3), "覆盖为admin组", int32(0), "", []byte(`[]`), nil, "[10]", true, true, now, now)
			mock.ExpectQuery("FROM monitor_notification_policy").WillReturnRows(rows)
			for _, groupID := range []int64{10, 20, 20, 10} {
				name, count := "admin", int64(1)
				if groupID == 20 {
					name, count = "ops", 0
				}
				mock.ExpectQuery("SELECT name FROM sys_user_group").WithArgs(groupID).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow(name))
				mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sys_user_group_member").WithArgs(test.userID, groupID).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			}
			recorder := httptest.NewRecorder()
			userChainEngine(&Handler{db: database}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			code, payload := decodeUserChain(t, recorder)
			if code != 200 || !payload.CanReceive || payload.User["id"] != float64(test.userID) {
				t.Fatalf("unexpected chain: %s", recorder.Body.String())
			}
			if len(payload.Bindings) != 1 || len(payload.Bindings[0].Policies) != 3 {
				t.Fatalf("unrelated group policies were not filtered: %+v", payload.Bindings)
			}
			for index, wantID := range []int64{1, 2, 5} {
				policy := payload.Bindings[0].Policies[index]
				if policy.ID != wantID || !policy.UserInGroup {
					t.Fatalf("unexpected policy: %+v", policy)
				}
			}
			if len(payload.SummaryIssues) != 0 || len(payload.Bindings[0].Issues) != 0 {
				t.Fatalf("unrelated groups generated false issues: %+v", payload)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserAlertChainNoApplicablePolicyAndMembershipFailure(t *testing.T) {
	for _, test := range []struct {
		name        string
		groups      string
		queryFailed bool
	}{
		{name: "other-group-only", groups: "[20]"},
		{name: "explicit-empty-group", groups: "[]"},
		{name: "membership-query-fails", groups: "[20]", queryFailed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			expectUserChainBinding(mock, 5)
			mock.ExpectQuery("FROM monitor_notification_policy").WillReturnRows(groupPolicyRows(test.groups))
			if test.groups != "[]" {
				mock.ExpectQuery("SELECT name FROM sys_user_group").WithArgs(int64(20)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("ops"))
				membership := mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sys_user_group_member").WithArgs(int32(5), int64(20))
				if test.queryFailed {
					membership.WillReturnError(errors.New("membership database unavailable"))
				} else {
					membership.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
				}
			}
			recorder := httptest.NewRecorder()
			userChainEngine(&Handler{db: database}).ServeHTTP(recorder,
				httptest.NewRequest(http.MethodGet, "/monitor/alert-notification/user-chain/", nil))
			code, payload := decodeUserChain(t, recorder)
			if test.queryFailed {
				if code != apperror.CodeInternal {
					t.Fatalf("membership failure was presented as a filtered chain: %s", recorder.Body.String())
				}
			} else {
				if code != 200 || payload.CanReceive || len(payload.Bindings) != 1 {
					t.Fatalf("unexpected chain: %s", recorder.Body.String())
				}
				if len(payload.Bindings[0].Policies) != 0 || len(payload.Bindings[0].Issues) != 0 {
					t.Fatalf("unrelated group policy or issue was returned: %+v", payload.Bindings)
				}
				if !containsString(payload.SummaryIssues, "媒介 smtp-main 暂无对当前用户生效的通知策略") ||
					containsString(payload.SummaryIssues, "媒介 smtp-main 未被任何通知策略出口命中") {
					t.Fatalf("summary conflated user filtering with globally missing outlets: %v", payload.SummaryIssues)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
