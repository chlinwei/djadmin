package router

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/middleware"
	"autoadmin/internal/api/response"
	"autoadmin/internal/assets"
	"autoadmin/internal/audit"
	"autoadmin/internal/automation"
	"autoadmin/internal/baseline"
	"autoadmin/internal/identity"
	"autoadmin/internal/inspection"
	"autoadmin/internal/job"
	"autoadmin/internal/logcollect"
	"autoadmin/internal/messaging/rabbitmq"
	"autoadmin/internal/monitor"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/rbac"
	"autoadmin/internal/scheduler"
	"autoadmin/internal/sysconfig"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// logBatchPublisher 是日志采集批量作业需要的投递能力：只用到"向采集队列投递一条消息"。
// 用窄接口而不是具体客户端类型，装配方给不出投递能力时（如未接队列的进程）批量执行器
// 依然能构造，只是批量接口会明确报错。
type logBatchPublisher interface {
	PublishLogCollect(ctx context.Context, message job.Message) error
}

// LogBatchOptions 是日志采集批量动作的执行规模，由角色装配方（app）从配置注入。
// 放在这里而不是直接读环境变量：并发上限属于部署参数，配置集中在 internal/config。
type LogBatchOptions struct {
	InstallConcurrency int
	ApplyConcurrency   int
	Prefetch           int
	Budget             time.Duration
}

// New 只返回 HTTP 引擎（历史签名，供不消费队列的调用方使用）。
func New(database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string) (*gin.Engine, error) {
	engine, _, err := NewWithGateway(database, tokens, allowedOrigins, schedulerPublisher, credentialEncryptionKey, djangoSecret, nil, LogBatchOptions{})
	return engine, err
}

// NewWithGateway 装配 HTTP 路由，并返回本进程需要消费的队列消费者。
//
// 第二个返回值是「日志采集批量作业」的消费者：这类作业要通过 agent gRPC 会话执行，
// 会话只存在于跑 api 角色的进程里，所以必须由 api 角色消费（见 rabbitmq.LogCollectRoute）。
// batchOptions 的零值字段用 logcollect 的缺省规模（0 视为未配置）。
func NewWithGateway(database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string, gateway *agent.Gateway, batchOptions LogBatchOptions) (*gin.Engine, rabbitmq.Consumer, error) {
	engine := gin.New()
	engine.Use(cors.New(cors.Config{
		AllowOrigins:  allowedOrigins,
		AllowMethods:  []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:  []string{"Authorization", "Content-Type", "Accept", "Range"},
		ExposeHeaders: []string{"Content-Disposition", "Content-Length", "Content-Range", "Accept-Ranges"},
		MaxAge:        12 * time.Hour,
	}), gin.LoggerWithConfig(gin.LoggerConfig{SkipQueryString: true}))
	auditRepository := audit.NewRepository(database)
	// Audit wraps Recovery so authenticated panics are persisted as completed 500 operations.
	engine.Use(audit.Capture(auditRepository), gin.Recovery())

	engine.GET("/health/live", func(context *gin.Context) {
		response.Success(context, gin.H{"status": "ok"})
	})
	engine.GET("/health/ready", readiness(database))

	// 静态文件（用户头像等）：与 identity/assets/monitor 的 media 目录一致（默认 autoadmin/media）。
	// 匿名可访问，路径穿越由 http.Dir 拦截；audit 中间件已跳过 /media 前缀。
	mediaRoot, err := filepath.Abs("media")
	if err != nil {
		return nil, nil, err
	}
	engine.Static("/media", mediaRoot)

	identityRepository := identity.NewRepository(database)
	identityHandler := identity.NewHandler(identity.NewService(identityRepository, tokens))
	apiTokenHandler := identity.NewAPITokenHandler(db.New(database))
	engine.POST("/sys/login", identityHandler.Login)
	engine.GET("/sys/users/current/", middleware.Authenticate(tokens), identityHandler.Current)
	engine.GET(
		"/sys/users/",
		middleware.Authenticate(tokens),
		middleware.RequirePermission("system:users:view"),
		identityHandler.ListUsers,
	)
	engine.POST("/user/changeAvatar", middleware.Authenticate(tokens), identityHandler.ChangeAvatar)
	apiTokens := engine.Group("/sys/usercenter", middleware.Authenticate(tokens))
	apiTokens.POST("/updateUserInfo/", middleware.RequirePermission("system:usercenter:updateUserInfo"), identityHandler.UpdateUserInfo)
	apiTokens.POST("/updateUserPassword/", identityHandler.UpdateUserPassword)
	apiTokens.GET("/alertMediaBindings/", identityHandler.AlertMediaBindings)
	apiTokens.POST("/updateAlertMediaBindings/", identityHandler.UpdateAlertMediaBindings)
	apiTokens.GET("/apiTokens/", middleware.RequirePermission("system:api_token:view"), apiTokenHandler.List)
	apiTokens.POST("/createApiToken/", middleware.RequirePermission("system:api_token:create"), apiTokenHandler.Create)
	apiTokens.POST("/rotateApiToken/", middleware.RequirePermission("system:api_token:rotate"), apiTokenHandler.Rotate)
	apiTokens.POST("/disableApiToken/", middleware.RequirePermission("system:api_token:disable"), apiTokenHandler.Disable)
	apiTokens.POST("/deleteApiToken/", middleware.RequirePermission("system:api_token:delete"), apiTokenHandler.Delete)
	playbookHandler := automation.NewHandler(database, gateway)
	playbooks := engine.Group("/sys/automation/playbooks", middleware.Authenticate(tokens))
	playbooks.GET("/", middleware.RequirePermission("automation:playbooks:view"), playbookHandler.List)
	playbooks.POST("/validate/", middleware.RequirePermission("automation:playbooks:view"), playbookHandler.Validate)
	playbooks.GET("/host-options/", middleware.RequirePermission("automation:jobs:create"), playbookHandler.HostOptions)
	playbooks.GET("/group-tree/", middleware.RequirePermission("automation:jobs:create"), playbookHandler.GroupTree)
	playbooks.GET("/:id/", middleware.RequirePermission("automation:playbooks:view"), playbookHandler.Get)
	playbooks.POST("/", middleware.RequirePermission("automation:playbooks:create"), playbookHandler.Create)
	playbooks.PATCH("/:id/", middleware.RequirePermission("automation:playbooks:update"), playbookHandler.Update)
	playbooks.PUT("/:id/", middleware.RequirePermission("automation:playbooks:update"), playbookHandler.Update)
	playbooks.POST("/:id/upload/", middleware.RequirePermission("automation:playbooks:update"), playbookHandler.UploadFile)
	playbooks.GET("/:id/download/", middleware.RequirePermission("automation:playbooks:view"), playbookHandler.DownloadFile)
	playbooks.POST("/batch-delete/", middleware.RequirePermission("automation:playbooks:delete"), playbookHandler.BatchDeletePlaybooks)
	automationInventories := engine.Group("/sys/automation/inventories", middleware.Authenticate(tokens))
	automationInventories.GET("/", middleware.RequirePermission("automation:inventory:view"), playbookHandler.ListInventories)
	automationInventories.POST("/", middleware.RequirePermission("automation:inventory:create"), playbookHandler.CreateInventory)
	automationInventories.GET("/:id/", middleware.RequirePermission("automation:inventory:view"), playbookHandler.GetInventory)
	automationInventories.PATCH("/:id/", middleware.RequirePermission("automation:inventory:update"), playbookHandler.UpdateInventory)
	automationInventories.PUT("/:id/", middleware.RequirePermission("automation:inventory:update"), playbookHandler.UpdateInventory)
	automationInventories.POST("/batch-delete/", middleware.RequirePermission("automation:inventory:delete"), playbookHandler.BatchDeleteInventories)
	automationInventories.POST("/:id/precheck-limit/", middleware.RequirePermission("automation:inventory:view"), playbookHandler.PrecheckInventoryLimit)
	automationTasks := engine.Group("/sys/automation/tasks", middleware.Authenticate(tokens))
	automationTasks.GET("/", middleware.RequirePermission("automation:tasks:view"), playbookHandler.ListTasks)
	automationTasks.POST("/", middleware.RequirePermission("automation:tasks:create"), playbookHandler.CreateTask)
	automationTasks.GET("/:id/", middleware.RequirePermission("automation:tasks:view"), playbookHandler.GetTask)
	automationTasks.PATCH("/:id/", middleware.RequirePermission("automation:tasks:update"), playbookHandler.UpdateTask)
	automationTasks.PUT("/:id/", middleware.RequirePermission("automation:tasks:update"), playbookHandler.UpdateTask)
	automationTasks.POST("/batch-delete/", middleware.RequirePermission("automation:tasks:delete"), playbookHandler.BatchDeleteTasks)
	automationTasks.POST("/:id/precheck/", middleware.RequirePermission("automation:jobs:create"), playbookHandler.PrecheckTask)
	automationTasks.POST("/:id/run_now/", middleware.RequirePermission("automation:jobs:create"), playbookHandler.RunTaskNow)
	automationJobs := engine.Group("/sys/automation/jobs", middleware.Authenticate(tokens))
	automationJobs.GET("/", middleware.RequirePermission("automation:jobs:view"), playbookHandler.ListJobs)
	automationJobs.GET("/:id/", middleware.RequirePermission("automation:jobs:view"), playbookHandler.GetJob)
	automationJobs.GET("/:id/log/", middleware.RequirePermission("automation:jobs:view"), playbookHandler.JobLog)
	automationJobs.GET("/:id/events/", middleware.RequirePermission("automation:jobs:view"), playbookHandler.JobEvents)
	automationJobs.GET("/:id/status-summary/", middleware.RequirePermission("automation:jobs:view"), playbookHandler.JobStatusSummary)
	automationJobs.POST("/:id/cancel/", middleware.RequirePermission("automation:jobs:create"), playbookHandler.CancelJob)
	schedulerHandler := scheduler.NewHandler(scheduler.NewService(scheduler.NewRepository(database)).WithPublisher(schedulerPublisher))
	schedulerTasks := engine.Group("/sys/scheduler/tasks", middleware.Authenticate(tokens), middleware.RequirePermission("system:scheduler:view"))
	schedulerTasks.GET("/", schedulerHandler.List)
	schedulerTasks.GET("/:id/", schedulerHandler.Get)
	schedulerTasks.PATCH("/:id/", schedulerHandler.Update)
	schedulerTasks.POST("/:id/toggle_enabled/", schedulerHandler.Toggle)
	schedulerTasks.POST("/:id/enable/", schedulerHandler.Enable)
	schedulerTasks.POST("/:id/disable/", schedulerHandler.Disable)
	schedulerTasks.POST("/:id/run_now/", schedulerHandler.RunNow)
	schedulerTasks.GET("/:id/status/", schedulerHandler.Status)
	schedulerTasks.POST("/start_scheduler/", schedulerHandler.StartScheduler)
	schedulerTasks.POST("/stop_scheduler/", schedulerHandler.StopScheduler)
	schedulerLogs := engine.Group("/sys/scheduler/task-logs", middleware.Authenticate(tokens), middleware.RequirePermission("system:scheduler:view"))
	schedulerLogs.GET("/", schedulerHandler.ListLogs)
	schedulerLogs.GET("/:id/", schedulerHandler.GetLog)
	users := engine.Group("/sys/users", middleware.Authenticate(tokens))
	users.GET("/:id/", middleware.RequirePermission("system:users:view"), identityHandler.UserDetail)
	users.POST("/", middleware.RequirePermission("system:users:create"), identityHandler.CreateUser)
	users.PATCH("/:id/", middleware.RequirePermission("system:users:update"), identityHandler.UpdateUser)
	users.GET("/checkUserName/", middleware.RequirePermission("system:users:view"), identityHandler.CheckUsername)
	users.POST("/batch-delete/", middleware.RequirePermission("system:users:delete"), identityHandler.BatchDeleteUsers)
	users.POST("/resetUserPwd/", middleware.RequirePermission("system:users:update"), identityHandler.ResetPassword)
	users.POST("/assginUserRoles/", middleware.RequirePermission("system:users:update"), identityHandler.AssignRoles)
	users.POST("/changeUserStatus/", middleware.RequirePermission("system:users:update"), identityHandler.ChangeUserStatus)
	users.GET("/getUserRolesById/", middleware.RequirePermission("system:users:view"), identityHandler.GetUserRoles)

	userGroups := engine.Group("/sys/user-groups", middleware.Authenticate(tokens))
	userGroups.GET("/", middleware.RequirePermission("system:usergroups:view"), identityHandler.ListUserGroups)
	userGroups.POST("/create/", middleware.RequirePermission("system:usergroups:create"), identityHandler.CreateUserGroup)
	userGroups.POST("/update/", middleware.RequirePermission("system:usergroups:update"), identityHandler.UpdateUserGroup)
	userGroups.POST("/batch-delete/", middleware.RequirePermission("system:usergroups:delete"), identityHandler.BatchDeleteUserGroups)

	rbacRepository := rbac.NewRepository(database)
	rbacHandler := rbac.NewHandler(rbac.NewService(rbacRepository))
	roles := engine.Group("/sys/roles", middleware.Authenticate(tokens))
	roles.GET("/", middleware.RequirePermission("system:roles:view"), rbacHandler.List)
	roles.GET("/:id/", middleware.RequirePermission("system:roles:view"), rbacHandler.Get)
	roles.POST("/", middleware.RequirePermission("system:roles:create"), rbacHandler.Create)
	roles.PATCH("/:id/", middleware.RequirePermission("system:roles:update"), rbacHandler.Update)
	roles.POST("/batch-delete/", middleware.RequirePermission("system:roles:delete"), rbacHandler.DeleteMany)
	roles.GET("/getCurrentUserRoleList/", rbacHandler.Current)

	menus := engine.Group("/sys/menus", middleware.Authenticate(tokens))
	menus.GET("/getMenuTree/", middleware.RequirePermission("system:menu:list"), rbacHandler.MenuTree)
	menus.GET("/:id/", middleware.RequirePermission("system:menu:list"), rbacHandler.GetMenu)
	menus.POST("/", middleware.RequirePermission("system:menus:create"), rbacHandler.CreateMenu)
	menus.PATCH("/:id/", middleware.RequirePermission("system:menus:update"), rbacHandler.UpdateMenu)
	menus.GET("/getMenuListByRoleId/", middleware.RequirePermission("system:roles:view"), rbacHandler.MenuIDsByRole)
	menus.POST("/grantMenu/", middleware.RequirePermission("system:roles:update"), rbacHandler.GrantMenus)
	menus.DELETE("/deleteMenuById/", middleware.RequirePermission("system:menus:delete"), rbacHandler.DeleteMenu)

	configRepository := sysconfig.NewRepository(database)
	configHandler := sysconfig.NewHandler(sysconfig.NewService(configRepository))
	configs := engine.Group("/sys/configs", middleware.Authenticate(tokens))
	configs.GET("/", configHandler.List)
	configs.GET("/:id/", configHandler.Get)
	configs.PATCH("/:id/", configHandler.Update)
	configs.POST("/:id/reset-default/", configHandler.Reset)
	configs.GET("/by-key/:key/", configHandler.GetByKey)
	configs.PATCH("/update-by-key/:key/", configHandler.UpdateByKey)

	auditHandler := audit.NewHandler(auditRepository)
	engine.GET(
		"/sys/audit/operation-logs/",
		middleware.Authenticate(tokens),
		middleware.RequirePermission("audit:operation_logs:view"),
		auditHandler.List,
	)
	engine.GET("/sys/audit/login-logs/", middleware.Authenticate(tokens), middleware.RequirePermission("audit:login_logs:view"), auditHandler.ListLoginLogs)
	websshAudit := engine.Group("/sys/audit/webssh-sessions", middleware.Authenticate(tokens), middleware.RequirePermission("audit:webssh_sessions:view"))
	websshAudit.GET("/", auditHandler.ListWebSSHSessions)
	websshAudit.GET("/:id/content/", auditHandler.WebSSHContent)
	websshAudit.GET("/:id/download/", auditHandler.DownloadWebSSH)
	websshAudit.GET("/download-all/", auditHandler.DownloadWebSSHMany)

	assetsService, err := assets.NewService(assets.NewRepository(database), credentialEncryptionKey, djangoSecret)
	if err != nil {
		return nil, nil, err
	}
	assetsHandler := assets.NewHandler(assetsService, gateway, "")
	assets.SetDeploymentGateway(gateway)
	// 与 /api/agent/install 相同的中间件链：安装包管理直接影响主机上的 Agent 二进制，权限口径一致。
	agentPackages := engine.Group("/api/agent/packages", middleware.Authenticate(tokens), middleware.RequirePermission("assets:hosts:update"))
	agentPackages.GET("/", assetsHandler.ListAgentPackages)
	agentPackages.GET("/download/", assetsHandler.DownloadAgentPackage)
	agentPackages.POST("/upload/", assetsHandler.UploadAgentPackage)
	agentPackages.POST("/:id/activate/", assetsHandler.ActivateAgentPackage)
	agentPackages.POST("/batch-delete/", assetsHandler.BatchDeleteAgentPackages)
	engine.POST("/api/agent/install", middleware.Authenticate(tokens), middleware.RequirePermission("assets:hosts:update"), assetsHandler.AgentInstall)
	projects := engine.Group("/assets/projects", middleware.Authenticate(tokens))
	projects.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListProjects)
	projects.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetProject)
	projects.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateProject)
	projects.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProject)
	projects.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProject)
	projects.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteProjects)

	businessSystems := engine.Group("/assets/business-systems", middleware.Authenticate(tokens))
	businessSystems.GET("/", middleware.RequirePermission("assets:service-tree:view"), assetsHandler.ListBusinessSystems)
	businessSystems.GET("/:id/", middleware.RequirePermission("assets:service-tree:view"), assetsHandler.GetBusinessSystem)
	businessSystems.POST("/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.CreateBusinessSystem)
	businessSystems.PUT("/:id/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.UpdateBusinessSystem)
	businessSystems.PATCH("/:id/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.UpdateBusinessSystem)
	businessSystems.POST("/batch-delete/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.BatchDeleteBusinessSystems)

	environments := engine.Group("/assets/business-environments", middleware.Authenticate(tokens))
	environments.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListEnvironments)
	environments.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetEnvironment)
	environments.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateEnvironment)
	environments.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateEnvironment)
	environments.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateEnvironment)
	environments.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteEnvironments)

	credentials := engine.Group("/assets/credentials", middleware.Authenticate(tokens))
	credentials.GET("/", middleware.RequirePermission("assets:credentials:view"), assetsHandler.ListCredentials)
	credentials.POST("/batch-create/", middleware.RequirePermission("assets:credentials:create"), assetsHandler.BatchCreateCredentials)
	credentials.POST("/batch-delete/", middleware.RequirePermission("assets:credentials:delete"), assetsHandler.DeleteCredentials)
	credentials.GET("/:id/", middleware.RequirePermission("assets:credentials:view"), assetsHandler.GetCredential)
	credentials.POST("/", middleware.RequirePermission("assets:credentials:create"), assetsHandler.CreateCredential)
	credentials.PUT("/:id/", middleware.RequirePermission("assets:credentials:update"), assetsHandler.UpdateCredential)
	credentials.PATCH("/:id/", middleware.RequirePermission("assets:credentials:update"), assetsHandler.UpdateCredential)

	hostGroups := engine.Group("/assets/host-groups", middleware.Authenticate(tokens))
	hostGroups.GET("/", middleware.RequirePermission("assets:hostgroups:view"), assetsHandler.ListHostGroups)
	hostGroups.GET("/tree/", middleware.RequirePermission("assets:hostgroups:view"), assetsHandler.HostGroupTree)
	hostGroups.GET("/:id/", middleware.RequirePermission("assets:hostgroups:view"), assetsHandler.GetHostGroup)
	hostGroups.POST("/", middleware.RequirePermission("assets:hostgroups:create"), assetsHandler.CreateHostGroup)
	hostGroups.PUT("/:id/", middleware.RequirePermission("assets:hostgroups:update"), assetsHandler.UpdateHostGroup)
	hostGroups.PATCH("/:id/", middleware.RequirePermission("assets:hostgroups:update"), assetsHandler.UpdateHostGroup)
	hostGroups.POST("/batch-delete/", middleware.RequirePermission("assets:hostgroups:delete"), assetsHandler.BatchDeleteHostGroups)

	hosts := engine.Group("/assets/hosts", middleware.Authenticate(tokens))
	hosts.GET("/:id/webssh/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.WebSSH)
	hosts.GET("/:id/webssh-active-count/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.GetHostWebSSHActiveCount)
	hosts.GET("/:id/webssh-active-sessions/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.GetHostWebSSHActiveSessions)
	hosts.GET("/:id/files/list/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.ListWebSSHFiles)
	hosts.GET("/:id/files/download/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.DownloadWebSSHFile)
	hosts.POST("/:id/files/upload/chunk/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.UploadWebSSHFile)
	hosts.POST("/:id/files/rename/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.RenameWebSSHFile)
	hosts.DELETE("/:id/files/delete/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.DeleteWebSSHFile)
	hosts.POST("/:id/files/create-dir/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.CreateWebSSHDirectory)
	hosts.POST("/:id/refresh-info/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.RefreshHostInfo)
	hosts.GET("/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.ListHosts)
	hosts.POST("/refresh-info/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.BatchRefreshHostInfo)
	hosts.POST("/batch-delete/", middleware.RequirePermission("assets:hosts:delete"), assetsHandler.DeleteHosts)
	hosts.GET("/:id/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.GetHost)
	hosts.POST("/", middleware.RequirePermission("assets:hosts:create"), assetsHandler.CreateHost)
	hosts.PUT("/:id/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.UpdateHost)
	hosts.PATCH("/:id/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.UpdateHost)

	engine.GET("/ws/assets/hosts/:id/webssh/", middleware.Authenticate(tokens), middleware.RequirePermission("assets:hosts:view"), assetsHandler.WebSSH)
	engine.GET("/ws/automation/jobs/:id/logs/", middleware.Authenticate(tokens), middleware.RequirePermission("automation:jobs:view"), playbookHandler.JobLogStream)

	applications := engine.Group("/assets/applications", middleware.Authenticate(tokens))
	applications.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListApplications)
	applications.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplication)
	applications.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateApplication)
	applications.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplication)
	applications.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplication)
	applications.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteApplications)
	versions := engine.Group("/assets/application-versions", middleware.Authenticate(tokens))
	versions.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListVersions)
	versions.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetVersion)
	versions.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateVersion)
	versions.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateVersion)
	versions.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateVersion)
	versions.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteVersions)
	profiles := engine.Group("/assets/cluster-profiles", middleware.Authenticate(tokens))
	profiles.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListProfiles)
	profiles.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetProfile)
	profiles.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateProfile)
	profiles.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProfile)
	profiles.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProfile)
	profiles.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteProfiles)

	baselineHandler := baseline.NewHandler(database, gateway)
	baselineGroup := engine.Group("/sys/security/baseline", middleware.Authenticate(tokens))
	baselineGroup.GET("/", middleware.RequirePermission("baseline:manage"), baselineHandler.ListBaselines)
	baselineGroup.GET("/:id/", middleware.RequirePermission("baseline:manage"), baselineHandler.GetBaseline)
	baselineGroup.POST("/", middleware.RequirePermission("baseline:manage"), baselineHandler.SaveBaseline)
	baselineGroup.PATCH("/:id/", middleware.RequirePermission("baseline:manage"), baselineHandler.SaveBaseline)
	baselineGroup.DELETE("/:id/", middleware.RequirePermission("baseline:manage"), baselineHandler.DeleteBaseline)
	baselineGroup.POST("/:id/scan/", middleware.RequirePermission("baseline:scan"), baselineHandler.StartScan)
	baselineGroup.POST("/:id/categories/", middleware.RequirePermission("baseline:manage"), baselineHandler.CreateBaselineCategory)
	// 类目子资源挂在基线下（避免与 /:id/ 通配符同级冲突）。
	baselineGroup.PATCH("/:id/categories/:categoryId/", middleware.RequirePermission("baseline:manage"), baselineHandler.UpdateBaselineCategory)
	baselineGroup.DELETE("/:id/categories/:categoryId/", middleware.RequirePermission("baseline:manage"), baselineHandler.DeleteBaselineCategory)
	// 策略单条 CRUD：弹窗确认即落库。
	baselineGroup.POST("/:id/items/", middleware.RequirePermission("baseline:manage"), baselineHandler.AddBaselineItem)
	baselineGroup.PATCH("/:id/items/:itemId/", middleware.RequirePermission("baseline:manage"), baselineHandler.UpdateBaselineItem)
	baselineGroup.DELETE("/:id/items/:itemId/", middleware.RequirePermission("baseline:manage"), baselineHandler.DeleteBaselineItem)
	securityScans := engine.Group("/sys/security/scans", middleware.Authenticate(tokens))
	securityScans.GET("/", middleware.RequirePermission("baseline:manage"), baselineHandler.ListScans)
	securityScans.GET("/:id/", middleware.RequirePermission("baseline:manage"), baselineHandler.GetScan)
	securityScans.POST("/:id/cancel/", middleware.RequirePermission("baseline:scan"), baselineHandler.CancelScan)

	templates := engine.Group("/assets/application-deployment-templates", middleware.Authenticate(tokens))
	templates.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListDeploymentTemplates)
	templates.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetDeploymentTemplate)
	templates.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateDeploymentTemplate)
	templates.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateDeploymentTemplate)
	templates.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateDeploymentTemplate)
	templates.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteDeploymentTemplates)

	services := engine.Group("/assets/application-services", middleware.Authenticate(tokens))
	services.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListApplicationServices)
	services.GET("/:id/log-config/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationServiceLogConfig)
	// 路径通配按需展开（只读展示）：把含 * 的日志路径在承载实例上展开成真实文件清单。
	services.GET("/:id/log-config/glob/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationServiceLogGlob)
	// 日志格式认证（架构文档 §4.8）：对一条 (逻辑服务 × 日志定义) 抽样校验一次格式。
	// 用 update 权限：认证要写回 format_verified_*（含人工豁免），属于改服务配置。
	services.POST("/:id/log-config/verify/", middleware.RequirePermission("assets:applications:update"), assetsHandler.VerifyApplicationServiceLogFormat)
	// 按行保存日志覆盖值（采集开关 / 保留档位），日志中心页的内联操作用。
	services.POST("/:id/log-config/settings/", middleware.RequirePermission("assets:applications:update"), assetsHandler.SaveApplicationServiceLogSetting)
	// 服务级日志采集总开关（关掉后该服务下所有日志都不采集，逐条开关不生效）。
	services.POST("/:id/log-collection/", middleware.RequirePermission("assets:applications:update"), assetsHandler.SetApplicationServiceLogCollection)
	services.POST("/:id/refresh-runtime-status/", middleware.RequirePermission("assets:applications:update"), assetsHandler.RefreshApplicationServiceRuntimeStatus)
	services.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationService)
	services.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateApplicationService)
	services.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplicationService)
	services.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteApplicationServices)
	deployments := engine.Group("/assets/application-deployments", middleware.Authenticate(tokens))
	deployments.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListApplicationDeployments)
	deployments.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationDeployment)
	deployments.POST("/:id/control/", middleware.RequirePermission("assets:applications:update"), assetsHandler.ControlApplicationDeployment)
	deployments.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateApplicationDeployment)
	deployments.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplicationDeployment)
	deployments.POST("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.BatchDeleteApplicationDeployments)

	inspectionHandler := inspection.NewHandler(database, gateway)
	// Cron-configured inspection tasks are dispatched from the API process because
	// executions go through the in-process Agent gateway.
	inspectionHandler.StartScheduler()
	inspectionGroups := engine.Group("/sys/inspection/groups", middleware.Authenticate(tokens))
	inspectionGroups.GET("/", middleware.RequirePermission("inspection:view"), inspectionHandler.ListGroups)
	inspectionGroups.GET("/:id/", middleware.RequirePermission("inspection:view"), inspectionHandler.GetGroup)
	inspectionGroups.POST("/", middleware.RequirePermission("inspection:groups:create"), inspectionHandler.SaveGroup)
	inspectionGroups.PATCH("/:id/", middleware.RequirePermission("inspection:groups:update"), inspectionHandler.SaveGroup)
	inspectionGroups.POST("/batch-delete/", middleware.RequirePermission("inspection:groups:delete"), inspectionHandler.BatchDeleteGroups)
	inspectionTasks := engine.Group("/sys/inspection/tasks", middleware.Authenticate(tokens))
	inspectionTasks.GET("/", middleware.RequirePermission("inspection:view"), inspectionHandler.ListTasks)
	inspectionTasks.GET("/:id/", middleware.RequirePermission("inspection:view"), inspectionHandler.GetTask)
	inspectionTasks.POST("/", middleware.RequirePermission("inspection:tasks:create"), inspectionHandler.SaveTask)
	inspectionTasks.PATCH("/:id/", middleware.RequirePermission("inspection:tasks:update"), inspectionHandler.SaveTask)
	inspectionTasks.POST("/batch-delete/", middleware.RequirePermission("inspection:tasks:delete"), inspectionHandler.BatchDeleteTasks)
	inspectionTasks.POST("/:id/run/", middleware.RequirePermission("inspection:tasks:run"), inspectionHandler.RunTask)
	inspectionExecutions := engine.Group("/sys/inspection/executions", middleware.Authenticate(tokens))
	inspectionExecutions.GET("/", middleware.RequirePermission("inspection:view"), inspectionHandler.ListExecutions)
	inspectionExecutions.GET("/:id/", middleware.RequirePermission("inspection:view"), inspectionHandler.GetExecution)
	inspectionExecutions.POST("/:id/cancel/", middleware.RequirePermission("inspection:executions:cancel"), inspectionHandler.CancelExecution)

	// 监控域与日志采集域共用的两件依赖，集中构造后分别注入，避免各自再建一份：
	// 凭据加解密器（读写 Elasticsearch 集群口令等敏感配置）、软件包根目录
	//（autoadmin 自身的 media 目录：monitor_packages/ 与 agent_packages/）。
	// Django 后端已废弃（源码已移出版本库），包存储随之从 MEDIA_ROOT 迁出。
	secretEncryptor, err := assets.NewSecretEncryptor(credentialEncryptionKey, djangoSecret)
	if err != nil {
		return nil, nil, err
	}
	packageRoot, err := filepath.Abs("media")
	if err != nil {
		return nil, nil, err
	}
	monitorHandler := monitor.NewHandler(database, gateway, playbookHandler, secretEncryptor, packageRoot)
	// 目标安装选包缺少主机架构时，允许 monitor 主动调 assets 补采一次资产信息。
	monitorHandler.SetHostInfoRefresher(assetsHandler.RefreshHostInfoByID)
	// 日志采集域（Filebeat 纳管目标、采集配置下发、链路体检、数据流清理、ES 集群与检索）
	// 与监控域挂在同一个 /monitor 路由组下，共用组级鉴权；两者的依赖由上面统一构造。
	logcollectHandler := logcollect.NewHandler(database, gateway, playbookHandler, secretEncryptor, packageRoot)
	// Filebeat 选包同样可能在架构缺失时补采一次资产信息。
	logcollectHandler.SetHostInfoRefresher(assetsHandler.RefreshHostInfoByID)
	// 日志格式认证（架构文档 §4.8）：编排与写回在 assets 侧，取样例 + 跑 ES 在日志采集域
	// （只有它持有 agent 文件通道与 ES 客户端）→ 反向注入，避免 assets 反向依赖 logcollect 成环。
	assetsService.SetLogFormatVerifier(logcollectHandler)
	// 日志路径通配的按需展开（界面「解析后」列）：实现同样在日志采集域（持有 agent 文件通道）。
	assetsService.SetLogGlobPreviewer(logcollectHandler)
	// 保存逻辑服务时的"采集配置自洽性"校验（2026-09-19）：配置不自洽（路径展不开、
	// 同主机上两个实例展开成同一路径、正则编译不过）就拒绝保存——这些问题是"只能回到配置里改"的，
	// 等到下发才以"跳过并告警"暴露出来就太晚了（同主机同路径会让同一条日志进 ES 两次）。
	// 同样是反向注入：渲染与宏展开的实现都在日志采集域。
	assetsService.SetLogConfigConsistencyChecker(logcollectHandler)
	// 主机列表要显示采集配置的"配置状态"，但渲染采集配置属于日志采集域 → 反向注入。
	monitorHandler.SetLogConfigStateEvaluator(logcollectHandler.EvaluateLogConfigStates)
	// 批量动作执行器：入队 + 有界并发 + 进度可查（见 logcollect/log_batch_job.go）。
	// 发布者取自 schedulerPublisher——它是同一个 rabbit 客户端，只是这里用采集队列那条路由；
	// 拿不到投递能力（如测试或未接队列的进程）时 runner 仍会建起来，只是批量作业接口会
	// 明确报"执行器未装配"，而不是静默地同步跑几千台。
	resolvedBatchOptions := logcollect.DefaultLogBatchOptions()
	if batchOptions.InstallConcurrency > 0 {
		resolvedBatchOptions.InstallConcurrency = batchOptions.InstallConcurrency
	}
	if batchOptions.ApplyConcurrency > 0 {
		resolvedBatchOptions.ApplyConcurrency = batchOptions.ApplyConcurrency
	}
	if batchOptions.Prefetch > 0 {
		resolvedBatchOptions.Prefetch = batchOptions.Prefetch
	}
	if batchOptions.Budget > 0 {
		resolvedBatchOptions.Budget = batchOptions.Budget
	}
	batchPublisher, _ := schedulerPublisher.(logBatchPublisher)
	batchRunner := logcollect.NewLogBatchRunner(logcollectHandler, batchPublisher, resolvedBatchOptions)
	logcollectHandler.SetLogBatchRunner(batchRunner)
	monitorRoutes := engine.Group("/monitor", middleware.Authenticate(tokens), middleware.RequirePermission("monitor:view"))
	monitorRoutes.GET("/summary/", monitorHandler.Summary)
	monitorRoutes.GET("/packages/", monitorHandler.ListPackages)
	monitorRoutes.POST("/packages/", monitorHandler.CreateSoftwarePackage)
	monitorRoutes.GET("/packages/:id/", monitorHandler.GetSoftwarePackage)
	monitorRoutes.PATCH("/packages/:id/", monitorHandler.UpdateSoftwarePackage)
	monitorRoutes.PUT("/packages/:id/", monitorHandler.UpdateSoftwarePackage)
	monitorRoutes.POST("/packages/:id/upload/", monitorHandler.UploadSoftwarePackage)
	monitorRoutes.POST("/packages/:id/sync-official/", monitorHandler.SyncSoftwarePackageFromOfficial)
	monitorRoutes.POST("/packages/batch-delete/", monitorHandler.BatchDeleteSoftwarePackages)
	monitorRoutes.GET("/targets/", monitorHandler.ListTargets)
	monitorRoutes.GET("/targets/host-group-tree/", monitorHandler.HostGroupTree)
	monitorRoutes.GET("/targets/exporter-options/", monitorHandler.ExporterOptions)
	monitorRoutes.GET("/targets/host-overview/", monitorHandler.HostOverview)
	monitorRoutes.POST("/targets/batch-create/", monitorHandler.BatchCreateTargets)
	monitorRoutes.POST("/targets/batch-delete/", monitorHandler.BatchDeleteTargets)
	monitorRoutes.POST("/targets/batch-start-service/", monitorHandler.BatchStartTargetService)
	monitorRoutes.POST("/targets/batch-stop-service/", monitorHandler.BatchStopTargetService)
	monitorRoutes.GET("/targets/:id/", monitorHandler.GetTarget)
	monitorRoutes.PATCH("/targets/:id/", monitorHandler.UpdateTarget)
	monitorRoutes.PUT("/targets/:id/", monitorHandler.UpdateTarget)
	monitorRoutes.POST("/targets/:id/retry/", monitorHandler.RetryTarget)
	monitorRoutes.POST("/targets/:id/cancel/", monitorHandler.CancelTarget)
	monitorRoutes.POST("/targets/:id/check-service-status/", monitorHandler.CheckTargetService)
	monitorRoutes.POST("/targets/:id/start-service/", monitorHandler.StartTargetService)
	monitorRoutes.POST("/targets/:id/stop-service/", monitorHandler.StopTargetService)
	monitorRoutes.GET("/install-histories/", monitorHandler.ListInstallHistories)
	monitorRoutes.GET("/install-histories/:id/", monitorHandler.GetInstallHistory)
	monitorRoutes.POST("/install-histories/:id/cancel/", monitorHandler.CancelInstallHistory)
	// 采集目标（Filebeat 纳管）单独挂 monitor:log_collect:view：这批接口只服务「日志管理 → 日志采集」，
	// 与 exporter 目标互不相干（同 . 主机列表 /targets/host-overview/ 两个页面共用，仍留在组级的
	// monitor:view 下）。存量登录用户的权限码随 JWT 签发，升级后需重新登录才能访问。
	logTargets := monitorRoutes.Group("/log-targets", middleware.RequirePermission("monitor:log_collect:view"))
	logTargets.POST("/batch-create/", logcollectHandler.BatchCreateLogTargets)
	logTargets.POST("/batch-start-service/", logcollectHandler.BatchStartLogTargets)
	logTargets.POST("/batch-stop-service/", logcollectHandler.BatchStopLogTargets)
	// 批量动作（下发配置 / 安装重试）不再在请求里跑完：建作业 + 入队，前端轮询作业进度。
	// 这两个动作原本分别是 /batch-apply/ 与 /batch-retry/（同步、逐台串行，1000 台必超时）。
	logTargets.POST("/batch-jobs/", logcollectHandler.CreateLogBatchJob)
	// 服务维度的下发：对"承载该服务的全部已纳管主机"逐个全量重下发（一次批量作业，进度可查）。
	// 仍然逐台下发，原因见 logcollect/log_apply_service.go 的说明——agent 侧 apply 是全量替换，
	// 只推一个服务的片段会删掉同主机其他服务的配置。
	logTargets.GET("/service-config-state/", logcollectHandler.GetServiceLogConfigState)
	// 服务维度的"采集链路"诊断：查不到日志时按层回答断在哪（agent / Filebeat / 配置 / 规则 / 写入）。
	// 判定复用链路体检的同名函数，见 logcollect/log_service_chain.go。
	logTargets.GET("/service-collection-chain/", logcollectHandler.GetServiceCollectionChain)
	logTargets.POST("/service-apply/", logcollectHandler.ApplyLogTargetsForService)
	logTargets.GET("/batch-jobs/active/", logcollectHandler.GetActiveLogBatchJob)
	logTargets.GET("/batch-jobs/:id/", logcollectHandler.GetLogBatchJob)
	// 全量"待下发"口径（列表的 config_state 筛选是逐页/全量二选一，这里是单独特意的一次数统计）。
	logTargets.GET("/pending-summary/", logcollectHandler.PendingConfigSummary)
	logTargets.POST("/:id/retry/", logcollectHandler.RetryLogTarget)
	logTargets.POST("/:id/cancel/", logcollectHandler.CancelLogTarget)
	logTargets.POST("/batch-delete/", logcollectHandler.BatchDeleteLogTargets)
	logTargets.POST("/:id/check-status/", logcollectHandler.CheckLogTargetService)
	logTargets.POST("/:id/start-service/", logcollectHandler.StartLogTargetService)
	logTargets.POST("/:id/stop-service/", logcollectHandler.StopLogTargetService)
	logTargets.POST("/:id/apply/", logcollectHandler.ApplyLogTargetConfig)
	monitorRoutes.POST("/log-datastreams/cleanup/", logcollectHandler.CleanupLogDataStream)
	// 按数据流名清理：只收**未识别流**（有服务归属的必须走服务维度那条路，见 handler 注释）。
	monitorRoutes.POST("/log-datastreams/cleanup-stream/", logcollectHandler.CleanupLogDataStreamByStream)
	monitorRoutes.GET("/alert-histories/", monitorHandler.ListAlertHistories)
	monitorRoutes.GET("/alert-histories/:id/", monitorHandler.GetAlertHistory)
	monitorRoutes.GET("/alert-histories/:id/notification-status/", monitorHandler.AlertNotificationStatus)
	monitorRoutes.GET("/alert-notification/user-chain/", monitorHandler.UserAlertChain)
	monitorRoutes.GET("/alert-notification/chain/:historyId/", monitorHandler.AlertChainEvaluation)
	monitorRoutes.GET("/media/", monitorHandler.ListAlertMedia)
	monitorRoutes.POST("/media/", monitorHandler.CreateAlertMedia)
	monitorRoutes.GET("/media/:id/", monitorHandler.GetAlertMedia)
	monitorRoutes.PATCH("/media/:id/", monitorHandler.UpdateAlertMedia)
	monitorRoutes.PUT("/media/:id/", monitorHandler.UpdateAlertMedia)
	monitorRoutes.POST("/media/:id/test/", monitorHandler.TestAlertMedia)
	monitorRoutes.POST("/media/batch-delete/", monitorHandler.BatchDeleteAlertMedia)
	monitorRoutes.GET("/notification-policies/", monitorHandler.ListNotificationPolicies)
	monitorRoutes.POST("/notification-policies/create/", monitorHandler.CreateNotificationPolicy)
	monitorRoutes.POST("/notification-policies/update/", monitorHandler.UpdateNotificationPolicy)
	monitorRoutes.POST("/notification-policies/batch-delete/", monitorHandler.BatchDeleteNotificationPolicies)
	monitorRoutes.GET("/log-retention-tiers/", logcollectHandler.ListRetentionTiers)
	monitorRoutes.POST("/log-retention-tiers/", logcollectHandler.CreateRetentionTier)
	monitorRoutes.GET("/log-retention-tiers/:id/", logcollectHandler.GetRetentionTier)
	monitorRoutes.PATCH("/log-retention-tiers/:id/", logcollectHandler.UpdateRetentionTier)
	monitorRoutes.PUT("/log-retention-tiers/:id/", logcollectHandler.UpdateRetentionTier)
	monitorRoutes.POST("/log-retention-tiers/batch-delete/", logcollectHandler.BatchDeleteRetentionTiers)
	monitorRoutes.GET("/log-processing-rules/", logcollectHandler.ListProcessingRules)
	monitorRoutes.POST("/log-processing-rules/", logcollectHandler.CreateProcessingRule)
	monitorRoutes.GET("/log-processing-rules/:id/", logcollectHandler.GetProcessingRule)
	monitorRoutes.PATCH("/log-processing-rules/:id/", logcollectHandler.UpdateProcessingRule)
	monitorRoutes.PUT("/log-processing-rules/:id/", logcollectHandler.UpdateProcessingRule)
	monitorRoutes.POST("/log-processing-rules/batch-delete/", logcollectHandler.BatchDeleteProcessingRules)
	monitorRoutes.GET("/log-collection-filter-rules/", logcollectHandler.ListFilterRules)
	monitorRoutes.POST("/log-collection-filter-rules/", logcollectHandler.CreateFilterRule)
	monitorRoutes.GET("/log-collection-filter-rules/:id/", logcollectHandler.GetFilterRule)
	monitorRoutes.PATCH("/log-collection-filter-rules/:id/", logcollectHandler.UpdateFilterRule)
	monitorRoutes.PUT("/log-collection-filter-rules/:id/", logcollectHandler.UpdateFilterRule)
	monitorRoutes.POST("/log-collection-filter-rules/batch-delete/", logcollectHandler.BatchDeleteFilterRules)
	monitorRoutes.GET("/elasticsearch-clusters/", logcollectHandler.ListElasticsearchClusters)
	monitorRoutes.POST("/elasticsearch-clusters/", logcollectHandler.CreateElasticsearchCluster)
	monitorRoutes.GET("/elasticsearch-clusters/:id/", logcollectHandler.GetElasticsearchCluster)
	monitorRoutes.PATCH("/elasticsearch-clusters/:id/", logcollectHandler.UpdateElasticsearchCluster)
	monitorRoutes.PUT("/elasticsearch-clusters/:id/", logcollectHandler.UpdateElasticsearchCluster)
	monitorRoutes.POST("/elasticsearch-clusters/batch-delete/", logcollectHandler.BatchDeleteElasticsearchClusters)
	monitorRoutes.POST("/elasticsearch-clusters/:id/test-connection/", logcollectHandler.TestElasticsearchConnection)
	monitorRoutes.GET("/elasticsearch-clusters/:id/log-health/", logcollectHandler.ElasticsearchLogHealth)
	monitorRoutes.GET("/elasticsearch-clusters/:id/index-template/", logcollectHandler.GetElasticsearchIndexTemplate)
	monitorRoutes.POST("/elasticsearch-clusters/:id/pipeline-simulate/", logcollectHandler.SimulateElasticsearchPipeline)
	monitorRoutes.GET("/elasticsearch-clusters/:id/log-search/", logcollectHandler.ElasticsearchLogSearch)
	monitorRoutes.POST("/elasticsearch-clusters/:id/log-refresh/", logcollectHandler.ElasticsearchLogRefresh)
	monitorRoutes.GET("/elasticsearch-clusters/:id/log-facet-stats/", logcollectHandler.ElasticsearchLogFacetStats)
	monitorRoutes.GET("/elasticsearch-clusters/:id/log-storage-overview/", logcollectHandler.GetLogStorageOverview)
	monitorRoutes.GET("/elasticsearch-clusters/:id/log-service-usage/", logcollectHandler.GetLogServiceUsage)
	monitorRoutes.GET("/log-targets/:id/config-preview/", logcollectHandler.GetHostLogConfigPreview)
	monitorRoutes.GET("/targets/summary/", monitorHandler.Summary)
	monitorRoutes.GET("/targets/prometheus/overview/", monitorHandler.PrometheusOverview)
	monitorRoutes.GET("/targets/prometheus/targets/", monitorHandler.PrometheusTargets)
	monitorRoutes.GET("/targets/prometheus/alerts/", monitorHandler.PrometheusAlerts)
	monitorRoutes.GET("/targets/prometheus/rules/", monitorHandler.PrometheusRules)
	monitorRoutes.GET("/targets/prometheus/tsdb-status/", monitorHandler.PrometheusTSDBStatus)
	monitorRoutes.GET("/targets/prometheus/config/", monitorHandler.PrometheusConfig)
	monitorRoutes.GET("/targets/prometheus/flags/", monitorHandler.PrometheusFlags)
	monitorRoutes.GET("/targets/prometheus/query/", monitorHandler.PrometheusQuery)
	monitorRoutes.GET("/targets/prometheus/query-range/", monitorHandler.PrometheusQueryRange)
	monitorRoutes.Any("/targets/prometheus/proxy/*apiPath", monitorHandler.PrometheusProxy)
	engine.GET("/monitor/prometheus/http-sd/", monitorHandler.MachineAuthenticate(), monitorHandler.PrometheusHTTPServiceDiscovery)
	engine.GET("/monitor/targets/prometheus/http-sd/", monitorHandler.MachineAuthenticate(), monitorHandler.PrometheusHTTPServiceDiscovery)
	engine.POST("/monitor/alert-webhook/api/v2/alerts", monitorHandler.MachineAuthenticate(), monitorHandler.AlertWebhook)

	// Preserve Django's public route boundaries while handlers are migrated domain by domain.
	for _, prefix := range []string{"/sys", "/sys/scheduler", "/sys/automation", "/sys/inspection", "/sys/audit", "/assets", "/monitor", "/api/agent"} {
		engine.Group(prefix)
	}
	return engine, batchRunner, nil
}

func readiness(database *sql.DB) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		ctx, cancel := context.WithTimeout(ginContext.Request.Context(), 2*time.Second)
		defer cancel()
		if err := database.PingContext(ctx); err != nil {
			ginContext.JSON(http.StatusServiceUnavailable, response.Envelope{
				Code: 503,
				Msg:  "database unavailable",
				Data: nil,
			})
			return
		}
		response.Success(ginContext, gin.H{"status": "ready"})
	}
}
