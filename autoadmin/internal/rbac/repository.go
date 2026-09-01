package rbac

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

func (repository *Repository) ListRoles(ctx context.Context, search string, page pagination.Page) ([]db.SysRole, int64, error) {
	if search == "" {
		count, err := repository.queries.CountRoles(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("count roles: %w", err)
		}
		roles, err := repository.queries.ListRoles(ctx, db.ListRolesParams{Limit: page.Size, Offset: page.Offset})
		return roles, count, err
	}
	pattern := sql.NullString{String: "%" + search + "%", Valid: true}
	count, err := repository.queries.CountRolesBySearch(ctx, db.CountRolesBySearchParams{
		NamePattern: pattern, CodePattern: pattern, RemarkPattern: pattern,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count searched roles: %w", err)
	}
	roles, err := repository.queries.SearchRoles(ctx, db.SearchRolesParams{
		NamePattern: pattern, CodePattern: pattern, RemarkPattern: pattern,
		Limit: page.Size, Offset: page.Offset,
	})
	return roles, count, err
}

func (repository *Repository) ListMenus(ctx context.Context) ([]db.SysMenu, error) {
	return repository.queries.ListMenus(ctx)
}

func (repository *Repository) GetMenu(ctx context.Context, menuID int32) (db.SysMenu, error) {
	return repository.queries.GetMenuByID(ctx, menuID)
}

func (repository *Repository) MenuIDsByRole(ctx context.Context, roleID int32) ([]int32, error) {
	return repository.queries.ListMenuIDsByRoleID(ctx, roleID)
}

func (repository *Repository) CreateMenu(ctx context.Context, params db.CreateMenuParams) (db.SysMenu, error) {
	result, err := repository.queries.CreateMenu(ctx, params)
	if err != nil {
		return db.SysMenu{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return db.SysMenu{}, err
	}
	return repository.queries.GetMenuByID(ctx, int32(id))
}

func (repository *Repository) UpdateMenu(ctx context.Context, params db.UpdateMenuParams) error {
	return repository.queries.UpdateMenu(ctx, params)
}

func (repository *Repository) DeleteMenu(ctx context.Context, menuID int32) error {
	return database.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		if err := queries.DeleteMenuRoles(ctx, menuID); err != nil {
			return err
		}
		return queries.DeleteMenuByID(ctx, menuID)
	})
}

func (repository *Repository) GetRole(ctx context.Context, roleID int32) (db.SysRole, error) {
	return repository.queries.GetRoleByID(ctx, roleID)
}

func (repository *Repository) ListRolesByUser(ctx context.Context, userID int32) ([]db.SysRole, error) {
	return repository.queries.ListRolesByUserID(ctx, userID)
}

func (repository *Repository) CreateRole(ctx context.Context, params db.CreateRoleParams) (db.SysRole, error) {
	result, err := repository.queries.CreateRole(ctx, params)
	if err != nil {
		return db.SysRole{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return db.SysRole{}, err
	}
	return repository.queries.GetRoleByID(ctx, int32(id))
}

func (repository *Repository) UpdateRole(ctx context.Context, params db.UpdateRoleParams) error {
	return repository.queries.UpdateRole(ctx, params)
}

func (repository *Repository) DeleteRoles(ctx context.Context, roleIDs []int32) error {
	return database.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		for _, roleID := range uniqueIDs(roleIDs) {
			if err := queries.DeleteRoleUsers(ctx, roleID); err != nil {
				return err
			}
			if err := queries.DeleteRoleMenus(ctx, roleID); err != nil {
				return err
			}
			if err := queries.DeleteRoleByID(ctx, roleID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (repository *Repository) ReplaceRoleMenus(ctx context.Context, roleID int32, menuIDs []int32) error {
	return database.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		if err := queries.DeleteRoleMenus(ctx, roleID); err != nil {
			return fmt.Errorf("delete role menus: %w", err)
		}
		for _, menuID := range uniqueIDs(menuIDs) {
			if err := queries.AddRoleMenu(ctx, db.AddRoleMenuParams{RoleID: roleID, MenuID: menuID}); err != nil {
				return fmt.Errorf("add role menu %d: %w", menuID, err)
			}
		}
		return nil
	})
}

func (repository *Repository) DeleteRole(ctx context.Context, roleID int32) error {
	return database.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		if err := queries.DeleteRoleUsers(ctx, roleID); err != nil {
			return err
		}
		if err := queries.DeleteRoleMenus(ctx, roleID); err != nil {
			return err
		}
		return queries.DeleteRoleByID(ctx, roleID)
	})
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
