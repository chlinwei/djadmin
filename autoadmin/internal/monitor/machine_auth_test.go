package monitor

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

const activeTokenQuery = `SELECT id,token_hash FROM sys_agent_token WHERE is_active=TRUE AND (expires_at IS NULL OR expires_at>UTC_TIMESTAMP(6))`

func TestMachineAuthenticateExcludesExpiredAgentTokens(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(regexp.QuoteMeta(activeTokenQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "token_hash"}))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.GET("/machine", handler.MachineAuthenticate(), func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/machine", nil)
	request.Header.Set("Authorization", "expired-agent-token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestMachineAuthenticateAcceptsValidBearerTokenAndTracksUsage(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	encoded, err := identity.HashPassword("valid-agent-token")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(activeTokenQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "token_hash"}).AddRow(42, encoded))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_agent_token SET last_used_at=? WHERE id=?`)).
		WithArgs(sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	recorder := serveMachineRequest(database, "Bearer valid-agent-token")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestMachineAuthenticateRejectsInvalidTokenWithoutTrackingUsage(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	encoded, err := identity.HashPassword("valid-agent-token")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(activeTokenQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "token_hash"}).AddRow(42, encoded))

	recorder := serveMachineRequest(database, "wrong-agent-token")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func TestMachineAuthenticateReturnsServerErrorWhenTokenQueryFails(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer database.Close()

	mock.ExpectQuery(regexp.QuoteMeta(activeTokenQuery)).WillReturnError(errors.New("database unavailable"))

	recorder := serveMachineRequest(database, "agent-token")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations: %v", err)
	}
}

func serveMachineRequest(database *sql.DB, authorization string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := &Handler{db: database}
	engine.GET("/machine", handler.MachineAuthenticate(), func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/machine", nil)
	request.Header.Set("Authorization", authorization)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}
