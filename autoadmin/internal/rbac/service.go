package rbac

import (
	"context"
	"database/sql"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type Role struct {
	ID         int32   `json:"id"`
	Name       *string `json:"name"`
	Code       *string `json:"code"`
	CreateTime *string `json:"create_time"`
	UpdateTime *string `json:"update_time"`
	Remark     *string `json:"remark"`
}
type RoleInput struct{ Name, Code, Remark *string }
type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

func (service *Service) List(ctx context.Context, search string, page pagination.Page) ([]Role, int64, error) {
	rows, count, err := service.repository.ListRoles(ctx, search, page)
	return mapRoles(rows), count, err
}
func (service *Service) Get(ctx context.Context, id int32) (Role, error) {
	row, err := service.repository.GetRole(ctx, id)
	return mapRole(row), err
}
func (service *Service) CurrentUserRoles(ctx context.Context, userID int32) ([]Role, error) {
	rows, err := service.repository.ListRolesByUser(ctx, userID)
	return mapRoles(rows), err
}
func (service *Service) Create(ctx context.Context, input RoleInput) (Role, error) {
	now := sql.NullTime{Time: time.Now().UTC(), Valid: true}
	row, err := service.repository.CreateRole(ctx, db.CreateRoleParams{Name: nullable(input.Name), Code: nullable(input.Code), Remark: nullable(input.Remark), CreateTime: now, UpdateTime: now})
	return mapRole(row), err
}
func (service *Service) Update(ctx context.Context, id int32, input RoleInput) (Role, error) {
	row, err := service.repository.GetRole(ctx, id)
	if err != nil {
		return Role{}, err
	}
	if input.Name != nil {
		row.Name = nullable(input.Name)
	}
	if input.Code != nil {
		row.Code = nullable(input.Code)
	}
	if input.Remark != nil {
		row.Remark = nullable(input.Remark)
	}
	err = service.repository.UpdateRole(ctx, db.UpdateRoleParams{Name: row.Name, Code: row.Code, Remark: row.Remark, UpdateTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, ID: id})
	if err != nil {
		return Role{}, err
	}
	return service.Get(ctx, id)
}
func mapRoles(rows []db.SysRole) []Role {
	result := make([]Role, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapRole(row))
	}
	return result
}
func mapRole(row db.SysRole) Role {
	return Role{ID: row.ID, Name: stringPtr(row.Name), Code: stringPtr(row.Code), CreateTime: timePtr(row.CreateTime), UpdateTime: timePtr(row.UpdateTime), Remark: stringPtr(row.Remark)}
}
func nullable(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
func timePtr(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.DateOnly)
	return &formatted
}
