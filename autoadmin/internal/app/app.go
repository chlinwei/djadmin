package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/signal"
	"strings"
	"syscall"

	"autoadmin/internal/agent"
	"autoadmin/internal/api"
	"autoadmin/internal/buildinfo"
	"autoadmin/internal/config"
	"autoadmin/internal/identity"
	"autoadmin/internal/messaging/rabbitmq"
	"autoadmin/internal/platform/database"
	"autoadmin/internal/platform/migration"
	"autoadmin/internal/scheduler"
	"autoadmin/internal/shared/pagination"

	"google.golang.org/grpc"
)

var supportedCommands = map[string]struct{}{
	"api":       {},
	"migrate":   {},
	"scheduler": {},
	"worker":    {},
}

// Run dispatches independently deployable process roles from one binary.
func Run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: autoadmin <api|scheduler|worker|migrate>")
	}
	if _, ok := supportedCommands[args[0]]; !ok {
		return fmt.Errorf("unknown command %q", args[0])
	}
	slog.Info("autoadmin starting", "version", buildinfo.Version, "role", args[0])
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	if err := configuration.Validate(args[0]); err != nil {
		return err
	}

	switch args[0] {
	case "api":
		return runAPI(configuration)
	case "migrate":
		return migration.Up(configuration.MigrationSource, configuration.MigrationDBURL)
	case "scheduler":
		return runScheduler(configuration)
	case "worker":
		return runWorker(configuration)
	default:
		return fmt.Errorf("command %q is scaffolded but not implemented", args[0])
	}
}

// openDatabase 按构建期方言打开数据库（默认 MySQL；-tags postgres 走 PostgreSQL）。
func openDatabase(ctx context.Context, configuration config.Config) (*sql.DB, error) {
	return database.Open(ctx, database.Configuration{
		MySQLDSN: configuration.MySQLDSN, PostgresDSN: configuration.PostgresDSN,
		MaxOpenConns: configuration.MySQLMaxOpen,
		MaxIdleConns: configuration.MySQLMaxIdle, ConnMaxLifetime: configuration.MySQLMaxLife,
	})
}

func openRabbit(configuration config.Config) (*rabbitmq.Client, error) {
	client, err := rabbitmq.Dial(configuration.RabbitMQURL)
	if err != nil {
		return nil, err
	}
	if err := client.DeclareTopology(); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func runAPI(configuration config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	databaseConnection, err := openDatabase(ctx, configuration)
	if err != nil {
		return err
	}
	defer databaseConnection.Close()
	rabbitClient, err := openRabbit(configuration)
	if err != nil {
		return err
	}
	defer rabbitClient.Close()

	tokens := identity.NewTokenManager(configuration.JWTSecret, configuration.JWTExpiration)
	// 过渡期默认放行 agent 连接（AGENT_GRPC_AUTH_MODE=open），后续切换 mTLS 时这个口整体移除；
	// token 模式保留 sys_agent_token 校验能力，便于灰度回切。
	var agentValidator func(string, string) bool
	if configuration.AgentGRPCAuthMode == "token" {
		agentValidator = newAgentTokenValidator(databaseConnection)
	}
	agentGateway := agent.NewGateway(agentValidator, newAgentHelloRecorder(databaseConnection))
	server, err := api.NewServerWithGateway(configuration.HTTPAddress, databaseConnection, tokens, configuration.CORSOrigins, rabbitClient, configuration.AssetsCredentialEncryptionKey, configuration.JWTSecret, agentGateway)
	if err != nil {
		return err
	}
	errChannel := make(chan error, 1)
	go func() { errChannel <- server.Run() }()
	var grpcServer *grpc.Server
	if configuration.AgentGRPCAddress != "" {
		grpcListener, listenErr := net.Listen("tcp", configuration.AgentGRPCAddress)
		if listenErr != nil {
			return fmt.Errorf("listen Agent gRPC: %w", listenErr)
		}
		grpcServer = grpc.NewServer()
		agentGateway.Register(grpcServer)
		go func() {
			if serveErr := grpcServer.Serve(grpcListener); serveErr != nil {
				errChannel <- fmt.Errorf("serve Agent gRPC: %w", serveErr)
			}
		}()
	}

	select {
	case err := <-errChannel:
		return err
	case <-ctx.Done():
		if grpcServer != nil {
			// Agent streams are long-lived and reconnect automatically; graceful stop would wait indefinitely.
			grpcServer.Stop()
		}
		shutdownContext, cancel := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}

func newAgentTokenValidator(databaseConnection *sql.DB) func(string, string) bool {
	return func(agentID, token string) bool {
		if strings.TrimSpace(agentID) == "" || strings.TrimSpace(token) == "" {
			return false
		}
		rows, err := databaseConnection.Query(`
			SELECT token_hash
			FROM sys_agent_token
			WHERE bind_mode = 'agent'
			  AND api_id = 'global'
			  AND is_active = TRUE
			  AND (expires_at IS NULL OR expires_at > UTC_TIMESTAMP(6))`)
		if err != nil {
			return false
		}
		defer rows.Close()
		for rows.Next() {
			var encoded string
			if rows.Scan(&encoded) == nil && identity.VerifyPassword(encoded, token) {
				return true
			}
		}
		return false
	}
}

// newAgentHelloRecorder 在 agent 握手成功时把版本与在线状态落库：Hello.version 为
// 构建期注入的版本号，agent 安装/更新重启后即刷新，无需等待按需 get_host_info 采集。
// 仅更新已有记录；instance_name 未匹配到主机时跳过，失败只记日志，不阻断会话。
//
// dj-agent 上报 instance_name（= DJ_AGENT_INSTANCE_NAME = assets_host.instance_name）
// 作为 gRPC 会话标识，backend 按该列匹配主机行；主机没有独立的 agent_id 列。
func newAgentHelloRecorder(databaseConnection *sql.DB) func(instanceName, version string) {
	return func(instanceName, version string) {
		if strings.TrimSpace(instanceName) == "" {
			return
		}
		result, err := databaseConnection.Exec(`
			UPDATE assets_host
			SET agent_online = TRUE, agent_online_time = UTC_TIMESTAMP(6), update_time = UTC_TIMESTAMP(6)
			WHERE instance_name = ?`, instanceName)
		if err != nil {
			slog.Warn("agent hello: update host online failed", "instance_name", instanceName, "err", err)
			return
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			slog.Warn("agent hello: no host bound to instance_name", "instance_name", instanceName)
			return
		}
		if strings.TrimSpace(version) == "" {
			return
		}
		if _, err = databaseConnection.Exec(`
			UPDATE assets_hostsystem hs
			JOIN assets_host h ON h.id = hs.host_id
			SET hs.agent_version = ?, hs.update_time = UTC_TIMESTAMP(6)
			WHERE h.instance_name = ?`, version, instanceName); err != nil {
			slog.Warn("agent hello: update agent_version failed", "instance_name", instanceName, "version", version, "err", err)
		}
	}
}

func runScheduler(configuration config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	databaseConnection, err := openDatabase(ctx, configuration)
	if err != nil {
		return err
	}
	defer databaseConnection.Close()
	rabbitClient, err := openRabbit(configuration)
	if err != nil {
		return err
	}
	defer rabbitClient.Close()
	repository := scheduler.NewRepository(databaseConnection)
	tasks, _, err := repository.ListTasks(ctx, scheduler.TaskFilter{Enabled: boolPointer(true)}, pagination.New(1, pagination.MaxSize))
	if err != nil {
		return fmt.Errorf("load scheduled tasks: %w", err)
	}
	manager, err := scheduler.New(rabbitClient)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if !task.CronExpression.Valid || !scheduler.IsSupportedTaskCode(task.Code) {
			continue
		}
		if _, err := manager.Register(scheduler.Definition{ID: task.ID, Name: task.Name, Kind: "scheduled_task", CronExpression: task.CronExpression.String}); err != nil {
			return fmt.Errorf("register scheduled task %s: %w", task.Code, err)
		}
	}
	manager.Start()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	defer cancel()
	return manager.Shutdown(shutdownCtx)
}

func runWorker(configuration config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	databaseConnection, err := openDatabase(ctx, configuration)
	if err != nil {
		return err
	}
	defer databaseConnection.Close()
	rabbitClient, err := openRabbit(configuration)
	if err != nil {
		return err
	}
	defer rabbitClient.Close()
	err = rabbitClient.Consume(ctx, configuration.WorkerName, configuration.WorkerPrefetch, scheduler.NewWorker(scheduler.NewRepository(databaseConnection)))
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func boolPointer(value bool) *bool { return &value }
