package identity

import (
	"context"
	"database/sql"
	"fmt"

	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type Repository struct {
	pool    *sql.DB
	queries *db.Queries
}

func NewRepository(pool *sql.DB) *Repository {
	return &Repository{pool: pool, queries: db.New(pool)}
}

func (repository *Repository) GetByID(ctx context.Context, id int32) (db.SysUser, error) {
	return repository.queries.GetUserByID(ctx, id)
}

func (repository *Repository) GetByUsername(ctx context.Context, username string) (db.SysUser, error) {
	return repository.queries.GetUserByUsername(ctx, username)
}

func (repository *Repository) ListMenus(ctx context.Context, userID int32) ([]db.SysMenu, error) {
	return repository.queries.ListMenusByUserID(ctx, userID)
}

func (repository *Repository) ListRoleCodes(ctx context.Context, userID int32) ([]sql.NullString, error) {
	return repository.queries.ListRoleCodesByUserID(ctx, userID)
}

func (repository *Repository) ListPermissionCodes(ctx context.Context, userID int32) ([]sql.NullString, error) {
	return repository.queries.ListPermissionCodesByUserID(ctx, userID)
}

func (repository *Repository) ListRoles(ctx context.Context, userID int32) ([]db.SysRole, error) {
	return repository.queries.ListRolesByUserID(ctx, userID)
}

func (repository *Repository) CreateLoginAudit(ctx context.Context, params db.CreateLoginAuditParams) error {
	return repository.queries.CreateLoginAudit(ctx, params)
}

func (repository *Repository) List(ctx context.Context, search string, page pagination.Page) ([]db.SysUser, int64, error) {
	if search == "" {
		count, err := repository.queries.CountUsers(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("count users: %w", err)
		}
		users, err := repository.queries.ListUsers(ctx, db.ListUsersParams{Limit: page.Size, Offset: page.Offset})
		if err != nil {
			return nil, 0, fmt.Errorf("list users: %w", err)
		}
		return users, count, nil
	}

	pattern := "%" + search + "%"
	params := db.CountUsersBySearchParams{
		UsernamePattern:    pattern,
		PhonenumberPattern: validString(pattern),
		RemarkPattern:      validString(pattern),
	}
	count, err := repository.queries.CountUsersBySearch(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("count searched users: %w", err)
	}
	users, err := repository.queries.SearchUsers(ctx, db.SearchUsersParams{
		UsernamePattern:    pattern,
		PhonenumberPattern: validString(pattern),
		RemarkPattern:      validString(pattern),
		Limit:              page.Size,
		Offset:             page.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search users: %w", err)
	}
	return users, count, nil
}

func (repository *Repository) ReplaceRoles(ctx context.Context, userID int32, roleIDs []int32) error {
	return database.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		if err := queries.DeleteUserRoles(ctx, userID); err != nil {
			return fmt.Errorf("delete user roles: %w", err)
		}
		for _, roleID := range uniqueIDs(roleIDs) {
			if err := queries.AddUserRole(ctx, db.AddUserRoleParams{UserID: userID, RoleID: roleID}); err != nil {
				return fmt.Errorf("add user role %d: %w", roleID, err)
			}
		}
		return nil
	})
}

func (repository *Repository) Create(ctx context.Context, params db.CreateUserParams) (db.SysUser, error) {
	result, err := repository.queries.CreateUser(ctx, params)
	if err != nil {
		return db.SysUser{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return db.SysUser{}, err
	}
	return repository.queries.GetUserByID(ctx, int32(id))
}

func (repository *Repository) Update(ctx context.Context, params db.UpdateUserProfileParams) error {
	return repository.queries.UpdateUserProfile(ctx, params)
}

func (repository *Repository) UpdatePassword(ctx context.Context, params db.UpdateUserPasswordParams) error {
	return repository.queries.UpdateUserPassword(ctx, params)
}

func (repository *Repository) DeleteMany(ctx context.Context, userIDs []int32) error {
	return database.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		for _, userID := range uniqueIDs(userIDs) {
			if err := queries.DeleteUserRoles(ctx, userID); err != nil {
				return err
			}
			if err := queries.DeleteUserByID(ctx, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

func validString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func uniqueIDs(ids []int32) []int32 {
	seen := make(map[int32]struct{}, len(ids))
	result := make([]int32, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (repository *Repository) Pool() *sql.DB {
	return repository.pool
}

func (repository *Repository) UpdatePhonenumber(ctx context.Context, params db.UpdateUserPhonenumberParams) error {
	return repository.queries.UpdateUserPhonenumber(ctx, params)
}
