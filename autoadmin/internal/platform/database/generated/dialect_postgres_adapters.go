//go:build postgres

// 本文件是方言适配层：把下面这些查询在 PostgreSQL 侧补成与 MySQL 侧**一致的签名**，
// 让应用代码不需要分方言。
//
// 为什么只有这些：sqlc 的两个引擎对「命名参数」的处理不同（可空性推导、重复参数是否拆分），
// 实测 263 个共有结构体里 205 个逐字段一致，真正要适配的是下面两组——
//  1. 只传一个 pattern 的 count 查询：PG 产物把结构体展开成了单个入参；
//  2. 少量参数字段类型不同（MySQL interface{} vs PG string/sql.NullString）与字段改名
//     （Column3 ↔ Column1）、以及一个聚合列类型不同的 Row（ListProjectsRow）。
//
// 其余差异（如 sql.NullInt64 → interface{}）可直接赋值，不需要适配。
//
// 详细数据与后续治理见 docs/architecture/SQL_DESIGN.md §4.6.1。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	postgres "autoadmin/internal/platform/database/generated/postgres"
)

// ---- 1) count 查询：PG 侧没有 Params 结构体 ----

type CountProjectsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountProjects(ctx context.Context, arg CountProjectsParams) (int64, error) {
	return q.Queries.CountProjects(ctx, arg.Pattern)
}

type CountApplicationsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountApplications(ctx context.Context, arg CountApplicationsParams) (int64, error) {
	return q.Queries.CountApplications(ctx, arg.Pattern)
}

type CountBusinessSystemsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountBusinessSystems(ctx context.Context, arg CountBusinessSystemsParams) (int64, error) {
	return q.Queries.CountBusinessSystems(ctx, arg.Pattern)
}

type CountBusinessEnvironmentsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountBusinessEnvironments(ctx context.Context, arg CountBusinessEnvironmentsParams) (int64, error) {
	return q.Queries.CountBusinessEnvironments(ctx, arg.Pattern)
}

type CountCredentialsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountCredentials(ctx context.Context, arg CountCredentialsParams) (int64, error) {
	return q.Queries.CountCredentials(ctx, arg.Pattern)
}

type CountHostGroupsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountHostGroups(ctx context.Context, arg CountHostGroupsParams) (int64, error) {
	return q.Queries.CountHostGroups(ctx, arg.Pattern)
}

type CountBaselinesParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountBaselines(ctx context.Context, arg CountBaselinesParams) (int64, error) {
	return q.Queries.CountBaselines(ctx, arg.Pattern)
}

type CountInventoriesParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountInventories(ctx context.Context, arg CountInventoriesParams) (int64, error) {
	return q.Queries.CountInventories(ctx, arg.Pattern)
}

type CountAutomationHostOptionsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountAutomationHostOptions(ctx context.Context, arg CountAutomationHostOptionsParams) (int64, error) {
	return q.Queries.CountAutomationHostOptions(ctx, arg.Pattern)
}

type CountInspectionGroupsParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountInspectionGroups(ctx context.Context, arg CountInspectionGroupsParams) (int64, error) {
	return q.Queries.CountInspectionGroups(ctx, arg.Pattern)
}

type CountInspectionTasksParams struct {
	Pattern sql.NullString `json:"pattern"`
}

func (q *Queries) CountInspectionTasks(ctx context.Context, arg CountInspectionTasksParams) (int64, error) {
	return q.Queries.CountInspectionTasks(ctx, arg.Pattern)
}

// ---- 2) 告警历史：标签键/值在两侧的字段类型不同 ----
//
// MySQL 侧 LabelKey/LabelValue 是 interface{}（调用方传 string 或 nil 表示不过滤），
// PG 侧 sqlc 推导成 string 与 sql.NullString，这里做一次显式转换。

type CountAlertHistoriesParams struct {
	ID         sql.NullInt64  `json:"id"`
	State      sql.NullString `json:"state"`
	Severity   sql.NullString `json:"severity"`
	Keyword    sql.NullString `json:"keyword"`
	StartTime  sql.NullTime   `json:"start_time"`
	EndTime    sql.NullTime   `json:"end_time"`
	LabelKey   interface{}    `json:"label_key"`
	LabelValue interface{}    `json:"label_value"`
}

func (q *Queries) CountAlertHistories(ctx context.Context, arg CountAlertHistoriesParams) (int64, error) {
	return q.Queries.CountAlertHistories(ctx, postgres.CountAlertHistoriesParams{
		ID: arg.ID, State: arg.State, Severity: arg.Severity, Keyword: arg.Keyword,
		StartTime: arg.StartTime, EndTime: arg.EndTime,
		LabelKey: labelKeyArg(arg.LabelKey), LabelValue: nullStringArg(arg.LabelValue),
	})
}

type ListAlertHistoriesParams struct {
	ID         sql.NullInt64  `json:"id"`
	State      sql.NullString `json:"state"`
	Severity   sql.NullString `json:"severity"`
	Keyword    sql.NullString `json:"keyword"`
	StartTime  sql.NullTime   `json:"start_time"`
	EndTime    sql.NullTime   `json:"end_time"`
	LabelKey   interface{}    `json:"label_key"`
	LabelValue interface{}    `json:"label_value"`
	Limit      int32          `json:"limit"`
	Offset     int32          `json:"offset"`
}

func (q *Queries) ListAlertHistories(ctx context.Context, arg ListAlertHistoriesParams) ([]ListAlertHistoriesRow, error) {
	return q.Queries.ListAlertHistories(ctx, postgres.ListAlertHistoriesParams{
		ID: arg.ID, State: arg.State, Severity: arg.Severity, Keyword: arg.Keyword,
		StartTime: arg.StartTime, EndTime: arg.EndTime,
		LabelKey: labelKeyArg(arg.LabelKey), LabelValue: nullStringArg(arg.LabelValue),
		Limit: arg.Limit, Offset: arg.Offset,
	})
}

// ---- 3) 部署模板：PG 侧把「判空用」的裸参数命名为 Column1，MySQL 侧叫 Column3 ----

type CountDeploymentTemplatesParams struct {
	ApplicationID sql.NullInt64 `json:"application_id"`
	Column3       interface{}   `json:"column_3"`
	Name          string        `json:"name"`
	Name_2        string        `json:"name_2"`
}

func (q *Queries) CountDeploymentTemplates(ctx context.Context, arg CountDeploymentTemplatesParams) (int64, error) {
	return q.Queries.CountDeploymentTemplates(ctx, postgres.CountDeploymentTemplatesParams{
		ApplicationID: arg.ApplicationID, Column1: arg.Column3, Name: arg.Name, Name_2: arg.Name_2,
	})
}

type ListDeploymentTemplatesParams struct {
	ApplicationID sql.NullInt64 `json:"application_id"`
	Column3       interface{}   `json:"column_3"`
	Name          string        `json:"name"`
	Name_2        string        `json:"name_2"`
	Limit         int32         `json:"limit"`
	Offset        int32         `json:"offset"`
}

func (q *Queries) ListDeploymentTemplates(ctx context.Context, arg ListDeploymentTemplatesParams) ([]ListDeploymentTemplatesRow, error) {
	return q.Queries.ListDeploymentTemplates(ctx, postgres.ListDeploymentTemplatesParams{
		ApplicationID: arg.ApplicationID, Column1: arg.Column3, Name: arg.Name, Name_2: arg.Name_2,
		Limit: arg.Limit, Offset: arg.Offset,
	})
}

// ---- 4) ListProjectsRow：聚合列（string_agg）在 PG 侧是 interface{}，MySQL 侧是 sql.NullString ----

type ListProjectsRow struct {
	ID                  int64          `json:"id"`
	CreateTime          time.Time      `json:"create_time"`
	UpdateTime          time.Time      `json:"update_time"`
	Remark              sql.NullString `json:"remark"`
	Name                string         `json:"name"`
	Code                string         `json:"code"`
	Owner               string         `json:"owner"`
	Enabled             bool           `json:"enabled"`
	BusinessSystemNames sql.NullString `json:"business_system_names"`
	BusinessSystemIds   sql.NullString `json:"business_system_ids"`
}

func (q *Queries) ListProjects(ctx context.Context, arg ListProjectsParams) ([]ListProjectsRow, error) {
	rows, err := q.Queries.ListProjects(ctx, arg)
	if err != nil {
		return nil, err
	}
	converted := make([]ListProjectsRow, 0, len(rows))
	for _, row := range rows {
		converted = append(converted, ListProjectsRow{
			ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
			Name: row.Name, Code: row.Code, Owner: row.Owner, Enabled: row.Enabled,
			BusinessSystemNames: nullStringArg(row.BusinessSystemNames),
			BusinessSystemIds:   nullStringArg(row.BusinessSystemIds),
		})
	}
	return converted, nil
}

// ---- 5) INSERT 的 :execresult：PG 侧是 :one + RETURNING，包成 sql.Result 对齐 MySQL 侧签名 ----
//
// 为什么：MySQL 侧这两件事都靠驱动给的自增主键——`:execlastid` 由生成代码直接调
// `LastInsertId()`，`:execresult` 则由调用点（`assets/repository.go`、`rbac/repository.go` 等）
// 拿到 `sql.Result` 后调 `LastInsertId()`。PostgreSQL 的驱动层不实现它（pgx 对普通 Exec 返回
// `driver.RowsAffected`，其 `LastInsertId()` 恒报错），所以派生脚本把 INSERT 的这两类注解
// 都改成了 `:one` + `RETURNING id`（见 derive/derive.go）。
//
// 这里再把返回的 id 包成一个最小的 sql.Result，让两侧调用点继续
// `result.LastInsertId()` / `result.RowsAffected()`，不必分方言。
// 漏写某个适配方法不会静默：调用点编译不过（生成的方法返回 int32/int64，没有 LastInsertId）。

// insertResult 是「刚插入一行」的最小结果：两者都只有一个确定答案。
type insertResult struct{ id int64 }

func (result insertResult) LastInsertId() (int64, error) { return result.id, nil }
func (result insertResult) RowsAffected() (int64, error) { return 1, nil }

func (q *Queries) CreateMenu(ctx context.Context, arg CreateMenuParams) (sql.Result, error) {
	id, err := q.Queries.CreateMenu(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: int64(id)}, nil
}

func (q *Queries) CreateRole(ctx context.Context, arg CreateRoleParams) (sql.Result, error) {
	id, err := q.Queries.CreateRole(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: int64(id)}, nil
}

func (q *Queries) CreateUser(ctx context.Context, arg CreateUserParams) (sql.Result, error) {
	id, err := q.Queries.CreateUser(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: int64(id)}, nil
}

func (q *Queries) CreateAPIToken(ctx context.Context, arg CreateAPITokenParams) (sql.Result, error) {
	id, err := q.Queries.CreateAPIToken(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: int64(id)}, nil
}

func (q *Queries) CreateConfig(ctx context.Context, arg CreateConfigParams) (sql.Result, error) {
	id, err := q.Queries.CreateConfig(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateProject(ctx context.Context, arg CreateProjectParams) (sql.Result, error) {
	id, err := q.Queries.CreateProject(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateBusinessSystem(ctx context.Context, arg CreateBusinessSystemParams) (sql.Result, error) {
	id, err := q.Queries.CreateBusinessSystem(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateBusinessEnvironment(ctx context.Context, arg CreateBusinessEnvironmentParams) (sql.Result, error) {
	id, err := q.Queries.CreateBusinessEnvironment(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateCredential(ctx context.Context, arg CreateCredentialParams) (sql.Result, error) {
	id, err := q.Queries.CreateCredential(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateHostGroup(ctx context.Context, arg CreateHostGroupParams) (sql.Result, error) {
	id, err := q.Queries.CreateHostGroup(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateHost(ctx context.Context, arg CreateHostParams) (sql.Result, error) {
	id, err := q.Queries.CreateHost(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateApplication(ctx context.Context, arg CreateApplicationParams) (sql.Result, error) {
	id, err := q.Queries.CreateApplication(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateApplicationVersion(ctx context.Context, arg CreateApplicationVersionParams) (sql.Result, error) {
	id, err := q.Queries.CreateApplicationVersion(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateClusterProfile(ctx context.Context, arg CreateClusterProfileParams) (sql.Result, error) {
	id, err := q.Queries.CreateClusterProfile(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

func (q *Queries) CreateUserGroup(ctx context.Context, arg CreateUserGroupParams) (sql.Result, error) {
	id, err := q.Queries.CreateUserGroup(ctx, arg)
	if err != nil {
		return nil, err
	}
	return insertResult{id: id}, nil
}

// ---- 6) 可变长 IN 的元素类型分歧：列可空时 MySQL 给 []sql.NullInt64，PG 的 ::bigint[] 给 []int64 ----
//
// 为什么会有分歧：MySQL 侧 sqlc.slice(x) 的元素类型取自"被比较列"的可空性
// （assets_agent_job.host_id 可空 → sql.NullInt64），而 PG 侧的派生 override 显式写了
// `::bigint[]`，元素类型固定为 int64。语义上两者等价：集合成员判定里的 NULL 元素永远
// 不可能命中（SQL 里 NULL 的比较是 unknown），调用方传的也都是真实主机 id。这里只做类型换算。

func int64SliceArg(values []sql.NullInt64) []int64 {
	converted := make([]int64, 0, len(values))
	for _, value := range values {
		if value.Valid {
			converted = append(converted, value.Int64)
		}
	}
	return converted
}

func (q *Queries) CountActiveAgentInstallJobs(ctx context.Context, hostIds []sql.NullInt64) (int64, error) {
	return q.Queries.CountActiveAgentInstallJobs(ctx, int64SliceArg(hostIds))
}

type FailStaleAgentInstallJobsParams struct {
	FinishedAt  sql.NullTime    `json:"finished_at"`
	UpdateTime  time.Time       `json:"update_time"`
	HostIds     []sql.NullInt64 `json:"host_ids"`
	StaleBefore time.Time       `json:"stale_before"`
}

func (q *Queries) FailStaleAgentInstallJobs(ctx context.Context, arg FailStaleAgentInstallJobsParams) error {
	return q.Queries.FailStaleAgentInstallJobs(ctx, postgres.FailStaleAgentInstallJobsParams{
		FinishedAt: arg.FinishedAt, UpdateTime: arg.UpdateTime,
		HostIds: int64SliceArg(arg.HostIds), StaleBefore: arg.StaleBefore,
	})
}

// ---- 7) 宿主总览的过滤参数：字符串/切片在两侧的可空性推导不同 ----
//
// `sqlc.arg(search_pattern) = ''` 这类"空串表示不过滤"的写法，MySQL 引擎推断为 string、
// PG 引擎推断为 interface{}；group_ids 因列可空在 MySQL 侧是 []sql.NullInt64、PG 侧是 []int64。
// 语义一致，这里只做类型换算（sql.NullString 本身实现 Valuer，PG 侧直接透传即可）。

type CountMonitorHostsParams struct {
	SearchPattern sql.NullString  `json:"search_pattern"`
	GroupIds      []sql.NullInt64 `json:"group_ids"`
	GroupFilter   interface{}     `json:"group_filter"`
	ManagedFilter interface{}     `json:"managed_filter"`
	ExporterType  string          `json:"exporter_type"`
	FluentFilter  interface{}     `json:"fluent_filter"`
}

func (q *Queries) CountMonitorHosts(ctx context.Context, arg CountMonitorHostsParams) (int64, error) {
	return q.Queries.CountMonitorHosts(ctx, postgres.CountMonitorHostsParams{
		SearchPattern: arg.SearchPattern, GroupIds: int64SliceArg(arg.GroupIds),
		GroupFilter: arg.GroupFilter, ManagedFilter: arg.ManagedFilter,
		ExporterType: arg.ExporterType, FluentFilter: arg.FluentFilter,
	})
}

type ListMonitorHostsParams struct {
	SearchPattern sql.NullString  `json:"search_pattern"`
	GroupIds      []sql.NullInt64 `json:"group_ids"`
	GroupFilter   interface{}     `json:"group_filter"`
	ManagedFilter interface{}     `json:"managed_filter"`
	ExporterType  string          `json:"exporter_type"`
	FluentFilter  interface{}     `json:"fluent_filter"`
	Limit         int32           `json:"limit"`
	Offset        int32           `json:"offset"`
}

func (q *Queries) ListMonitorHosts(ctx context.Context, arg ListMonitorHostsParams) ([]ListMonitorHostsRow, error) {
	return q.Queries.ListMonitorHosts(ctx, postgres.ListMonitorHostsParams{
		Limit: arg.Limit, Offset: arg.Offset,
		SearchPattern: arg.SearchPattern, GroupIds: int64SliceArg(arg.GroupIds),
		GroupFilter: arg.GroupFilter, ManagedFilter: arg.ManagedFilter,
		ExporterType: arg.ExporterType, FluentFilter: arg.FluentFilter,
	})
}

// ---- 转换 helper：三种参数形态的显式换算 ----

// labelKeyArg 把调用方传的 interface{}（约定为 string）转成 PG 需要的标签键文本。
func labelKeyArg(value any) string {
	text, ok := value.(string)
	if !ok {
		return fmt.Sprint(value)
	}
	return text
}

// nullStringArg 把 interface{} 转成 sql.NullString：nil 保持 NULL，其余按文本处理。
// 覆盖 string / sql.NullString / []byte（PG 驱动态扫描 text 常给 []byte）三种实际形态。
func nullStringArg(value any) sql.NullString {
	switch typed := value.(type) {
	case nil:
		return sql.NullString{}
	case string:
		return sql.NullString{String: typed, Valid: true}
	case []byte:
		return sql.NullString{String: string(typed), Valid: true}
	case sql.NullString:
		return typed
	default:
		return sql.NullString{String: fmt.Sprint(typed), Valid: true}
	}
}
