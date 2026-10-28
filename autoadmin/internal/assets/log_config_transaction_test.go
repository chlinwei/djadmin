package assets

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"autoadmin/internal/shared/apperror"

	"github.com/go-sql-driver/mysql"
)

type logConfigTxProbe struct {
	options []driver.TxOptions
	err     error
}

func (probe *logConfigTxProbe) Connect(context.Context) (driver.Conn, error) { return probe, nil }
func (probe *logConfigTxProbe) Driver() driver.Driver                        { return probe }
func (probe *logConfigTxProbe) Open(string) (driver.Conn, error)             { return probe, nil }
func (probe *logConfigTxProbe) Close() error                                 { return nil }
func (probe *logConfigTxProbe) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (probe *logConfigTxProbe) Begin() (driver.Tx, error) {
	return nil, errors.New("transaction options were not passed to driver")
}
func (probe *logConfigTxProbe) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	probe.options = append(probe.options, options)
	return nil, probe.err
}

func TestLogConfigWritesRequireSerializableIsolation(t *testing.T) {
	ctx := context.Background()
	tier := int64(3)
	operations := map[string]func(*Repository) error{
		"service": func(r *Repository) error {
			_, err := r.SaveApplicationService(ctx, 0, ApplicationServiceInput{LogRetentionTier: &tier}, nil)
			return err
		},
		"template": func(r *Repository) error {
			_, err := r.SaveDeploymentTemplate(ctx, 1, DeploymentTemplateInput{})
			return err
		},
		"deployment": func(r *Repository) error {
			_, err := r.SaveApplicationDeployment(ctx, 1, ApplicationDeploymentInput{})
			return err
		},
		"log-override": func(r *Repository) error {
			return r.UpsertServiceLogOverride(ctx, 1, ServiceLogOverrideInput{})
		},
		"batch-log-overrides": func(r *Repository) error {
			return r.UpsertServiceLogOverrides(ctx, 1, nil)
		},
		"collection-switch": func(r *Repository) error {
			_, err := newTestService(t, r).SetServiceLogCollection(ctx, 1, true)
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			beginError := errors.New("stop after observing isolation")
			probe := &logConfigTxProbe{err: beginError}
			pool := sql.OpenDB(probe)
			defer pool.Close()
			if err := operation(NewRepository(pool)); !errors.Is(err, beginError) {
				t.Fatalf("begin error must propagate: %v", err)
			}
			if len(probe.options) != 1 || probe.options[0].Isolation != driver.IsolationLevel(sql.LevelSerializable) ||
				probe.options[0].ReadOnly {
				t.Fatalf("write must begin a serializable read/write transaction: %+v", probe.options)
			}
		})
	}
}

func TestTranslateConcurrentConfigWriteErrors(t *testing.T) {
	for _, cause := range []error{
		&mysql.MySQLError{Number: 1205, Message: "lock wait timeout"},
		&mysql.MySQLError{Number: 1213, Message: "deadlock"},
		pgTestError{state: "40001", message: "serialization failure"},
		pgTestError{state: "40P01", message: "deadlock"},
	} {
		err := translate(fmt.Errorf("validate log configuration: %w", cause))
		var businessError *apperror.Error
		if !errors.As(err, &businessError) || businessError.Code() != apperror.CodeInvalidArgument ||
			businessError.Message() != ErrConcurrentWrite.Error() || !errors.Is(err, cause) {
			t.Errorf("concurrency failure must be explicit and preserve its cause: %v", err)
		}
	}
}
