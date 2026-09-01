package router

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/middleware"
	"autoadmin/internal/api/response"
	"autoadmin/internal/assets"
	"autoadmin/internal/audit"
	"autoadmin/internal/automation"
	"autoadmin/internal/identity"
	"autoadmin/internal/inspection"
	"autoadmin/internal/monitor"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/rbac"
	"autoadmin/internal/scheduler"
	"autoadmin/internal/sysconfig"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func New(database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string) (*gin.Engine, error) {
	return NewWithGateway(database, tokens, allowedOrigins, schedulerPublisher, credentialEncryptionKey, djangoSecret, nil)
}

func NewWithGateway(database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string, gateway *agent.Gateway) (*gin.Engine, error) {
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
	apiTokens := engine.Group("/sys/usercenter", middleware.Authenticate(tokens))
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
	playbooks.DELETE("/:id/", middleware.RequirePermission("automation:playbooks:delete"), playbookHandler.Delete)
	automationInventories := engine.Group("/sys/automation/inventories", middleware.Authenticate(tokens))
	automationInventories.GET("/", middleware.RequirePermission("automation:inventory:view"), playbookHandler.ListInventories)
	automationInventories.POST("/", middleware.RequirePermission("automation:inventory:create"), playbookHandler.CreateInventory)
	automationInventories.GET("/:id/", middleware.RequirePermission("automation:inventory:view"), playbookHandler.GetInventory)
	automationInventories.PATCH("/:id/", middleware.RequirePermission("automation:inventory:update"), playbookHandler.UpdateInventory)
	automationInventories.PUT("/:id/", middleware.RequirePermission("automation:inventory:update"), playbookHandler.UpdateInventory)
	automationInventories.DELETE("/:id/", middleware.RequirePermission("automation:inventory:delete"), playbookHandler.DeleteInventory)
	automationInventories.POST("/:id/precheck-limit/", middleware.RequirePermission("automation:inventory:view"), playbookHandler.PrecheckInventoryLimit)
	automationTasks := engine.Group("/sys/automation/tasks", middleware.Authenticate(tokens))
	automationTasks.GET("/", middleware.RequirePermission("automation:tasks:view"), playbookHandler.ListTasks)
	automationTasks.POST("/", middleware.RequirePermission("automation:tasks:create"), playbookHandler.CreateTask)
	automationTasks.GET("/:id/", middleware.RequirePermission("automation:tasks:view"), playbookHandler.GetTask)
	automationTasks.PATCH("/:id/", middleware.RequirePermission("automation:tasks:update"), playbookHandler.UpdateTask)
	automationTasks.PUT("/:id/", middleware.RequirePermission("automation:tasks:update"), playbookHandler.UpdateTask)
	automationTasks.DELETE("/:id/", middleware.RequirePermission("automation:tasks:delete"), playbookHandler.DeleteTask)
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
	users.DELETE("/userBatchDelete/", middleware.RequirePermission("system:users:delete"), identityHandler.BatchDeleteUsers)
	users.POST("/resetUserPwd/", middleware.RequirePermission("system:users:update"), identityHandler.ResetPassword)
	users.POST("/assginUserRoles/", middleware.RequirePermission("system:users:update"), identityHandler.AssignRoles)
	users.POST("/changeUserStatus/", middleware.RequirePermission("system:users:update"), identityHandler.ChangeUserStatus)
	users.GET("/getUserRolesById/", middleware.RequirePermission("system:users:view"), identityHandler.GetUserRoles)

	rbacRepository := rbac.NewRepository(database)
	rbacHandler := rbac.NewHandler(rbac.NewService(rbacRepository))
	roles := engine.Group("/sys/roles", middleware.Authenticate(tokens))
	roles.GET("/", middleware.RequirePermission("system:roles:view"), rbacHandler.List)
	roles.GET("/:id/", middleware.RequirePermission("system:roles:view"), rbacHandler.Get)
	roles.POST("/", middleware.RequirePermission("system:roles:create"), rbacHandler.Create)
	roles.PATCH("/:id/", middleware.RequirePermission("system:roles:update"), rbacHandler.Update)
	roles.DELETE("/batch-delete/", middleware.RequirePermission("system:roles:delete"), rbacHandler.DeleteMany)
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
		return nil, err
	}
	assetsHandler := assets.NewHandler(assetsService, gateway)
	assets.SetDeploymentGateway(gateway)
	engine.GET("/api/agent/jobs/query", middleware.Authenticate(tokens), assetsHandler.QueryAgentJobs)
	projects := engine.Group("/assets/projects", middleware.Authenticate(tokens))
	projects.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListProjects)
	projects.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetProject)
	projects.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateProject)
	projects.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProject)
	projects.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProject)
	projects.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteProject)

	businessSystems := engine.Group("/assets/business-systems", middleware.Authenticate(tokens))
	businessSystems.GET("/", middleware.RequirePermission("assets:service-tree:view"), assetsHandler.ListBusinessSystems)
	businessSystems.GET("/:id/", middleware.RequirePermission("assets:service-tree:view"), assetsHandler.GetBusinessSystem)
	businessSystems.POST("/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.CreateBusinessSystem)
	businessSystems.PUT("/:id/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.UpdateBusinessSystem)
	businessSystems.PATCH("/:id/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.UpdateBusinessSystem)
	businessSystems.DELETE("/:id/", middleware.RequirePermission("assets:service-tree:manage"), assetsHandler.DeleteBusinessSystem)

	environments := engine.Group("/assets/business-environments", middleware.Authenticate(tokens))
	environments.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListEnvironments)
	environments.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetEnvironment)
	environments.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateEnvironment)
	environments.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateEnvironment)
	environments.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateEnvironment)
	environments.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteEnvironment)

	credentials := engine.Group("/assets/credentials", middleware.Authenticate(tokens))
	credentials.GET("/", middleware.RequirePermission("assets:credentials:view"), assetsHandler.ListCredentials)
	credentials.DELETE("/batch-delete/", middleware.RequirePermission("assets:credentials:delete"), assetsHandler.DeleteCredentials)
	credentials.GET("/:id/", middleware.RequirePermission("assets:credentials:view"), assetsHandler.GetCredential)
	credentials.POST("/", middleware.RequirePermission("assets:credentials:create"), assetsHandler.CreateCredential)
	credentials.PUT("/:id/", middleware.RequirePermission("assets:credentials:update"), assetsHandler.UpdateCredential)
	credentials.PATCH("/:id/", middleware.RequirePermission("assets:credentials:update"), assetsHandler.UpdateCredential)
	credentials.DELETE("/:id/", middleware.RequirePermission("assets:credentials:delete"), assetsHandler.DeleteCredential)

	hostGroups := engine.Group("/assets/host-groups", middleware.Authenticate(tokens))
	hostGroups.GET("/", middleware.RequirePermission("assets:hostgroups:view"), assetsHandler.ListHostGroups)
	hostGroups.GET("/tree/", middleware.RequirePermission("assets:hostgroups:view"), assetsHandler.HostGroupTree)
	hostGroups.GET("/:id/", middleware.RequirePermission("assets:hostgroups:view"), assetsHandler.GetHostGroup)
	hostGroups.POST("/", middleware.RequirePermission("assets:hostgroups:create"), assetsHandler.CreateHostGroup)
	hostGroups.PUT("/:id/", middleware.RequirePermission("assets:hostgroups:update"), assetsHandler.UpdateHostGroup)
	hostGroups.PATCH("/:id/", middleware.RequirePermission("assets:hostgroups:update"), assetsHandler.UpdateHostGroup)
	hostGroups.DELETE("/:id/", middleware.RequirePermission("assets:hostgroups:delete"), assetsHandler.DeleteHostGroup)

	hosts := engine.Group("/assets/hosts", middleware.Authenticate(tokens))
	hosts.GET("/:id/webssh/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.WebSSH)
	hosts.GET("/:id/agent-runtime-status/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.GetHostAgentRuntimeStatus)
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
	hosts.DELETE("/batch-delete/", middleware.RequirePermission("assets:hosts:delete"), assetsHandler.DeleteHosts)
	hosts.GET("/:id/", middleware.RequirePermission("assets:hosts:view"), assetsHandler.GetHost)
	hosts.POST("/", middleware.RequirePermission("assets:hosts:create"), assetsHandler.CreateHost)
	hosts.PUT("/:id/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.UpdateHost)
	hosts.PATCH("/:id/", middleware.RequirePermission("assets:hosts:update"), assetsHandler.UpdateHost)
	hosts.DELETE("/:id/", middleware.RequirePermission("assets:hosts:delete"), assetsHandler.DeleteHost)

	engine.GET("/ws/assets/hosts/:id/webssh/", middleware.Authenticate(tokens), middleware.RequirePermission("assets:hosts:view"), assetsHandler.WebSSH)

	applications := engine.Group("/assets/applications", middleware.Authenticate(tokens))
	applications.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListApplications)
	applications.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplication)
	applications.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateApplication)
	applications.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplication)
	applications.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplication)
	applications.DELETE("/batch-delete/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteApplications)
	versions := engine.Group("/assets/application-versions", middleware.Authenticate(tokens))
	versions.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListVersions)
	versions.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetVersion)
	versions.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateVersion)
	versions.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateVersion)
	versions.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateVersion)
	versions.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteVersion)
	profiles := engine.Group("/assets/cluster-profiles", middleware.Authenticate(tokens))
	profiles.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListProfiles)
	profiles.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetProfile)
	profiles.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateProfile)
	profiles.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProfile)
	profiles.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateProfile)
	profiles.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteProfile)

	templates := engine.Group("/assets/application-deployment-templates", middleware.Authenticate(tokens))
	templates.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListDeploymentTemplates)
	templates.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetDeploymentTemplate)
	templates.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateDeploymentTemplate)
	templates.PUT("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateDeploymentTemplate)
	templates.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateDeploymentTemplate)
	templates.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteDeploymentTemplate)

	services := engine.Group("/assets/application-services", middleware.Authenticate(tokens))
	services.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListApplicationServices)
	services.GET("/:id/log-config/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationServiceLogConfig)
	services.POST("/:id/refresh-runtime-status/", middleware.RequirePermission("assets:applications:update"), assetsHandler.RefreshApplicationServiceRuntimeStatus)
	services.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationService)
	services.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateApplicationService)
	services.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplicationService)
	services.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteApplicationService)
	deployments := engine.Group("/assets/application-deployments", middleware.Authenticate(tokens))
	deployments.GET("/", middleware.RequirePermission("assets:applications:view"), assetsHandler.ListApplicationDeployments)
	deployments.GET("/:id/", middleware.RequirePermission("assets:applications:view"), assetsHandler.GetApplicationDeployment)
	deployments.POST("/:id/control/", middleware.RequirePermission("assets:applications:update"), assetsHandler.ControlApplicationDeployment)
	deployments.POST("/", middleware.RequirePermission("assets:applications:create"), assetsHandler.CreateApplicationDeployment)
	deployments.PATCH("/:id/", middleware.RequirePermission("assets:applications:update"), assetsHandler.UpdateApplicationDeployment)
	deployments.DELETE("/:id/", middleware.RequirePermission("assets:applications:delete"), assetsHandler.DeleteApplicationDeployment)

	inspectionHandler := inspection.NewHandler(database, gateway)
	inspectionGroups := engine.Group("/sys/inspection/groups", middleware.Authenticate(tokens))
	inspectionGroups.GET("/", middleware.RequirePermission("inspection:view"), inspectionHandler.ListGroups)
	inspectionGroups.GET("/:id/", middleware.RequirePermission("inspection:view"), inspectionHandler.GetGroup)
	inspectionGroups.POST("/", middleware.RequirePermission("inspection:groups:create"), inspectionHandler.SaveGroup)
	inspectionGroups.PATCH("/:id/", middleware.RequirePermission("inspection:groups:update"), inspectionHandler.SaveGroup)
	inspectionGroups.DELETE("/:id/", middleware.RequirePermission("inspection:groups:delete"), inspectionHandler.DeleteGroup)
	inspectionTasks := engine.Group("/sys/inspection/tasks", middleware.Authenticate(tokens))
	inspectionTasks.GET("/", middleware.RequirePermission("inspection:view"), inspectionHandler.ListTasks)
	inspectionTasks.GET("/host-scope-tree/", middleware.RequirePermission("inspection:view"), inspectionHandler.HostScopeTree)
	inspectionTasks.GET("/:id/", middleware.RequirePermission("inspection:view"), inspectionHandler.GetTask)
	inspectionTasks.POST("/", middleware.RequirePermission("inspection:tasks:create"), inspectionHandler.SaveTask)
	inspectionTasks.PATCH("/:id/", middleware.RequirePermission("inspection:tasks:update"), inspectionHandler.SaveTask)
	inspectionTasks.DELETE("/:id/", middleware.RequirePermission("inspection:tasks:delete"), inspectionHandler.DeleteTask)
	inspectionTasks.POST("/:id/run/", middleware.RequirePermission("inspection:tasks:run"), inspectionHandler.RunTask)
	inspectionExecutions := engine.Group("/sys/inspection/executions", middleware.Authenticate(tokens))
	inspectionExecutions.GET("/", middleware.RequirePermission("inspection:view"), inspectionHandler.ListExecutions)
	inspectionExecutions.GET("/:id/", middleware.RequirePermission("inspection:view"), inspectionHandler.GetExecution)
	inspectionExecutions.POST("/:id/cancel/", middleware.RequirePermission("inspection:executions:cancel"), inspectionHandler.CancelExecution)

	monitorHandler, err := monitor.NewHandler(database, gateway, credentialEncryptionKey, djangoSecret)
	if err != nil {
		return nil, err
	}
	monitorRoutes := engine.Group("/monitor", middleware.Authenticate(tokens), middleware.RequirePermission("monitor:view"))
	monitorRoutes.GET("/summary/", monitorHandler.Summary)
	monitorRoutes.GET("/packages/", monitorHandler.ListPackages)
	monitorRoutes.POST("/packages/", monitorHandler.CreateSoftwarePackage)
	monitorRoutes.GET("/packages/:id/", monitorHandler.GetSoftwarePackage)
	monitorRoutes.PATCH("/packages/:id/", monitorHandler.UpdateSoftwarePackage)
	monitorRoutes.PUT("/packages/:id/", monitorHandler.UpdateSoftwarePackage)
	monitorRoutes.POST("/packages/:id/upload/", monitorHandler.UploadSoftwarePackage)
	monitorRoutes.DELETE("/packages/:id/", monitorHandler.DeleteSoftwarePackage)
	monitorRoutes.GET("/targets/", monitorHandler.ListTargets)
	monitorRoutes.GET("/targets/host-group-tree/", monitorHandler.HostGroupTree)
	monitorRoutes.GET("/targets/exporter-options/", monitorHandler.ExporterOptions)
	monitorRoutes.GET("/targets/host-overview/", monitorHandler.HostOverview)
	monitorRoutes.POST("/targets/batch-create/", monitorHandler.BatchCreateTargets)
	monitorRoutes.GET("/targets/:id/", monitorHandler.GetTarget)
	monitorRoutes.PATCH("/targets/:id/", monitorHandler.UpdateTarget)
	monitorRoutes.PUT("/targets/:id/", monitorHandler.UpdateTarget)
	monitorRoutes.DELETE("/targets/:id/", monitorHandler.DeleteTarget)
	monitorRoutes.POST("/targets/:id/cancel/", monitorHandler.CancelTarget)
	monitorRoutes.POST("/targets/:id/check-service-status/", monitorHandler.CheckTargetService)
	monitorRoutes.POST("/targets/:id/start-service/", monitorHandler.StartTargetService)
	monitorRoutes.POST("/targets/:id/stop-service/", monitorHandler.StopTargetService)
	monitorRoutes.GET("/install-histories/", monitorHandler.ListInstallHistories)
	monitorRoutes.GET("/install-histories/:id/", monitorHandler.GetInstallHistory)
	monitorRoutes.POST("/install-histories/:id/cancel/", monitorHandler.CancelInstallHistory)
	monitorRoutes.GET("/alert-histories/", monitorHandler.ListAlertHistories)
	monitorRoutes.GET("/alert-histories/:id/", monitorHandler.GetAlertHistory)
	monitorRoutes.GET("/alert-histories/:id/notification-status/", monitorHandler.AlertNotificationStatus)
	monitorRoutes.GET("/media/", monitorHandler.ListAlertMedia)
	monitorRoutes.POST("/media/", monitorHandler.CreateAlertMedia)
	monitorRoutes.GET("/media/:id/", monitorHandler.GetAlertMedia)
	monitorRoutes.PATCH("/media/:id/", monitorHandler.UpdateAlertMedia)
	monitorRoutes.PUT("/media/:id/", monitorHandler.UpdateAlertMedia)
	monitorRoutes.DELETE("/media/:id/", monitorHandler.DeleteAlertMedia)
	monitorRoutes.GET("/alert-routes/", monitorHandler.ListAlertRoutes)
	monitorRoutes.POST("/alert-routes/", monitorHandler.CreateAlertRoute)
	monitorRoutes.GET("/alert-routes/:id/", monitorHandler.GetAlertRoute)
	monitorRoutes.PATCH("/alert-routes/:id/", monitorHandler.UpdateAlertRoute)
	monitorRoutes.PUT("/alert-routes/:id/", monitorHandler.UpdateAlertRoute)
	monitorRoutes.DELETE("/alert-routes/:id/", monitorHandler.DeleteAlertRoute)
	monitorRoutes.GET("/log-retention-tiers/", monitorHandler.ListRetentionTiers)
	monitorRoutes.POST("/log-retention-tiers/", monitorHandler.CreateRetentionTier)
	monitorRoutes.GET("/log-retention-tiers/:id/", monitorHandler.GetRetentionTier)
	monitorRoutes.PATCH("/log-retention-tiers/:id/", monitorHandler.UpdateRetentionTier)
	monitorRoutes.PUT("/log-retention-tiers/:id/", monitorHandler.UpdateRetentionTier)
	monitorRoutes.DELETE("/log-retention-tiers/:id/", monitorHandler.DeleteRetentionTier)
	monitorRoutes.GET("/log-processing-rules/", monitorHandler.ListProcessingRules)
	monitorRoutes.POST("/log-processing-rules/", monitorHandler.CreateProcessingRule)
	monitorRoutes.GET("/log-processing-rules/:id/", monitorHandler.GetProcessingRule)
	monitorRoutes.PATCH("/log-processing-rules/:id/", monitorHandler.UpdateProcessingRule)
	monitorRoutes.PUT("/log-processing-rules/:id/", monitorHandler.UpdateProcessingRule)
	monitorRoutes.DELETE("/log-processing-rules/:id/", monitorHandler.DeleteProcessingRule)
	monitorRoutes.GET("/log-collection-filter-rules/", monitorHandler.ListFilterRules)
	monitorRoutes.POST("/log-collection-filter-rules/", monitorHandler.CreateFilterRule)
	monitorRoutes.GET("/log-collection-filter-rules/:id/", monitorHandler.GetFilterRule)
	monitorRoutes.PATCH("/log-collection-filter-rules/:id/", monitorHandler.UpdateFilterRule)
	monitorRoutes.PUT("/log-collection-filter-rules/:id/", monitorHandler.UpdateFilterRule)
	monitorRoutes.DELETE("/log-collection-filter-rules/:id/", monitorHandler.DeleteFilterRule)
	monitorRoutes.GET("/opensearch-clusters/", monitorHandler.ListOpenSearchClusters)
	monitorRoutes.POST("/opensearch-clusters/", monitorHandler.CreateOpenSearchCluster)
	monitorRoutes.GET("/opensearch-clusters/:id/", monitorHandler.GetOpenSearchCluster)
	monitorRoutes.PATCH("/opensearch-clusters/:id/", monitorHandler.UpdateOpenSearchCluster)
	monitorRoutes.PUT("/opensearch-clusters/:id/", monitorHandler.UpdateOpenSearchCluster)
	monitorRoutes.DELETE("/opensearch-clusters/:id/", monitorHandler.DeleteOpenSearchCluster)
	monitorRoutes.POST("/opensearch-clusters/:id/test-connection/", monitorHandler.TestOpenSearchConnection)
	monitorRoutes.POST("/opensearch-clusters/:id/pipeline-simulate/", monitorHandler.SimulateOpenSearchPipeline)
	monitorRoutes.GET("/opensearch-clusters/:id/log-search/", monitorHandler.OpenSearchLogSearch)
	monitorRoutes.GET("/opensearch-clusters/:id/log-facet-stats/", monitorHandler.OpenSearchLogFacetStats)
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
	monitorRoutes.GET("/targets/prometheus/proxy/*apiPath", monitorHandler.PrometheusProxy)
	engine.GET("/monitor/prometheus/http-sd/", monitorHandler.MachineAuthenticate(), monitorHandler.PrometheusHTTPServiceDiscovery)
	engine.GET("/monitor/targets/prometheus/http-sd/", monitorHandler.MachineAuthenticate(), monitorHandler.PrometheusHTTPServiceDiscovery)
	engine.POST("/monitor/alert-webhook/api/v2/alerts", monitorHandler.MachineAuthenticate(), monitorHandler.AlertWebhook)

	// Preserve Django's public route boundaries while handlers are migrated domain by domain.
	for _, prefix := range []string{"/sys", "/sys/scheduler", "/sys/automation", "/sys/inspection", "/sys/audit", "/assets", "/monitor", "/api/agent"} {
		engine.Group(prefix)
	}
	return engine, nil
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
