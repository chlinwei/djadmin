package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// TestSmokeInspectionQueriesAgainstRealDatabase 把 inspection 域迁移后的 sqlc 查询在**真库**上
// 按执行流程跑一遍（建组/改组/建任务/建执行/目标状态流转/结果落库/取消/保留期清理/级联删除）。
//
// 为什么需要它：sqlmock 用例只验证调用形态，验不了驱动层的参数类型与方言行为——baseline 试点
// 就是靠真库才暴露 `:execlastid` 在 PostgreSQL 上取不回主键（pgx 不实现 LastInsertId）。
// 本包用到的 sqlc 里还有几处只能在真库上验的东西：可变长 IN 的数组参数（P4-7 的
// ListHostBusinessChains 就是它）、COUNT(CASE WHEN ...) 的零行结果、json 列的写入。
//
// 与 baseline 冒烟的差别：巡检的执行链路由 handler 方法自己开事务、且要 Agent 网关，
// 所以这里只做"域内可自洽"的部分（不解析挂载点、不下发 Agent），用完的行显式清理。
//
// 用法：
//
//	INSPECTION_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/inspection/ -run RealDatabase -v
//	INSPECTION_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/inspection/ -run RealDatabase -v
func TestSmokeInspectionQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("INSPECTION_SMOKE_DSN")
	if dsn == "" {
		t.Skip("INSPECTION_SMOKE_DSN 未设置：跳过真库冒烟（两个方言各跑一次的说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect(%s): %v", redactSmokeDSN(dsn), err)
	}
	defer pool.Close()
	queries, handler := db.New(pool), &Handler{db: pool}
	now := time.Now().UTC()
	suffix := now.Format("150405.000000")
	var groupID, taskID, executionID, targetID int64

	// host_id 有物理外键指向 assets_host（注意：db/schema 的快照没声明这条外键，
	// 只有真库上有），所以取一台真实主机；库里没有主机就写 NULL（列可空）。
	hostID := sql.NullInt64{}
	var existingHost int64
	if err := pool.QueryRowContext(ctx, "SELECT id FROM assets_host ORDER BY id LIMIT 1").Scan(&existingHost); err == nil {
		hostID = sql.NullInt64{Int64: existingHost, Valid: true}
	}

	// 收尾：只删本次创建的行（用 id 精确删除）。注意不能用保留期清理语句来收尾——
	// 它的 cutoff 是全局的，会把库里其他人的历史执行记录一起删掉；那三条语句在下面对
	// **回滚事务**里验证。这里的裸 SQL 是测试脚手架：生产代码里删执行行没有对应入口。
	defer func() {
		if executionID > 0 {
			pool.ExecContext(ctx, "DELETE FROM inspection_result WHERE target_id IN (SELECT id FROM inspection_target_execution WHERE execution_id = ?)", executionID)
			pool.ExecContext(ctx, "DELETE FROM inspection_target_execution WHERE execution_id = ?", executionID)
			pool.ExecContext(ctx, "DELETE FROM inspection_execution WHERE id = ?", executionID)
		}
		if taskID > 0 {
			queries.DetachInspectionExecutionsFromTask(ctx, db.DetachInspectionExecutionsFromTaskParams{
				TaskID: sql.NullInt64{Int64: taskID, Valid: true}, UpdateTime: time.Now().UTC()})
			queries.DeleteInspectionTaskGroups(ctx, taskID)
			queries.DeleteInspectionTask(ctx, taskID)
		}
		if groupID > 0 {
			queries.DeleteInspectionChecksByGroup(ctx, groupID)
			queries.DeleteInspectionGroup(ctx, groupID)
		}
	}()

	// 建组（:execlastid 取主键）+ 检查项 + 检查项整表替换。
	groupID, err = queries.CreateInspectionGroup(ctx, db.CreateInspectionGroupParams{
		CreateTime: now, UpdateTime: now, Name: "smoke-group-" + suffix, Description: "冒烟",
		Enabled: true, Category: "general", Params: json.RawMessage("[]"),
	})
	if err != nil {
		t.Fatalf("建组：%v", err)
	}
	// 名字唯一键冲突必须是业务错误（MySQL 1062 / PG 23505），不能静默成功。
	if _, err = queries.CreateInspectionGroup(ctx, db.CreateInspectionGroupParams{
		CreateTime: now, UpdateTime: now, Name: "smoke-group-" + suffix, Description: "",
		Enabled: true, Category: "general", Params: json.RawMessage("[]"),
	}); err == nil {
		t.Fatalf("重名建组应报错")
	}
	if err = queries.CreateInspectionCheck(ctx, db.CreateInspectionCheckParams{
		GroupID: groupID, Name: "check-1", Config: json.RawMessage(`{"policy":"package x"}`),
		Severity: "critical", Enabled: true, CheckOrder: 0, CreateTime: now, UpdateTime: now,
	}); err != nil {
		t.Fatalf("建检查项：%v", err)
	}
	// 组的部分更新（PATCH 合并：FOR UPDATE 读回 + 整行写）。
	current, err := queries.GetInspectionGroupForUpdate(ctx, groupID)
	if err != nil {
		t.Fatalf("读组现值：%v", err)
	}
	if current.Name == "" || string(current.Params) == "" {
		t.Fatalf("读回的组现值不完整：%+v", current)
	}
	if _, err = queries.UpdateInspectionGroup(ctx, db.UpdateInspectionGroupParams{
		Name: current.Name, Description: "冒烟-改名", Enabled: false, Category: current.Category,
		ApplicationID: current.ApplicationID, Params: current.Params, UpdateTime: time.Now().UTC(), ID: groupID,
	}); err != nil {
		t.Fatalf("改组：%v", err)
	}
	meta, err := queries.GetInspectionGroupRunMeta(ctx, groupID)
	if err != nil {
		t.Fatalf("读组运行元信息：%v", err)
	}
	if meta.Enabled || meta.EnabledCheckCount != 1 {
		t.Fatalf("组运行元信息 = %+v，期望 enabled=false 且启用检查项 1 个", meta)
	}

	// 建任务（:execlastid）+ 绑定行 + 同组重名计数 + 任务状态读回。
	taskID, err = queries.CreateInspectionTask(ctx, db.CreateInspectionTaskParams{
		Name: "smoke-task-" + suffix, InspectionName: "冒烟", GroupID: groupID, Concurrency: 5,
		TimeoutSeconds: 60, CronExpression: "", NextRunTime: sql.NullTime{}, Enabled: true,
		CreateTime: now, UpdateTime: now,
	})
	if err != nil {
		t.Fatalf("建任务：%v", err)
	}
	if err = queries.CreateInspectionTaskGroup(ctx, db.CreateInspectionTaskGroupParams{
		TaskID: taskID, GroupID: groupID, MountType: mountProject, ProjectID: sql.NullInt64{Int64: 1, Valid: true},
		ParamValues: json.RawMessage("{}"),
	}); err != nil {
		t.Fatalf("建任务绑定：%v", err)
	}
	if count, err := queries.CountInspectionTasksByNameInGroup(ctx, db.CountInspectionTasksByNameInGroupParams{
		Name: "smoke-task-" + suffix, GroupID: groupID, ExcludeID: 0,
	}); err != nil || count != 1 {
		t.Fatalf("同组重名计数 = %d, %v，期望 1", count, err)
	}
	state, err := queries.GetInspectionTaskState(ctx, taskID)
	if err != nil {
		t.Fatalf("读任务状态：%v", err)
	}
	if state.Name != "smoke-task-"+suffix || state.Concurrency != 5 {
		t.Fatalf("任务状态 = %+v", state)
	}
	// 任务状态更新（列表/表单共用的整行写）。
	if _, err = queries.UpdateInspectionTask(ctx, db.UpdateInspectionTaskParams{
		Name: state.Name, InspectionName: state.InspectionName, GroupID: groupID,
		Concurrency: state.Concurrency, TimeoutSeconds: state.TimeoutSeconds,
		CronExpression: "*/5 * * * *", NextRunTime: sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
		Enabled: true, UpdateTime: time.Now().UTC(), ID: taskID,
	}); err != nil {
		t.Fatalf("更新任务：%v", err)
	}
	if count, err := queries.CountInspectionTasksByGroup(ctx, groupID); err != nil || count != 1 {
		t.Fatalf("按组统计任务 = %d, %v，期望 1", count, err)
	}
	if _, err = queries.GetInspectionTaskRunState(ctx, taskID); err != nil {
		t.Fatalf("读任务运行状态：%v", err)
	}
	// 调度器的两条语句：到期列表 + 原子认领（next_run_time 在同一 UPDATE 里前移，
	// 重复认领必须影响 0 行——这是防多副本重复触发的锚点）。
	due, err := queries.ListDueInspectionTasks(ctx, sql.NullTime{Time: time.Now().UTC(), Valid: true})
	if err != nil {
		t.Fatalf("列到期任务：%v", err)
	}
	found := false
	for _, row := range due {
		if row.ID == taskID {
			found = true
		}
	}
	if !found {
		t.Fatalf("到期任务列表里没有刚把 next_run_time 设成过去的任务（共 %d 条）", len(due))
	}
	next := sql.NullTime{Time: time.Now().UTC().Add(time.Hour), Valid: true}
	if claimed, err := queries.ClaimDueInspectionTask(ctx, db.ClaimDueInspectionTaskParams{
		NextRunTime: next, UpdateTime: time.Now().UTC(), ID: taskID,
		Now: sql.NullTime{Time: time.Now().UTC(), Valid: true},
	}); err != nil || claimed != 1 {
		t.Fatalf("认领到期任务：claimed=%d err=%v", claimed, err)
	}
	if claimed, err := queries.ClaimDueInspectionTask(ctx, db.ClaimDueInspectionTaskParams{
		NextRunTime: next, UpdateTime: time.Now().UTC(), ID: taskID,
		Now: sql.NullTime{Time: time.Now().UTC(), Valid: true},
	}); err != nil || claimed != 0 {
		t.Fatalf("重复认领应返回 0：claimed=%d err=%v", claimed, err)
	}
	// 资产侧存在性校验（通用组/应用组与挂载点保存时用）。
	if count, err := queries.CountApplicationByID(ctx, 0); err != nil || count != 0 {
		t.Fatalf("不存在的应用计数 = %d, %v", count, err)
	}
	if count, err := queries.CountApplicationServiceByID(ctx, 0); err != nil || count != 0 {
		t.Fatalf("不存在的逻辑服务计数 = %d, %v", count, err)
	}
	bindings, err := queries.ListInspectionTaskBindings(ctx, taskID)
	if err != nil || len(bindings) != 1 {
		t.Fatalf("任务绑定 = %d, %v，期望 1", len(bindings), err)
	}
	// 绑定行的可空列必须真能读回 NULL（PG 侧曾因接口参数类型不一致炸过）。
	if bindings[0].ProjectID.Valid != true || bindings[0].EnvironmentID.Valid {
		t.Fatalf("绑定行可空列不符合预期：%+v", bindings[0])
	}

	// 执行落库：execution → target → 状态流转 → 结果。
	executionID, err = queries.CreateInspectionExecution(ctx, db.CreateInspectionExecutionParams{
		TaskID: sql.NullInt64{Int64: taskID, Valid: true}, TriggerType: "manual",
		TaskSnapshot: json.RawMessage(`{"id":1}`), GroupSnapshot: json.RawMessage("[]"),
		ServiceSnapshot: json.RawMessage(`{"name":"smoke"}`), TargetSnapshot: json.RawMessage("[]"),
		RequestedUserID: sql.NullInt32{Int32: 0, Valid: true}, RequestedUsername: "smoke",
		CreateTime: now, UpdateTime: now,
	})
	if err != nil {
		t.Fatalf("建执行：%v", err)
	}
	targetID, err = queries.CreateInspectionTargetExecution(ctx, db.CreateInspectionTargetExecutionParams{
		ExecutionID: executionID, DeploymentID: sql.NullInt64{}, HostID: hostID,
		TargetName: "smoke-target", HostIDSnapshot: sql.NullInt32{Int32: int32(hostID.Int64), Valid: hostID.Valid},
		HostIpSnapshot: "127.0.0.1", InstanceNameSnapshot: "smoke-host", CreateTime: now, UpdateTime: now,
	})
	if err != nil {
		t.Fatalf("建执行目标：%v", err)
	}
	if err = queries.TouchInspectionTaskLastRun(ctx, db.TouchInspectionTaskLastRunParams{
		LastRunTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: taskID,
	}); err != nil {
		t.Fatalf("更新任务最近运行时间：%v", err)
	}
	claimed, err := queries.MarkInspectionExecutionRunning(ctx, db.MarkInspectionExecutionRunningParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: executionID,
	})
	if err != nil || claimed != 1 {
		t.Fatalf("执行置 running：claimed=%d err=%v", claimed, err)
	}
	if err = queries.MarkInspectionTargetRunning(ctx, db.MarkInspectionTargetRunningParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: targetID,
	}); err != nil {
		t.Fatalf("目标置 running：%v", err)
	}
	// 结果落库：expected/actual 为 NULL 时写 NULL（可空 json 列的写入路径）。
	if err = queries.CreateInspectionResult(ctx, db.CreateInspectionResultParams{
		TargetID: targetID, CheckKey: "inspection:1:0", CheckType: "opa", Name: "check-1",
		Status: "fail", Severity: "warning", GroupID: sql.NullInt64{Int64: groupID, Valid: true}, GroupName: "smoke",
		ExpectedValue: nil, ActualValue: json.RawMessage(`{"ok":false}`), Message: "冒烟",
		CreateTime: now, UpdateTime: now,
	}); err != nil {
		t.Fatalf("落检查结果：%v", err)
	}
	results, err := queries.ListInspectionResultsByExecution(ctx, executionID)
	if err != nil || len(results) != 1 {
		t.Fatalf("读检查结果 = %d, %v，期望 1", len(results), err)
	}
	if results[0].GroupID.Int64 != groupID || string(results[0].ExpectedValue) != "null" {
		t.Fatalf("检查结果字段不符：%+v", results[0])
	}
	warnings, err := queries.CountInspectionWarningResults(ctx, executionID)
	if err != nil || warnings != 1 {
		t.Fatalf("warning 计数 = %d, %v，期望 1", warnings, err)
	}
	// COUNT(CASE WHEN ...)：零行时必须是 0 而不是 NULL（PG 侧 NULL 会 Scan 失败）。
	emptyOutcomes, err := queries.CountInspectionTargetOutcomes(ctx, executionID+1_000_000)
	if err != nil {
		t.Fatalf("零行目标统计：%v", err)
	}
	if emptyOutcomes.Failed != 0 || emptyOutcomes.Success != 0 {
		t.Fatalf("零行目标统计应为全 0，实得 %+v", emptyOutcomes)
	}
	// 目标跳过与取消（离线跳过、执行中被取消两条分支各自的目标级语句）。
	if err = queries.SkipInspectionTarget(ctx, db.SkipInspectionTargetParams{
		ErrorMessage: "Agent 离线，未执行巡检", EndTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("目标置 skipped：%v", err)
	}
	if err = queries.CancelInspectionTarget(ctx, db.CancelInspectionTargetParams{
		EndTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("目标置 canceled：%v", err)
	}
	// 目标收尾：结果写入失败路径改写 error_message，再置成功/失败。
	passed := false
	if err = queries.FinishInspectionTarget(ctx, db.FinishInspectionTargetParams{
		Status: "failed", Passed: &passed, ErrorMessage: "冒烟失败", RawResult: json.RawMessage(`{"passed":false}`),
		EndTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("目标收尾：%v", err)
	}
	outcomes, err := queries.CountInspectionTargetOutcomes(ctx, executionID)
	if err != nil || outcomes.Failed != 1 {
		t.Fatalf("目标统计 = %+v, %v，期望 failed=1", outcomes, err)
	}

	// 业务链路快照查询：可变长 IN（PG 侧派生为 = ANY($1::bigint[])，P4-7 的回归点）。
	// 三个值（含库里没有的 id）：P4-7 的故障正是"值个数 > 1 时占位符与参数不符"，
	// 与是否命中行无关，用不存在的 id 也能覆盖按 PG 数组参数的编码路径。
	chains, err := queries.ListHostBusinessChains(ctx, []int64{hostID.Int64, hostID.Int64 + 1, hostID.Int64 + 2})
	if err != nil {
		t.Fatalf("业务链路快照（多值 IN）：%v", err)
	}
	if _, err = queries.ListHostBusinessChains(ctx, []int64{}); err != nil {
		t.Fatalf("业务链路快照（空 IN）：%v", err)
	}
	t.Logf("业务链路快照返回 %d 行（主机不一定是应用部署目标，0 行也正常）", len(chains))

	// 挂载点解析用到的资产侧命名查询：id 不存在时应是 ErrNoRows，不是驱动错误。
	if _, err = queries.GetProjectNameByID(ctx, 0); err != sql.ErrNoRows {
		t.Fatalf("不存在的项目：期望 sql.ErrNoRows，实得 %v", err)
	}
	if _, err = queries.GetBusinessEnvironmentNameByID(ctx, 0); err != sql.ErrNoRows {
		t.Fatalf("不存在的环境：期望 sql.ErrNoRows，实得 %v", err)
	}
	if _, err = queries.GetApplicationServiceName(ctx, 0); err != sql.ErrNoRows {
		t.Fatalf("不存在的逻辑服务：期望 sql.ErrNoRows，实得 %v", err)
	}

	// 取消执行（FOR UPDATE 读回 + summary 应用层合并 + 目标批量置取消）。
	handler.db = pool
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Params = gin.Params{{Key: "id", Value: fmt.Sprint(executionID)}}
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	handler.CancelExecution(ginContext)
	if !strings.Contains(recorder.Body.String(), `"canceled"`) {
		t.Fatalf("取消执行响应 = %s", recorder.Body.String())
	}
	canceled, err := queries.GetInspectionExecutionForUpdate(ctx, executionID)
	if err != nil {
		t.Fatalf("取消后读执行：%v", err)
	}
	if canceled.Status != "canceled" {
		t.Fatalf("取消后状态 = %q", canceled.Status)
	}
	// summary 里的 canceled 标记由应用层合并（原实现是 MySQL 的 JSON_SET）。
	var summary map[string]any
	if json.Unmarshal(canceled.Summary, &summary) != nil || summary["canceled"] != true {
		t.Fatalf("取消后 summary = %s，期望含 canceled=true", canceled.Summary)
	}

	// 执行收尾（`execute` 在无 Agent 网关时也会走到，这里直接验语句）。
	if _, err = queries.FinishInspectionExecution(ctx, db.FinishInspectionExecutionParams{
		Status: "success", Summary: json.RawMessage(`{"total":1}`),
		EndTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, UpdateTime: time.Now().UTC(), ID: executionID,
	}); err != nil {
		t.Fatalf("执行收尾：%v", err)
	}


	// 保留期清理（多表 DELETE 的改写）：cutoff 放到将来，三条语句应在**回滚事务**里把刚
	// 终态的执行删掉。放在事务里是为了不误删库里别人的历史记录——这条语句的 cutoff 是全局的。
	cleanupTx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin 清理事务：%v", err)
	}
	cleanupQueries := db.New(cleanupTx)
	step := sql.NullTime{Time: time.Now().UTC().Add(time.Hour), Valid: true}
	if _, err = cleanupQueries.DeleteFinishedInspectionResults(ctx, step); err != nil {
		cleanupTx.Rollback()
		t.Fatalf("清理结果：%v", err)
	}
	if _, err = cleanupQueries.DeleteFinishedInspectionTargetExecutions(ctx, step); err != nil {
		cleanupTx.Rollback()
		t.Fatalf("清理执行目标：%v", err)
	}
	if _, err = cleanupQueries.DeleteFinishedInspectionExecutions(ctx, step); err != nil {
		cleanupTx.Rollback()
		t.Fatalf("清理执行：%v", err)
	}
	if _, err = cleanupQueries.GetInspectionExecutionForUpdate(ctx, executionID); err != sql.ErrNoRows {
		cleanupTx.Rollback()
		t.Fatalf("清理后执行应不存在，实得 %v", err)
	}
	if err = cleanupTx.Rollback(); err != nil {
		t.Fatalf("回滚清理事务：%v", err)
	}
	// 回滚后行还在（证明上面的删除确实发生在事务里，没有动到库里其他数据）。
	if _, err = queries.GetInspectionExecutionForUpdate(ctx, executionID); err != nil {
		t.Fatalf("回滚后执行应仍在：%v", err)
	}

	// 任务级联删除：执行记录解绑、绑定行删除、任务删除。
	if err = queries.DetachInspectionExecutionsFromTask(ctx, db.DetachInspectionExecutionsFromTaskParams{
		TaskID: sql.NullInt64{Int64: taskID, Valid: true}, UpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("执行解绑任务：%v", err)
	}
	if err = queries.DeleteInspectionTaskGroups(ctx, taskID); err != nil {
		t.Fatalf("删任务绑定：%v", err)
	}
	if affected, err := queries.DeleteInspectionTask(ctx, taskID); err != nil || affected != 1 {
		t.Fatalf("删任务：affected=%d err=%v", affected, err)
	}
	taskID = 0
	// 组内检查项由应用层级联删除（物理外键是 NO ACTION，跳过这步 PG 会拒绝删组）。
	if err = queries.DeleteInspectionChecksByGroup(ctx, groupID); err != nil {
		t.Fatalf("删组内检查项：%v", err)
	}
	if affected, err := queries.DeleteInspectionGroup(ctx, groupID); err != nil || affected != 1 {
		t.Fatalf("删组：affected=%d err=%v", affected, err)
	}
	groupID = 0
}

// redactSmokeDSN 只保留主机与库名，避免把口令写进测试日志。
func redactSmokeDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return "***" + dsn[at:]
	}
	return dsn
}
