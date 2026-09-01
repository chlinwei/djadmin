package assets

import (
	"context"
	"database/sql"
	"fmt"

	platformdb "autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type Repository struct {
	pool    *sql.DB
	queries *db.Queries
}

func NewRepository(pool *sql.DB) *Repository { return &Repository{pool: pool, queries: db.New(pool)} }

func (r *Repository) ListProjects(ctx context.Context, search string, page pagination.Page) ([]db.AssetsProject, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountProjects(ctx, db.CountProjectsParams{Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListProjects(ctx, db.ListProjectsParams{Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetProject(ctx context.Context, id int64) (db.AssetsProject, error) {
	return r.queries.GetProject(ctx, id)
}
func (r *Repository) CreateProject(ctx context.Context, p db.CreateProjectParams) (int64, error) {
	result, err := r.queries.CreateProject(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateProject(ctx context.Context, p db.UpdateProjectParams) error {
	return r.queries.UpdateProject(ctx, p)
}
func (r *Repository) DeleteProject(ctx context.Context, id int64) error {
	count, err := r.queries.CountBusinessSystemsByProject(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrDeleteProtected
	}
	return r.queries.DeleteProject(ctx, id)
}

func (r *Repository) ListBusinessSystems(ctx context.Context, search string, page pagination.Page) ([]db.ListBusinessSystemsRow, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountBusinessSystems(ctx, db.CountBusinessSystemsParams{Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListBusinessSystems(ctx, db.ListBusinessSystemsParams{Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetBusinessSystem(ctx context.Context, id int64) (db.GetBusinessSystemRow, error) {
	return r.queries.GetBusinessSystem(ctx, id)
}
func (r *Repository) CreateBusinessSystem(ctx context.Context, p db.CreateBusinessSystemParams) (int64, error) {
	result, err := r.queries.CreateBusinessSystem(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateBusinessSystem(ctx context.Context, p db.UpdateBusinessSystemParams) error {
	return r.queries.UpdateBusinessSystem(ctx, p)
}
func (r *Repository) DeleteBusinessSystem(ctx context.Context, id int64) error {
	return r.queries.DeleteBusinessSystem(ctx, id)
}

func (r *Repository) ListEnvironments(ctx context.Context, search string, page pagination.Page) ([]db.AssetsBusinessEnvironment, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountBusinessEnvironments(ctx, db.CountBusinessEnvironmentsParams{Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListBusinessEnvironments(ctx, db.ListBusinessEnvironmentsParams{Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetEnvironment(ctx context.Context, id int64) (db.AssetsBusinessEnvironment, error) {
	return r.queries.GetBusinessEnvironment(ctx, id)
}
func (r *Repository) CreateEnvironment(ctx context.Context, p db.CreateBusinessEnvironmentParams) (int64, error) {
	result, err := r.queries.CreateBusinessEnvironment(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateEnvironment(ctx context.Context, p db.UpdateBusinessEnvironmentParams) error {
	return r.queries.UpdateBusinessEnvironment(ctx, p)
}
func (r *Repository) DeleteEnvironment(ctx context.Context, id int64) error {
	count, err := r.queries.CountHostsByEnvironment(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrDeleteProtected
	}
	return r.queries.DeleteBusinessEnvironment(ctx, id)
}

func (r *Repository) ListCredentials(ctx context.Context, search string, page pagination.Page) ([]db.AssetsCredential, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountCredentials(ctx, db.CountCredentialsParams{Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListCredentials(ctx, db.ListCredentialsParams{Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetCredential(ctx context.Context, id int64) (db.AssetsCredential, error) {
	return r.queries.GetCredential(ctx, id)
}
func (r *Repository) CreateCredential(ctx context.Context, p db.CreateCredentialParams) (int64, error) {
	result, err := r.queries.CreateCredential(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateCredential(ctx context.Context, p db.UpdateCredentialParams) error {
	return r.queries.UpdateCredential(ctx, p)
}
func (r *Repository) DeleteCredential(ctx context.Context, id int64) error {
	count, err := r.queries.CountHostCredentialsByCredential(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrDeleteProtected
	}
	return r.queries.DeleteCredential(ctx, id)
}

func (r *Repository) ListHostGroups(ctx context.Context, search string, page pagination.Page) ([]db.ListHostGroupsRow, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountHostGroups(ctx, db.CountHostGroupsParams{Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListHostGroups(ctx, db.ListHostGroupsParams{Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetHostGroup(ctx context.Context, id int64) (db.GetHostGroupRow, error) {
	return r.queries.GetHostGroup(ctx, id)
}
func (r *Repository) ListAllHostGroups(ctx context.Context) ([]db.ListAllHostGroupsRow, error) {
	return r.queries.ListAllHostGroups(ctx)
}
func (r *Repository) CreateHostGroup(ctx context.Context, p db.CreateHostGroupParams) (int64, error) {
	result, err := r.queries.CreateHostGroup(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateHostGroup(ctx context.Context, p db.UpdateHostGroupParams) error {
	return r.queries.UpdateHostGroup(ctx, p)
}
func (r *Repository) DeleteHostGroup(ctx context.Context, id int64) error {
	value := sql.NullInt64{Int64: id, Valid: true}
	children, err := r.queries.CountChildHostGroups(ctx, value)
	if err != nil {
		return err
	}
	hosts, err := r.queries.CountHostsByGroup(ctx, value)
	if err != nil {
		return err
	}
	if children > 0 || hosts > 0 {
		return ErrDeleteProtected
	}
	return r.queries.DeleteHostGroup(ctx, id)
}

func (r *Repository) ListHosts(ctx context.Context, search string, groupID, environmentID int64, page pagination.Page) ([]db.ListHostsRow, int64, error) {
	params := db.CountHostsParams{Pattern: pattern(search), GroupID: sql.NullInt64{Int64: groupID, Valid: true}, EnvironmentID: sql.NullInt64{Int64: environmentID, Valid: true}}
	count, err := r.queries.CountHosts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListHosts(ctx, db.ListHostsParams{Pattern: params.Pattern, GroupID: params.GroupID, EnvironmentID: params.EnvironmentID, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetHost(ctx context.Context, id int64) (db.GetHostRow, error) {
	return r.queries.GetHost(ctx, id)
}
func (r *Repository) CreateHost(ctx context.Context, p db.CreateHostParams) (int64, error) {
	result, err := r.queries.CreateHost(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateHost(ctx context.Context, p db.UpdateHostParams) error {
	return r.queries.UpdateHost(ctx, p)
}
func (r *Repository) DeleteHost(ctx context.Context, id int64) error {
	return platformdb.InTransaction(ctx, r.pool, func(queries *db.Queries) error {
		if _, err := queries.GetHost(ctx, id); err != nil {
			return err
		}
		if err := queries.DeleteHost(ctx, id); err != nil {
			return fmt.Errorf("delete host: %w", err)
		}
		return nil
	})
}

func (r *Repository) DeleteCredentials(ctx context.Context, ids []int64) error {
	return platformdb.InTransaction(ctx, r.pool, func(queries *db.Queries) error {
		for _, id := range ids {
			count, err := queries.CountHostCredentialsByCredential(ctx, id)
			if err != nil {
				return err
			}
			if count > 0 {
				return ErrDeleteProtected
			}
			if err := queries.DeleteCredential(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) DeleteHosts(ctx context.Context, ids []int64) error {
	return platformdb.InTransaction(ctx, r.pool, func(queries *db.Queries) error {
		for _, id := range ids {
			if _, err := queries.GetHost(ctx, id); err != nil {
				return err
			}
			if err := queries.DeleteHost(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}
