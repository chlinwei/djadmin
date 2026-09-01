package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os/signal"
	"strings"
	"syscall"

	"autoadmin/internal/agent"
	"autoadmin/internal/api"
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

func openDatabase(ctx context.Context, configuration config.Config) (*sql.DB, error) {
	return database.OpenMySQL(ctx, database.MySQLConfig{
		DSN: configuration.MySQLDSN, MaxOpenConns: configuration.MySQLMaxOpen,
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
	agentGateway := agent.NewGateway(newAgentTokenValidator(databaseConnection))
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
			  AND agent_id = 'global'
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
