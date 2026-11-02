package monitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"autoadmin/internal/shared/apperror"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPolicyMatcherSummary(t *testing.T) {
	tests := []struct {
		name     string
		matchers []policyMatcher
		prepare  func(sqlmock.Sqlmock)
		want     string
		wantErr  bool
	}{
		{name: "unrestricted", want: "不限"},
		{
			name: "label-operators",
			matchers: []policyMatcher{
				{Type: "label", Label: "severity", Operator: "=", Value: "critical"},
				{Type: "label", Label: "instance", Operator: "!=", Value: ""},
				{Type: "label", Label: "job", Operator: "=~", Value: "web.*"},
				{Type: "label", Label: "alertname", Operator: "!~", Value: "Test|Demo"},
			},
			want: "标签「severity」等于「critical」 且 标签「instance」不等于「」 且 标签「job」匹配正则「web.*」 且 标签「alertname」不匹配正则「Test|Demo」",
		},
		{
			name:     "project",
			matchers: []policyMatcher{{Type: "tree", NodeType: "project", ID: 1}},
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM assets_project").WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("kul项目"))
			},
			want: "项目「kul项目」",
		},
		{
			name:     "business",
			matchers: []policyMatcher{{Type: "tree", NodeType: "business", ID: 2}},
			prepare: func(mock sqlmock.Sqlmock) {
				now := time.Now().UTC()
				mock.ExpectQuery("FROM assets_business_system s").WithArgs(int64(2)).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "create_time", "update_time", "remark", "name", "code", "owner",
						"enabled", "project_id", "project_name", "project_code",
					}).AddRow(int64(2), now, now, nil, "支付业务", "payment", "", true, int64(1), "kul项目", "kul"))
			},
			want: "业务系统「支付业务」",
		},
		{
			name:     "environment",
			matchers: []policyMatcher{{Type: "tree", NodeType: "environment", ID: 3}},
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM assets_business_environment").WithArgs(int64(3)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("生产环境"))
			},
			want: "环境「生产环境」",
		},
		{
			name:     "service",
			matchers: []policyMatcher{{Type: "tree", NodeType: "service", ID: 4}},
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM assets_application_service").WithArgs(int64(4)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("支付服务"))
			},
			want: "服务「支付服务」",
		},
		{
			name:     "deleted-project",
			matchers: []policyMatcher{{Type: "tree", NodeType: "project", ID: 1}},
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM assets_project").WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}))
			},
			want: "项目不存在（原编号：1）",
		},
		{
			name:     "missing-name",
			matchers: []policyMatcher{{Type: "tree", NodeType: "project", ID: 1}},
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM assets_project").WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow(" "))
			},
			want: "项目名称为空（编号：1）",
		},
		{
			name:     "database-failure",
			matchers: []policyMatcher{{Type: "tree", NodeType: "project", ID: 1}},
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT name FROM assets_project").WithArgs(int64(1)).
					WillReturnError(errors.New("asset database unavailable"))
			},
			wantErr: true,
		},
		{name: "unknown-type", matchers: []policyMatcher{{Type: "unknown"}}, wantErr: true},
		{name: "unknown-node", matchers: []policyMatcher{{Type: "tree", NodeType: "unknown", ID: 1}}, wantErr: true},
		{name: "unknown-operator", matchers: []policyMatcher{{Type: "label", Operator: "unknown"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if test.prepare != nil {
				test.prepare(mock)
			}
			got, err := (&Handler{db: database}).policyMatcherSummary(
				context.Background(), []*policyNode{{Matchers: test.matchers}}, map[string]string{},
			)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("summary=%q, err=%v; want=%q, wantErr=%v", got, err, test.want, test.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserAlertChainReadableMatchers(t *testing.T) {
	for _, queryFailed := range []bool{false, true} {
		name := "inherited-conditions-and-cache"
		if queryFailed {
			name = "name-query-failure"
		}
		t.Run(name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			expectUserChainBinding(mock, 5)
			now := time.Now().UTC()
			mock.ExpectQuery("FROM monitor_notification_policy").WillReturnRows(groupPolicyRows(nil).
				AddRow(int64(2), int64(1), "项目策略", int32(0), "", []byte(`[{"type":"tree","node_type":"project","id":1}]`), nil, nil, true, true, now, now).
				AddRow(int64(3), int64(2), "严重告警", int32(0), "", []byte(`[{"type":"label","label":"severity","operator":"=","value":"critical"}]`), nil, nil, true, true, now, now).
				AddRow(int64(4), int64(2), "沿用项目范围", int32(1), "", []byte(`[]`), nil, nil, true, true, now, now).
				AddRow(int64(5), int64(1), "其他组策略", int32(1), "", []byte(`[{"type":"tree","node_type":"project","id":99}]`), nil, "[20]", true, true, now, now))
			projectQuery := mock.ExpectQuery("SELECT name FROM assets_project").WithArgs(int64(1))
			if queryFailed {
				projectQuery.WillReturnError(errors.New("asset database unavailable"))
			} else {
				projectQuery.WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("kul项目"))
				mock.ExpectQuery("SELECT name FROM sys_user_group").WithArgs(int64(20)).
					WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("ops"))
				mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sys_user_group_member").WithArgs(int32(5), int64(20)).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
			}
			recorder := httptest.NewRecorder()
			userChainEngine(&Handler{db: database}).ServeHTTP(recorder,
				httptest.NewRequest(http.MethodGet, "/monitor/alert-notification/user-chain/", nil))
			code, payload := decodeUserChain(t, recorder)
			if queryFailed {
				if code != apperror.CodeInternal {
					t.Fatalf("name query failure was hidden: %s", recorder.Body.String())
				}
			} else {
				if code != 200 || len(payload.Bindings) != 1 || len(payload.Bindings[0].Policies) != 4 {
					t.Fatalf("unexpected chain: %s", recorder.Body.String())
				}
				policies := payload.Bindings[0].Policies
				for index, want := range []string{
					"不限", "项目「kul项目」",
					"项目「kul项目」 且 标签「severity」等于「critical」", "项目「kul项目」",
				} {
					if policies[index].MatcherSummary != want {
						t.Fatalf("policy %d summary=%q; want=%q", policies[index].ID, policies[index].MatcherSummary, want)
					}
				}
				if string(policies[1].Matchers) != `[{"type":"tree","node_type":"project","id":1}]` ||
					string(policies[3].Matchers) != "[]" {
					t.Fatalf("raw per-node matcher contract changed: %+v", policies)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
