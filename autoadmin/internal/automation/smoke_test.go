package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/google/uuid"
)

// TestSmokeAutomationQueriesAgainstRealDatabase 把 automation 域迁移后的 sqlc 查询在**真库**上
// 按业务流程跑一遍（模板 CRUD → Inventory CRUD → 任务 CRUD → 作业派发/认领/收尾 → 主机日志 → 目标主机解析）。
//
// 为什么需要它：sqlmock 验不了驱动层的参数类型与方言行为。本包里有几处只有真库能验：
// `:execlastid`（PG 侧靠 RETURNING 取主键）、可空 json/布尔列的写入、`IN (sqlc.slice(x))`
// 的可变长数组参数（PG 侧是 `= ANY($1::bigint[])`）、以及 double 列（duration_seconds）
// 的写入。**每个包迁 sqlc 后都应在两个方言上各跑一次。**
//
// 库里没有主机的场景（assets_host 为空）会自动跳过主机相关步骤：其余步骤不依赖资产数据。
// 所有创建的行在结束时显式删除（handler 方法各自开事务，包不进一层事务里）。
//
// 用法：
//
//	AUTOMATION_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/automation/ -run RealDatabase -v
//	AUTOMATION_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/automation/ -run RealDatabase -v
func TestSmokeAutomationQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("AUTOMATION_SMOKE_DSN")
	if dsn == "" {
		t.Skip("AUTOMATION_SMOKE_DSN 未设置：跳过真库冒烟（两个方言各跑一次的说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect(%s): %v", redactSmokeDSN(dsn), err)
	}
	defer pool.Close()
	queries := db.New(pool)
	now := time.Now().UTC()
	suffix := now.Format("150405.000000")
	var playbookID, inventoryID, taskID, jobID int64

	// 收尾：只删本次创建的行（按 id，顺序服从外键：主机日志 → 作业 → 任务 → Inventory → 模板）。
	// 作业与主机日志在生产代码里没有删除入口（只有派发/收尾），这两条裸 SQL 是测试脚手架。
	defer func() {
		if jobID > 0 {
			pool.ExecContext(ctx, "DELETE FROM automation_execution_host_log WHERE job_id = ?", jobID)
			pool.ExecContext(ctx, "DELETE FROM automation_execution_job WHERE id = ?", jobID)
		}
		if taskID > 0 {
			queries.DeleteAutomationTask(ctx, taskID)
		}
		if inventoryID > 0 {
			queries.DeleteAutomationInventory(ctx, inventoryID)
		}
		if playbookID > 0 {
			queries.DeleteAutomationPlaybook(ctx, playbookID)
		}
	}()

	// 模板 CRUD：:execlastid 取主键 → 改名 → 覆盖内容（上传走的就是这条）→ 按条件搜索。
	playbookID, err = queries.CreateAutomationPlaybook(ctx, db.CreateAutomationPlaybookParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-playbook-" + suffix,
		Description: "冒烟", Content: "- hosts: all\n", Category: "general",
	})
	if err != nil {
		t.Fatalf("建模板：%v", err)
	}
	playbook, err := queries.GetAutomationPlaybook(ctx, playbookID)
	if err != nil {
		t.Fatalf("读模板：%v", err)
	}
	if playbook.Name != "smoke-playbook-"+suffix || playbook.Content != "- hosts: all\n" {
		t.Fatalf("模板字段不符：%+v", playbook)
	}
	if err = queries.UpdateAutomationPlaybook(ctx, db.UpdateAutomationPlaybookParams{
		UpdateTime: time.Now().UTC(), Remark: sql.NullString{String: "改过", Valid: true},
		Name: playbook.Name, Description: playbook.Description, Content: "- hosts: web\n", Category: playbook.Category, ID: playbookID,
	}); err != nil {
		t.Fatalf("改模板：%v", err)
	}
	affected, err := queries.UpdateAutomationPlaybookContent(ctx, db.UpdateAutomationPlaybookContentParams{
		Content: "- hosts: db\n", UpdateTime: time.Now().UTC(), ID: playbookID,
	})
	if err != nil || affected != 1 {
		t.Fatalf("覆盖模板内容：affected=%d err=%v", affected, err)
	}
	pattern := sql.NullString{String: "%smoke-playbook-" + suffix + "%", Valid: true}
	if count, err := queries.CountAutomationPlaybooks(ctx, db.CountAutomationPlaybooksParams{Pattern: pattern}); err != nil || count != 1 {
		t.Fatalf("模板计数 = %d, %v，期望 1", count, err)
	}
	// 排序参数按八个分支之一走（此前是运行时拼列名，现在是 CASE 表达式）。
	books, err := queries.ListAutomationPlaybooks(ctx, db.ListAutomationPlaybooksParams{
		Pattern: pattern, Category: sql.NullString{}, SortKey: "-update_time", Limit: 10, Offset: 0,
	})
	if err != nil || len(books) != 1 {
		t.Fatalf("模板列表 = %d, %v，期望 1", len(books), err)
	}
	if books[0].Content != "- hosts: db\n" {
		t.Fatalf("模板内容未更新：%q", books[0].Content)
	}

	// Inventory CRUD：json 列（selected_host_ids）+ unsigned 列（update_cache_timeout）。
	inventoryID, err = queries.CreateAutomationInventory(ctx, db.CreateAutomationInventoryParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-inventory-" + suffix,
		SelectedHostIds: json.RawMessage("[]"), Enabled: true, UpdateOnLaunch: false, UpdateCacheTimeout: 300,
	})
	if err != nil {
		t.Fatalf("建 Inventory：%v", err)
	}
	inventory, err := queries.GetInventoryTyped(ctx, inventoryID)
	if err != nil {
		t.Fatalf("读 Inventory：%v", err)
	}
	if inventory.UpdateCacheTimeout != 300 || !inventory.Enabled || inventory.LastSyncStatus != "never" {
		t.Fatalf("Inventory 字段不符：%+v", inventory)
	}
	if err = queries.UpdateAutomationInventory(ctx, db.UpdateAutomationInventoryParams{
		UpdateTime: time.Now().UTC(), Remark: sql.NullString{}, Name: inventory.Name,
		SelectedHostIds: json.RawMessage("[]"), Enabled: false, UpdateOnLaunch: true,
		UpdateCacheTimeout: 0, ID: inventoryID,
	}); err != nil {
		t.Fatalf("改 Inventory：%v", err)
	}
	inventory, err = queries.GetInventoryTyped(ctx, inventoryID)
	if err != nil || inventory.UpdateCacheTimeout != 0 || inventory.Enabled || !inventory.UpdateOnLaunch {
		t.Fatalf("Inventory 更新未生效：%+v, %v", inventory, err)
	}

	// 任务 CRUD：可空外键（inventory_id / playbook_template_id）+ 列表查询。
	taskID, err = queries.CreateAutomationTask(ctx, db.CreateAutomationTaskParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-task-" + suffix,
		PlaybookTemplateID: sql.NullInt64{Int64: playbookID, Valid: true},
		InventoryID:        sql.NullInt64{Int64: inventoryID, Valid: true},
		EnvVars:            json.RawMessage("{}"), DefaultLimit: "", Enabled: true,
		ExecutionTimeoutSeconds: 600, RunAsUser: "root", RunAsGroup: "", WorkDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("建任务：%v", err)
	}
	if err = queries.SetAutomationTaskEnabled(ctx, db.SetAutomationTaskEnabledParams{
		Enabled: false, UpdateTime: time.Now().UTC(), ID: taskID,
	}); err != nil {
		t.Fatalf("任务启用开关：%v", err)
	}
	task, err := queries.GetTaskTyped(ctx, taskID)
	if err != nil {
		t.Fatalf("读任务：%v", err)
	}
	if task.Enabled || task.TemplateName != "smoke-playbook-"+suffix || task.InventoryName != inventory.Name {
		t.Fatalf("任务字段不符：%+v", task)
	}
	taskPattern := sql.NullString{String: "%smoke-task-" + suffix + "%", Valid: true}
	if count, err := queries.CountTasks(ctx, db.CountTasksParams{ID: sql.NullInt64{}, Pattern: taskPattern}); err != nil || count != 1 {
		t.Fatalf("任务计数 = %d, %v，期望 1", count, err)
	}
	if err = queries.UpdateAutomationTask(ctx, db.UpdateAutomationTaskParams{
		UpdateTime: time.Now().UTC(), Remark: sql.NullString{}, Name: task.Name,
		PlaybookTemplateID: task.PlaybookTemplateID, InventoryID: task.InventoryID,
		EnvVars: json.RawMessage(`{"A":"1"}`), DefaultLimit: task.DefaultLimit, Enabled: true,
		ExecutionTimeoutSeconds: task.ExecutionTimeoutSeconds, RunAsUser: task.RunAsUser,
		RunAsGroup: task.RunAsGroup, WorkDirectory: task.WorkDirectory, ID: taskID,
	}); err != nil {
		t.Fatalf("改任务：%v", err)
	}

	// 作业派发 → 认领 → 收尾（含 double 列 duration_seconds 与可空外键 task_id）。
	jobID, err = queries.CreateAutomationJob(ctx, db.CreateAutomationJobParams{
		CreateTime: now, UpdateTime: now, JobID: uuid.NewString(),
		TaskID:            sql.NullInt64{Int64: taskID, Valid: true},
		InventorySnapshot: json.RawMessage(`{"selected_host_ids":[],"hosts":[]}`),
		TaskNameSnapshot:  task.Name, TemplateNameSnapshot: "smoke-playbook-" + suffix,
		TemplateContentSnapshot: "- hosts: all\n", ExtraVars: json.RawMessage("{}"), JobLimit: "",
		ResultSummary: json.RawMessage(`{"message":"queued"}`), RunAsUserSnapshot: "root",
		RunAsGroupSnapshot: "", WorkDirectorySnapshot: "/tmp", RequestedUsername: "",
	})
	if err != nil {
		t.Fatalf("建作业：%v", err)
	}
	claimed, err := queries.ClaimAutomationJob(ctx, db.ClaimAutomationJobParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, ResultSummary: json.RawMessage(`{"message":"running"}`),
		UpdateTime: now, ID: jobID,
	})
	if err != nil || claimed != 1 {
		t.Fatalf("认领作业：claimed=%d err=%v", claimed, err)
	}
	// 重复认领必须是 0（status 已不是 pending），这是防重复执行的锚点。
	if claimed, err = queries.ClaimAutomationJob(ctx, db.ClaimAutomationJobParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, ResultSummary: json.RawMessage(`{"message":"running"}`),
		UpdateTime: now, ID: jobID,
	}); err != nil || claimed != 0 {
		t.Fatalf("重复认领应返回 0：claimed=%d err=%v", claimed, err)
	}
	if _, err = queries.FinishAutomationJob(ctx, db.FinishAutomationJobParams{
		Status: "success", EndTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: 1.5, Valid: true},
		ResultSummary:   json.RawMessage(`{"message":"done"}`), UpdateTime: time.Now().UTC(), ID: jobID,
	}); err != nil {
		t.Fatalf("收尾作业：%v", err)
	}
	job, err := queries.GetJobTyped(ctx, jobID)
	if err != nil {
		t.Fatalf("读作业：%v", err)
	}
	if job.Status != "success" || !job.DurationSeconds.Valid || job.DurationSeconds.Float64 != 1.5 {
		t.Fatalf("作业字段不符：status=%q duration=%+v", job.Status, job.DurationSeconds)
	}
	// 取消路径：读 start_time → 算时长 → 置 cancelled（status<>'pending'/'running' 时为 0 行）。
	if _, err = queries.GetAutomationJobStartTime(ctx, jobID); err != nil {
		t.Fatalf("读作业开始时间：%v", err)
	}
	if canceled, err := queries.CancelAutomationJob(ctx, db.CancelAutomationJobParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, EndTime: sql.NullTime{Time: now, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: 2, Valid: true},
		ResultSummary:   json.RawMessage(`{"message":"cancelled"}`), UpdateTime: now, ID: jobID,
	}); err != nil || canceled != 0 {
		t.Fatalf("已结束的作业不可取消：canceled=%d err=%v", canceled, err)
	}

	// 主机日志：可空 int（exit_code / host_id_snapshot）+ json 列（result_data）。
	if err = queries.CreateAutomationJobHostLog(ctx, db.CreateAutomationJobHostLogParams{
		CreateTime: now, UpdateTime: now, JobID: jobID,
		HostID: sql.NullInt64{}, HostIDSnapshot: sql.NullInt32{}, HostNameSnapshot: "", HostIpSnapshot: "",
		Status: "failed", ExitCode: sql.NullInt32{}, Stdout: "", Stderr: "", ErrorMessage: "冒烟失败",
	}); err != nil {
		t.Fatalf("落主机日志（NULL exit_code）：%v", err)
	}
	if err = queries.CreateAutomationJobHostLog(ctx, db.CreateAutomationJobHostLogParams{
		CreateTime: now, UpdateTime: now, JobID: jobID,
		HostID: sql.NullInt64{}, HostIDSnapshot: sql.NullInt32{}, HostNameSnapshot: "smoke-host",
		HostIpSnapshot: "127.0.0.1", Status: "success", ExitCode: sql.NullInt32{Int32: 0, Valid: true},
		Stdout: "ok\n", Stderr: "", ErrorMessage: "",
	}); err != nil {
		t.Fatalf("落主机日志：%v", err)
	}
	logs, err := queries.ListAutomationJobHostLogs(ctx, jobID)
	if err != nil || len(logs) != 2 {
		t.Fatalf("主机日志 = %d, %v，期望 2", len(logs), err)
	}
	if logs[0].ErrorMessage != "冒烟失败" || logs[1].Stdout != "ok\n" {
		t.Fatalf("主机日志字段不符：%+v", logs)
	}

	// 目标主机解析：可变长 IN（PG 侧 `= ANY($1::bigint[])`）+ 存在性过滤 + 聚合计数。
	hostIDs := []int64{}
	rows, err := pool.QueryContext(ctx, "SELECT id FROM assets_host ORDER BY id LIMIT 3")
	if err != nil {
		t.Fatalf("读主机 id：%v", err)
	}
	for rows.Next() {
		var hostID int64
		if err = rows.Scan(&hostID); err != nil {
			rows.Close()
			t.Fatalf("扫描主机 id：%v", err)
		}
		hostIDs = append(hostIDs, hostID)
	}
	rows.Close()
	hosts, err := queries.ListAutomationInventoryHosts(ctx, hostIDs)
	if err != nil {
		t.Fatalf("主机快照：%v", err)
	}
	if len(hosts) != len(hostIDs) {
		t.Fatalf("主机快照 = %d 行，期望 %d（只按 id 过滤，ip IS NOT NULL 的行数应一致）", len(hosts), len(hostIDs))
	}
	if _, err = queries.ListAutomationInventoryHosts(ctx, []int64{}); err != nil {
		t.Fatalf("主机快照（空 IN）：%v", err)
	}
	// 库中主机数不影响数组参数编码的验证：P4-7 的故障正是"传入值个数 > 1 时
	// 占位符与参数个数不符"，用几个不存在的 id 走一遍同样能覆盖 D 侧编码。
	synthetic := []int64{2_000_000_001, 2_000_000_002, 2_000_000_003}
	if rows, err := queries.ListAutomationInventoryHosts(ctx, synthetic); err != nil || len(rows) != 0 {
		t.Fatalf("主机快照（多个不存在的 id）：%d 行, %v", len(rows), err)
	}
	if rows, err := queries.ListAutomationHostAgentIdentities(ctx, synthetic); err != nil || len(rows) != 0 {
		t.Fatalf("主机 agent 身份（多个不存在的 id）：%d 行, %v", len(rows), err)
	}
	if counts, err := queries.CountAutomationInventoryHosts(ctx, synthetic); err != nil || counts.Existing != 0 {
		t.Fatalf("主机聚合计数（多个不存在的 id）：%+v, %v", counts, err)
	}
	t.Logf("库中主机数 %d（真库冒烟同样覆盖了数组参数编码）", len(hostIDs))
	identities, err := queries.ListAutomationHostAgentIdentities(ctx, hostIDs)
	if err != nil || len(identities) != len(hostIDs) {
		t.Fatalf("主机 agent 身份 = %d 行, %v，期望 %d", len(identities), err, len(hostIDs))
	}
	counts, err := queries.CountAutomationInventoryHosts(ctx, hostIDs)
	if err != nil {
		t.Fatalf("主机聚合计数：%v", err)
	}
	if int(counts.Existing) != len(hostIDs) || counts.Resolved > counts.Existing {
		t.Fatalf("主机聚合计数不符：%+v（hosts=%d）", counts, len(hostIDs))
	}
	// 零行必须返回全 0（COUNT(CASE WHEN ...) 在 PG 上不会给 NULL）。
	empty, err := queries.CountAutomationInventoryHosts(ctx, []int64{})
	if err != nil || empty.Existing != 0 || empty.Resolved != 0 || empty.GroupCount != 0 {
		t.Fatalf("空集合聚合应为全 0：%+v, %v", empty, err)
	}
	// 主机选项（narg 可选过滤 + 分页 + 计数）。
	if count, err := queries.CountAutomationHostOptions(ctx, db.CountAutomationHostOptionsParams{}); err != nil || count < int64(len(hostIDs)) {
		t.Fatalf("主机选项计数 = %d, %v（库中主机 %d 台）", count, err, len(hostIDs))
	}
	if _, err = queries.ListAutomationHostOptions(ctx, db.ListAutomationHostOptionsParams{
		Pattern: sql.NullString{}, Limit: 10, Offset: 0,
	}); err != nil {
		t.Fatalf("主机选项：%v", err)
	}
	if _, err = queries.ListAutomationHostGroupTree(ctx); err != nil {
		t.Fatalf("主机组树：%v", err)
	}

	// 控制器 SSH 密钥的三条语句（FOR UPDATE 读 → 全删 → 插）在**回滚事务**里验证：
	// loadOrCreateControllerKey 会真的替换掉库里的密钥（Agent 侧已信任公钥），
	// 在共享库上直接调用会把线上自动化链路打断，所以只验证 SQL 本身。
	keyTx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin 密钥事务：%v", err)
	}
	keyQueries := db.New(keyTx)
	controllerKeys, err := keyQueries.ListAutomationControllerKeysForUpdate(ctx)
	if err != nil {
		keyTx.Rollback()
		t.Fatalf("读控制器密钥（FOR UPDATE）：%v", err)
	}
	if err = keyQueries.DeleteAutomationControllerKeys(ctx); err != nil {
		keyTx.Rollback()
		t.Fatalf("删控制器密钥：%v", err)
	}
	if err = keyQueries.CreateAutomationControllerKey(ctx, db.CreateAutomationControllerKeyParams{
		CreateTime: now, UpdateTime: now, PublicKey: "ssh-ed25519 AAAAsmoke", PrivateKey: "go:v1:smoke",
	}); err != nil {
		keyTx.Rollback()
		t.Fatalf("写控制器密钥：%v", err)
	}
	if err = keyTx.Rollback(); err != nil {
		t.Fatalf("回滚密钥事务：%v", err)
	}
	t.Logf("控制器密钥表原有 %d 行（已回滚，未改动）", len(controllerKeys))

	// 作业与主机日志的清理交给 defer（生产代码没有对应删除入口）。
}

// redactSmokeDSN 只保留主机与库名，避免把口令写进测试日志。
func redactSmokeDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return "***" + dsn[at:]
	}
	return dsn
}
