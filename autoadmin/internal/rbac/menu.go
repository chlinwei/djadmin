package rbac

import (
	"context"
	"database/sql"
	"sort"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

type Menu struct {
	ID         int32   `json:"id"`
	Name       string  `json:"name"`
	Icon       *string `json:"icon"`
	ParentID   *int32  `json:"parent_id"`
	OrderNum   *int32  `json:"order_num"`
	Path       *string `json:"path"`
	Component  *string `json:"component"`
	MenuType   *string `json:"menu_type"`
	IsExpanded bool    `json:"is_expanded"`
	Perms      *string `json:"perms"`
	CreateTime *string `json:"create_time"`
	UpdateTime *string `json:"update_time"`
	Remark     *string `json:"remark"`
	Location   int16   `json:"location"`
	Children   []Menu  `json:"children,omitempty"`
}
type MenuInput struct {
	Name       string
	Icon       *string
	ParentID   *int32
	OrderNum   *int32
	Path       *string
	Component  *string
	MenuType   *string
	Perms      *string
	Remark     *string
	Location   int16
	IsExpanded bool
}

func (service *Service) MenuTree(ctx context.Context) ([]Menu, error) {
	rows, err := service.repository.ListMenus(ctx)
	if err != nil {
		return nil, err
	}
	return menuTree(rows), nil
}
func (service *Service) GetMenu(ctx context.Context, id int32) (Menu, error) {
	row, err := service.repository.GetMenu(ctx, id)
	return mapMenu(row), err
}
func (service *Service) MenuIDsByRole(ctx context.Context, roleID int32) ([]int32, error) {
	return service.repository.MenuIDsByRole(ctx, roleID)
}
func (service *Service) GrantMenus(ctx context.Context, roleID int32, menuIDs []int32) error {
	if _, err := service.repository.GetRole(ctx, roleID); err != nil {
		return err
	}
	return service.repository.ReplaceRoleMenus(ctx, roleID, menuIDs)
}
func (service *Service) CreateMenu(ctx context.Context, input MenuInput) (Menu, error) {
	if err := service.validateParent(ctx, 0, input.ParentID); err != nil {
		return Menu{}, err
	}
	now := sql.NullTime{Time: time.Now().UTC(), Valid: true}
	row, err := service.repository.CreateMenu(ctx, db.CreateMenuParams{Name: input.Name, Icon: nullable(input.Icon), ParentID: nullInt(input.ParentID), OrderNum: nullInt(input.OrderNum), Path: nullable(input.Path), Component: nullable(input.Component), MenuType: nullable(input.MenuType), Perms: nullable(input.Perms), CreateTime: now, UpdateTime: now, Remark: nullable(input.Remark), Location: input.Location, IsExpanded: input.IsExpanded})
	return mapMenu(row), err
}
func (service *Service) UpdateMenu(ctx context.Context, id int32, input MenuInput) (Menu, error) {
	row, err := service.repository.GetMenu(ctx, id)
	if err != nil {
		return Menu{}, err
	}
	if err := service.validateParent(ctx, id, input.ParentID); err != nil {
		return Menu{}, err
	}
	if input.Name != "" {
		row.Name = input.Name
	}
	if input.Icon != nil {
		row.Icon = nullable(input.Icon)
	}
	if input.ParentID != nil {
		row.ParentID = nullInt(input.ParentID)
	}
	if input.OrderNum != nil {
		row.OrderNum = nullInt(input.OrderNum)
	}
	if input.Path != nil {
		row.Path = nullable(input.Path)
	}
	if input.Component != nil {
		row.Component = nullable(input.Component)
	}
	if input.MenuType != nil {
		row.MenuType = nullable(input.MenuType)
	}
	if input.Perms != nil {
		row.Perms = nullable(input.Perms)
	}
	if input.Remark != nil {
		row.Remark = nullable(input.Remark)
	}
	if input.Location != 0 {
		row.Location = input.Location
	}
	row.IsExpanded = input.IsExpanded
	err = service.repository.UpdateMenu(ctx, db.UpdateMenuParams{Name: row.Name, Icon: row.Icon, ParentID: row.ParentID, OrderNum: row.OrderNum, Path: row.Path, Component: row.Component, MenuType: row.MenuType, Perms: row.Perms, UpdateTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, Remark: row.Remark, Location: row.Location, IsExpanded: row.IsExpanded, ID: id})
	if err != nil {
		return Menu{}, err
	}
	return service.GetMenu(ctx, id)
}
func (service *Service) DeleteMenu(ctx context.Context, id int32) error {
	return service.repository.DeleteMenu(ctx, id)
}
func (service *Service) validateParent(ctx context.Context, id int32, parent *int32) error {
	if parent == nil || *parent == 0 {
		return nil
	}
	if *parent == id {
		return ErrMenuSelfParent
	}
	_, err := service.repository.GetMenu(ctx, *parent)
	return err
}
func menuTree(rows []db.SysMenu) []Menu {
	children := map[int32][]db.SysMenu{}
	for _, row := range rows {
		if row.ParentID.Valid {
			children[row.ParentID.Int32] = append(children[row.ParentID.Int32], row)
		}
	}
	for id := range children {
		sort.SliceStable(children[id], func(i, j int) bool {
			a, b := children[id][i], children[id][j]
			if a.OrderNum.Valid && b.OrderNum.Valid && a.OrderNum.Int32 != b.OrderNum.Int32 {
				return a.OrderNum.Int32 < b.OrderNum.Int32
			}
			return a.ID < b.ID
		})
	}
	var visit func(int32, map[int32]bool) []Menu
	visit = func(parent int32, seen map[int32]bool) []Menu {
		result := []Menu{}
		for _, row := range children[parent] {
			if seen[row.ID] {
				continue
			}
			next := map[int32]bool{}
			for key := range seen {
				next[key] = true
			}
			next[row.ID] = true
			item := mapMenu(row)
			item.Children = visit(row.ID, next)
			result = append(result, item)
		}
		return result
	}
	return visit(0, map[int32]bool{})
}
func mapMenu(row db.SysMenu) Menu {
	return Menu{ID: row.ID, Name: row.Name, Icon: stringPtr(row.Icon), ParentID: intPtr(row.ParentID), OrderNum: intPtr(row.OrderNum), Path: stringPtr(row.Path), Component: stringPtr(row.Component), MenuType: stringPtr(row.MenuType), IsExpanded: row.IsExpanded, Perms: stringPtr(row.Perms), CreateTime: timePtr(row.CreateTime), UpdateTime: timePtr(row.UpdateTime), Remark: stringPtr(row.Remark), Location: row.Location}
}
func nullInt(value *int32) sql.NullInt32 {
	if value == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *value, Valid: true}
}
func intPtr(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
