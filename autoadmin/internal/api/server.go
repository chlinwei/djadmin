package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/router"
	"autoadmin/internal/identity"
	"autoadmin/internal/messaging/rabbitmq"
	"autoadmin/internal/scheduler"
)

type Server struct {
	server *http.Server
	// queueConsumer 是本进程要消费的作业队列（日志采集批量动作，见 rabbitmq.LogCollectRoute）。
	// 为 nil 表示这个进程只提供 HTTP 路由、不消费队列。
	queueConsumer rabbitmq.Consumer
}

func NewServer(address string, database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string) (*Server, error) {
	return NewServerWithGateway(address, database, tokens, allowedOrigins, schedulerPublisher, credentialEncryptionKey, djangoSecret, nil, router.LogBatchOptions{})
}

func NewServerWithGateway(address string, database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string, gateway *agent.Gateway, batchOptions router.LogBatchOptions) (*Server, error) {
	handler, consumer, err := router.NewWithGateway(database, tokens, allowedOrigins, schedulerPublisher, credentialEncryptionKey, djangoSecret, gateway, batchOptions)
	if err != nil {
		return nil, fmt.Errorf("configure API router: %w", err)
	}
	return &Server{
		server: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		queueConsumer: consumer,
	}, nil
}

func (server *Server) Run() error {
	if err := server.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

// ConsumeQueues 消费本进程负责的队列（阻塞到 ctx 结束），并拉起失联对账。
//
// 由 api 角色调用：日志采集的批量动作要通过 agent gRPC 会话在主机上执行，而 agent 会话是
// 本进程内的 Gateway 会话表，所以这条队列必须由"同时提供 HTTP 与 gRPC 服务"的进程消费
// （worker 角色里 gateway.IsOnline 恒为 false，作业会全部失败）。
func (server *Server) ConsumeQueues(ctx context.Context, client *rabbitmq.Client) error {
	if server.queueConsumer == nil {
		return nil
	}
	server.queueConsumer.StartReaper(ctx)
	return server.queueConsumer.Consume(ctx, client)
}

func (server *Server) Shutdown(ctx context.Context) error {
	return server.server.Shutdown(ctx)
}
