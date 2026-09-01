package sysconfig

import (
	"context"
	"database/sql"
	"fmt"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type Repository struct {
	queries *db.Queries
}

func NewRepository(pool *sql.DB) *Repository {
	return &Repository{queries: db.New(pool)}
}

func (repository *Repository) GetByKey(ctx context.Context, key string) (db.SysConfig, error) {
	return repository.queries.GetConfigByKey(ctx, key)
}

func (repository *Repository) GetByID(ctx context.Context, id int64) (db.SysConfig, error) {
	return repository.queries.GetConfigByID(ctx, id)
}
func (repository *Repository) Update(ctx context.Context, params db.UpdateConfigValueParams) error {
	return repository.queries.UpdateConfigValue(ctx, params)
}

func (repository *Repository) List(ctx context.Context, search string, page pagination.Page) ([]db.SysConfig, int64, error) {
	if search == "" {
		count, err := repository.queries.CountConfigs(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("count configs: %w", err)
		}
		configs, err := repository.queries.ListConfigs(ctx, db.ListConfigsParams{Limit: page.Size, Offset: page.Offset})
		return configs, count, err
	}
	pattern := "%" + search + "%"
	count, err := repository.queries.CountConfigsBySearch(ctx, db.CountConfigsBySearchParams{
		NamePattern: pattern, KeyPattern: pattern,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count searched configs: %w", err)
	}
	configs, err := repository.queries.SearchConfigs(ctx, db.SearchConfigsParams{
		NamePattern: pattern, KeyPattern: pattern, Limit: page.Size, Offset: page.Offset,
	})
	return configs, count, err
}
