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
	"autoadmin/internal/scheduler"
)

type Server struct {
	server *http.Server
}

func NewServer(address string, database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string) (*Server, error) {
	return NewServerWithGateway(address, database, tokens, allowedOrigins, schedulerPublisher, credentialEncryptionKey, djangoSecret, nil)
}

func NewServerWithGateway(address string, database *sql.DB, tokens *identity.TokenManager, allowedOrigins []string, schedulerPublisher scheduler.Publisher, credentialEncryptionKey, djangoSecret string, gateway *agent.Gateway) (*Server, error) {
	handler, err := router.NewWithGateway(database, tokens, allowedOrigins, schedulerPublisher, credentialEncryptionKey, djangoSecret, gateway)
	if err != nil {
		return nil, fmt.Errorf("configure API router: %w", err)
	}
	return &Server{server: &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}}, nil
}

func (server *Server) Run() error {
	if err := server.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

func (server *Server) Shutdown(ctx context.Context) error {
	return server.server.Shutdown(ctx)
}
