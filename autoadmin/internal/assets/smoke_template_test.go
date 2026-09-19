package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

// TestSmokeTemplateServiceQueriesAgainstRealDatabase 把资产域的部署模板与逻辑服务/部署实例写路径
// 在**真库**上跑一遍：模板（含 5 类嵌套子表 + docker/compose 配置）建→读→删、逻辑服务建→读→删、
// 部署实例建→改→删、以及列表查询的**可选过滤**（sqlc.narg：NULL 表示不过滤）。
//
// 为什么需要它：这组查询里有几处 mock 验不了的东西——
//   - 列表过滤用 `(col = narg(x) OR narg(x) IS NULL)`，PG 在解析期按参数首次出现定类型，
//     写错位置会直接 `could not determine data type of parameter $1`（SQL_DESIGN §2.2），
//     只有真 PG 能验；
//   - 嵌套子表与 docker/compose 配置的 json 列往返；
//   - 部署实例的 `host_id`、`application_id` 等可空/非空列的写入。
//
// 全程一个事务 + 回滚：为了不依赖库里预先有数据，测试自己在事务内建最小链路
// （项目 → 业务系统 → 应用 → 应用版本 → 部署模板 → 主机）。
//
// 用法：
//
//	ASSETS_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/assets/ -run RealDatabase -v
//	ASSETS_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/assets/ -run RealDatabase -v
func TestSmokeTemplateServiceQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("ASSETS_SMOKE_DSN")
	if dsn == "" {
		t.Skip("ASSETS_SMOKE_DSN 未设置：跳过真库冒烟（两个方言各跑一次的说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect(%s): %v", redactSmokeDSN(dsn), err)
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	queries, now := db.New(tx), time.Now().UTC()
	suffix := now.Format("150405.000000")

	// 最小链路：项目 → 业务系统 → 应用 → 版本 → 部署模板；再建一台主机供部署实例引用。
	projectResult, err := queries.CreateProject(ctx, db.CreateProjectParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-project-" + suffix,
		Code: "sp-" + suffix, Owner: "smoke", Enabled: true,
	})
	if err != nil {
		t.Fatalf("建项目：%v", err)
	}
	projectID, err := projectResult.LastInsertId()
	if err != nil {
		t.Fatalf("项目主键：%v", err)
	}
	businessResult, err := queries.CreateBusinessSystem(ctx, db.CreateBusinessSystemParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-bs-" + suffix,
		Code: "sb-" + suffix, Owner: "smoke", Enabled: true, ProjectID: sql.NullInt64{Int64: projectID, Valid: true},
	})
	if err != nil {
		t.Fatalf("建业务系统：%v", err)
	}
	businessID, err := businessResult.LastInsertId()
	if err != nil {
		t.Fatalf("业务系统主键：%v", err)
	}
	applicationResult, err := queries.CreateApplication(ctx, db.CreateApplicationParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-app-" + suffix,
		Category: "general", Code: "sa-" + suffix, Description: "", Enabled: true, Vendor: "",
	})
	if err != nil {
		t.Fatalf("建应用：%v", err)
	}
	applicationID, err := applicationResult.LastInsertId()
	if err != nil {
		t.Fatalf("应用主键：%v", err)
	}
	versionResult, err := queries.CreateApplicationVersion(ctx, db.CreateApplicationVersionParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Version: "v1",
		ReleaseDate: sql.NullTime{}, EndOfSupport: sql.NullTime{}, Enabled: true, ApplicationID: applicationID,
	})
	if err != nil {
		t.Fatalf("建应用版本：%v", err)
	}
	versionID, err := versionResult.LastInsertId()
	if err != nil {
		t.Fatalf("应用版本主键：%v", err)
	}
	hostResult, err := queries.CreateHost(ctx, db.CreateHostParams{
		CreateTime: now, UpdateTime: now, Status: "active", IsDeletedInCloud: false,
		InstanceName:  sql.NullString{String: "smoke-svc-host-" + suffix, Valid: true},
		Ip:            sql.NullString{String: "10.255.255.202", Valid: true},
		CollectStatus: "pending", CollectMessage: "", AgentOnline: false,
		WebsshDefaultUsername: "", WebsshLoginUsers: "",
	})
	if err != nil {
		t.Fatalf("建主机：%v", err)
	}
	hostID, err := hostResult.LastInsertId()
	if err != nil {
		t.Fatalf("主机主键：%v", err)
	}

	// 部署模板 + 全部嵌套子表（原实现是运行时拼表名的 DELETE，现在按表名分派到显式语句）。
	templateID, err := queries.CreateDeploymentTemplate(ctx, db.CreateDeploymentTemplateParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-template-" + suffix,
		ControlType: "systemd", RunUser: "root", RunGroup: "root", AppHome: "/opt/app",
		WorkDirectory: "/opt/app", ServiceName: "app", SystemdScope: "system",
		Enabled: true, ApplicationID: sql.NullInt64{Int64: applicationID, Valid: true},
		MacroDefinitions: json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatalf("建部署模板：%v", err)
	}
	if err = queries.CreateTemplatePort(ctx, db.CreateTemplatePortParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "http", Protocol: "tcp",
		BindAddress: "0.0.0.0", Port: 8080, Required: true, ExternalAccess: false, CheckEnabled: true,
		DeploymentTemplateID: templateID,
	}); err != nil {
		t.Fatalf("建模板端口：%v", err)
	}
	if err = queries.CreateTemplatePath(ctx, db.CreateTemplatePathParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "conf", PathType: "dir",
		Path: "/opt/app/conf", Required: true, CheckEnabled: true, DeploymentTemplateID: templateID,
	}); err != nil {
		t.Fatalf("建模板路径：%v", err)
	}
	if err = queries.CreateTemplateConfigFile(ctx, db.CreateTemplateConfigFileParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "app.yml", Path: "/opt/app/app.yml",
		FileFormat: "yaml", Required: true, DeploymentTemplateID: templateID,
	}); err != nil {
		t.Fatalf("建模板配置文件：%v", err)
	}
	if err = queries.CreateTemplateLogDefinition(ctx, db.CreateTemplateLogDefinitionParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "app.log",
		PathPattern: "/opt/app/logs/*.log", DeploymentTemplateID: templateID,
		ExtraFields: json.RawMessage(`{"tag":"smoke"}`), ProcessingRuleID: sql.NullInt64{},
	}); err != nil {
		t.Fatalf("建模板日志定义：%v", err)
	}
	if err = queries.CreateTemplateControlAction(ctx, db.CreateTemplateControlActionParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Action: "start", Command: "systemctl start app",
		TimeoutSeconds: 30, SuccessExitCodes: json.RawMessage(`[0]`), DeploymentTemplateID: templateID,
	}); err != nil {
		t.Fatalf("建模板控制动作：%v", err)
	}
	if err = queries.CreateTemplateDockerConfig(ctx, db.CreateTemplateDockerConfigParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, ContainerName: "app",
		DockerHost: "unix:///var/run/docker.sock", ExpectedImage: "app", ExpectedImageTag: "v1",
		DeploymentTemplateID: templateID,
	}); err != nil {
		t.Fatalf("建 docker 配置：%v", err)
	}
	if err = queries.CreateTemplateComposeConfig(ctx, db.CreateTemplateComposeConfigParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, ProjectName: "app", ServiceName: "web",
		ComposeFilePath: "/opt/app/docker-compose.yml", WorkingDirectory: "/opt/app", EnvFile: ".env",
		ExpectedImage: "app", ExpectedImageTag: "v1", DeploymentTemplateID: templateID,
	}); err != nil {
		t.Fatalf("建 compose 配置：%v", err)
	}

	// 嵌套读取：子表按 protocol,port / path_type,id 排序，docker/compose 各取一条。
	template, err := (&Repository{pool: pool, queries: queries}).loadTemplateNested(ctx, templateID)
	if err != nil {
		t.Fatalf("读模板嵌套：%v", err)
	}
	if len(template.Ports) != 1 || len(template.Paths) != 1 || len(template.ConfigFiles) != 1 ||
		len(template.Logs) != 1 || len(template.ControlActions) != 1 {
		t.Fatalf("嵌套子表数量不符：ports=%d paths=%d files=%d logs=%d actions=%d",
			len(template.Ports), len(template.Paths), len(template.ConfigFiles), len(template.Logs), len(template.ControlActions))
	}
	if template.DockerConfig == nil || template.ComposeConfig == nil {
		t.Fatalf("docker/compose 配置缺失：%+v / %+v", template.DockerConfig, template.ComposeConfig)
	}
	// json 列由库归一化（MySQL 会加空格），比对解析后的值而不是文本。
	var extraFields map[string]any
	if json.Unmarshal(template.Logs[0].ExtraFields, &extraFields) != nil || extraFields["tag"] != "smoke" {
		t.Fatalf("模板日志的 extra_fields 往返有误：%s", template.Logs[0].ExtraFields)
	}
	if template.Ports[0].Port != 8080 || template.Ports[0].Protocol != "tcp" {
		t.Fatalf("嵌套子表字段不符：%+v", template.Ports[0])
	}

	// 逻辑服务 + 成员部署 + 日志设置。
	serviceID, err := queries.CreateApplicationService(ctx, db.CreateApplicationServiceParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, Name: "smoke-service-" + suffix,
		Code: "ss-" + suffix, TopologyType: "standalone", AccessAddress: "127.0.0.1", Enabled: true,
		ApplicationID: applicationID, ApplicationVersionID: versionID, DeploymentTemplateID: templateID,
		BusinessSystemID: businessID, MacroValues: json.RawMessage(`{"ORACLE_SID":"smoke"}`),
		LogCollectionEnabled: true,
	})
	if err != nil {
		t.Fatalf("建逻辑服务：%v", err)
	}
	deploymentID, err := queries.CreateApplicationDeployment(ctx, db.CreateApplicationDeploymentParams{
		CreateTime: now, UpdateTime: now, Remark: sql.NullString{}, InstanceName: "smoke-inst-" + suffix,
		Enabled: true, HostID: hostID, RuntimeStatus: "unknown", RuntimeStatusOutput: "",
		HaRole: "unknown", RuntimeVariables: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("建部署实例：%v", err)
	}
	if err = queries.CreateServiceDeployment(ctx, db.CreateServiceDeploymentParams{
		CreateTime: now, UpdateTime: now, Enabled: true, DeploymentID: deploymentID, ServiceID: serviceID,
	}); err != nil {
		t.Fatalf("建服务成员：%v", err)
	}
	logDefinitions, err := queries.ListTemplateLogDefinitions(ctx, templateID)
	if err != nil || len(logDefinitions) != 1 {
		t.Fatalf("读模板日志定义：%d 行, %v", len(logDefinitions), err)
	}
	if err = queries.CreateServiceLogSetting(ctx, db.CreateServiceLogSettingParams{
		CreateTime: now, UpdateTime: now, CollectionEnabled: boolPtr(true),
		LogDefinitionID: logDefinitions[0].ID, RetentionTierID: sql.NullInt64{}, ServiceID: serviceID,
		CollectionFilterRuleID: sql.NullInt64{},
	}); err != nil {
		t.Fatalf("建服务日志设置：%v", err)
	}

	// 服务详情（含成员实例）与日志设置/模板日志读取。
	detail, err := (&Repository{pool: pool, queries: queries}).GetApplicationService(ctx, serviceID)
	if err != nil {
		t.Fatalf("读服务详情：%v", err)
	}
	if detail.Application != applicationID || detail.BusinessSystem != businessID ||
		len(detail.MemberInstances) != 1 || detail.MemberInstances[0] != deploymentID {
		t.Fatalf("服务详情不符：%+v", detail)
	}
	if detail.Environment != nil || detail.ClusterProfile != nil || detail.LogRetentionTier != nil {
		t.Fatalf("服务详情的可空列应为 nil：%+v", detail)
	}
	logSettings, err := (&Repository{pool: pool, queries: queries}).ListServiceLogSettings(ctx, serviceID)
	if err != nil || len(logSettings) != 1 || logSettings[0].CollectionEnabled == nil || !*logSettings[0].CollectionEnabled {
		t.Fatalf("服务日志设置 = %d 行, %v", len(logSettings), err)
	}
	templateLogs, err := (&Repository{pool: pool, queries: queries}).ListServiceTemplateLogs(ctx, serviceID)
	if err != nil {
		t.Fatalf("读模板日志表：%v", err)
	}
	if len(templateLogs) != 1 || templateLogs[0].DataStream == "" || templateLogs[0].ResolvedPath == "" {
		t.Fatalf("模板日志行不符：%+v", templateLogs)
	}

	// 列表查询的可选过滤：命中（按服务/业务系统/环境过滤）与不过滤（全 NULL）两条路径都要能跑。
	matches, err := queries.CountApplicationDeployments(ctx, db.CountApplicationDeploymentsParams{
		ServiceID: sql.NullInt64{Int64: serviceID, Valid: true},
	})
	if err != nil || matches != 1 {
		t.Fatalf("按服务过滤部署实例 = %d, %v，期望 1", matches, err)
	}
	if matches, err = queries.CountApplicationDeployments(ctx, db.CountApplicationDeploymentsParams{
		BusinessSystemID: sql.NullInt64{Int64: businessID, Valid: true},
	}); err != nil || matches != 1 {
		t.Fatalf("按业务系统过滤部署实例 = %d, %v，期望 1", matches, err)
	}
	// 环境过滤：服务没有环境（NULL）→ 传一个不存在的环境 id 应命中 0 行。
	if matches, err = queries.CountApplicationDeployments(ctx, db.CountApplicationDeploymentsParams{
		EnvironmentID: sql.NullInt64{Int64: 2_000_000_000, Valid: true},
	}); err != nil || matches != 0 {
		t.Fatalf("按不存在环境过滤 = %d, %v，期望 0", matches, err)
	}
	if matches, err = queries.CountApplicationDeployments(ctx, db.CountApplicationDeploymentsParams{
		ServiceID:     sql.NullInt64{Int64: serviceID, Valid: true},
		EnvironmentID: sql.NullInt64{Int64: 2_000_000_000, Valid: true},
	}); err != nil || matches != 0 {
		t.Fatalf("服务命中但环境不命中 = %d, %v，期望 0", matches, err)
	}
	filtered, _, err := (&Repository{pool: pool, queries: queries}).ListApplicationDeployments(ctx,
		pagination.Page{Number: 1, Size: 10}, ApplicationDeploymentFilter{ApplicationServiceID: serviceID})
	if err != nil || len(filtered) != 1 || len(filtered[0].ApplicationServiceIDs) != 1 {
		t.Fatalf("部署实例列表（按服务过滤）= %d 行, %v", len(filtered), err)
	}
	services, _, err := (&Repository{pool: pool, queries: queries}).ListApplicationServices(ctx, "", pagination.Page{Number: 1, Size: 10}, businessID)
	if err != nil {
		t.Fatalf("逻辑服务列表：%v", err)
	}
	found := false
	for _, item := range services {
		if item.ID == serviceID {
			found = true
		}
	}
	if !found {
		t.Fatalf("按业务系统过滤的服务列表里找不到刚建的服务")
	}

	// 模板保存的日志定义写路径（按 id 增量，不再整表删重建）：
	//   ① 改路径 → 定义 id 不变，引用它的服务级覆盖行必须还在（旧实现会换 id → 覆盖值失效，
	//      而且外键会让这次保存直接失败）；
	//   ② 移除该定义 → 级联清掉覆盖行，不撞外键。
	// 直接调 applyTemplateLogWrites（与 SaveDeploymentTemplate 同一段逻辑），因为它接受
	// 事务内的 queries；Repository.SaveDeploymentTemplate 会另开自己的事务，跑在冒烟事务之外。
	if err = applyTemplateLogWrites(ctx, queries, templateID, false, []TemplateLogInput{{
		ID: logDefinitions[0].ID, Name: logDefinitions[0].Name,
		PathPattern: "/opt/app/logs/*.log", ExtraFields: json.RawMessage(`{"tag":"smoke"}`),
	}}, now); err != nil {
		t.Fatalf("按 id 更新日志定义：%v", err)
	}
	afterUpdate, err := queries.ListTemplateLogDefinitions(ctx, templateID)
	if err != nil || len(afterUpdate) != 1 {
		t.Fatalf("更新后应仍有 1 条日志定义：%d 行, %v", len(afterUpdate), err)
	}
	if afterUpdate[0].ID != logDefinitions[0].ID || afterUpdate[0].PathPattern != "/opt/app/logs/*.log" {
		t.Fatalf("日志定义应原地更新（id 不变）：%+v", afterUpdate[0])
	}
	if logSettings, err = (&Repository{pool: pool, queries: queries}).ListServiceLogSettings(ctx, serviceID); err != nil || len(logSettings) != 1 {
		t.Fatalf("改路径后服务级覆盖行应保留：%d 行, %v", len(logSettings), err)
	}
	// 空提交 = 该模板不再有日志定义 → 删行并级联清覆盖行。
	if err = applyTemplateLogWrites(ctx, queries, templateID, false, nil, now); err != nil {
		t.Fatalf("删除日志定义：%v", err)
	}
	if rows, err := queries.ListTemplateLogDefinitions(ctx, templateID); err != nil || len(rows) != 0 {
		t.Fatalf("日志定义应已删除：%d 行, %v", len(rows), err)
	}
	if logSettings, err = (&Repository{pool: pool, queries: queries}).ListServiceLogSettings(ctx, serviceID); err != nil || len(logSettings) != 0 {
		t.Fatalf("覆盖行应被级联清理：%d 行, %v", len(logSettings), err)
	}

	// 部署实例更新与删除、服务删除、模板嵌套删除。
	if err = queries.UpdateApplicationDeployment(ctx, db.UpdateApplicationDeploymentParams{
		UpdateTime: time.Now().UTC(), Remark: sql.NullString{String: "smoke", Valid: true},
		InstanceName: "smoke-inst2-" + suffix, Enabled: false, HostID: hostID, RuntimeStatus: "running",
		RuntimeStatusOutput: "ok", HaRole: "master", RuntimeVariables: json.RawMessage(`{"a":1}`), ID: deploymentID,
	}); err != nil {
		t.Fatalf("改部署实例：%v", err)
	}
	if err = queries.DeleteServiceDeployments(ctx, serviceID); err != nil {
		t.Fatalf("删服务成员：%v", err)
	}
	if err = queries.DeleteServiceLogSettings(ctx, serviceID); err != nil {
		t.Fatalf("删服务日志设置：%v", err)
	}
	if err = queries.DeleteApplicationService(ctx, serviceID); err != nil {
		t.Fatalf("删逻辑服务：%v", err)
	}
	if err = queries.DeleteApplicationDeployment(ctx, deploymentID); err != nil {
		t.Fatalf("删部署实例：%v", err)
	}
	for name, remove := range map[string]func() error{
		"ports":   func() error { return queries.DeleteTemplatePorts(ctx, templateID) },
		"paths":   func() error { return queries.DeleteTemplatePaths(ctx, templateID) },
		"files":   func() error { return queries.DeleteTemplateConfigFiles(ctx, templateID) },
		"logs":    func() error { return queries.DeleteTemplateLogDefinitions(ctx, templateID) },
		"actions": func() error { return queries.DeleteTemplateControlActions(ctx, templateID) },
		"docker":  func() error { return queries.DeleteTemplateDockerConfig(ctx, templateID) },
		"compose": func() error { return queries.DeleteTemplateComposeConfig(ctx, templateID) },
	} {
		if err = remove(); err != nil {
			t.Fatalf("删模板子表 %s：%v", name, err)
		}
	}
	if err = queries.DeleteDeploymentTemplate(ctx, templateID); err != nil {
		t.Fatalf("删部署模板：%v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("回滚：%v", err)
	}
}

func boolPtr(value bool) *bool { return &value }
