package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type CurrentUser struct {
	ID          int32   `json:"id"`
	Username    string  `json:"username"`
	Avatar      *string `json:"avatar"`
	Phonenumber *string `json:"phonenumber"`
	LoginDate   *string `json:"login_date"`
	Status      int16   `json:"status"`
	CreateTime  *string `json:"create_time"`
	UpdateTime  *string `json:"update_time"`
	Remark      *string `json:"remark"`
	Timezone    string  `json:"timezone"`
}

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

type LoginResult struct {
	CurrentUser CurrentUser `json:"currentUser"`
	Token       string      `json:"token"`
	MenuList    []Menu      `json:"menuList"`
	RoleCodes   []string    `json:"role_codes"`
}

type Role struct {
	ID         int32   `json:"id"`
	Name       *string `json:"name"`
	Code       *string `json:"code"`
	CreateTime *string `json:"create_time"`
	UpdateTime *string `json:"update_time"`
	Remark     *string `json:"remark"`
}

type UserListItem struct {
	CurrentUser
	Roles []Role `json:"roles"`
}

type Service struct {
	repository *Repository
	tokens     *TokenManager
}

func NewService(repository *Repository, tokens *TokenManager) *Service {
	return &Service{repository: repository, tokens: tokens}
}

func (service *Service) Login(ctx context.Context, username string, password string, clientIP string, userAgent string) (LoginResult, error) {
	user, err := service.repository.GetByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !VerifyPassword(user.Password, password)) {
		service.writeAudit(ctx, username, nil, "failed", clientIP, userAgent, ErrInvalidCredentials.Error())
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("get login user: %w", err)
	}
	if user.Status == 0 {
		service.writeAudit(ctx, username, &user.ID, "failed", clientIP, userAgent, ErrUserDisabled.Error())
		return LoginResult{}, ErrUserDisabled
	}

	menus, err := service.repository.ListMenus(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("list login menus: %w", err)
	}
	roleCodes, err := service.repository.ListRoleCodes(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("list login role codes: %w", err)
	}
	permissions, err := service.repository.ListPermissionCodes(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("list login permissions: %w", err)
	}
	permissionValues := validStrings(permissions)
	token, err := service.tokens.Issue(user.ID, user.Username, permissionValues)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue login token: %w", err)
	}
	service.writeAudit(ctx, user.Username, &user.ID, "success", clientIP, userAgent, "登录成功")

	return LoginResult{
		CurrentUser: mapUser(user),
		Token:       token,
		MenuList:    buildMenuTree(menus),
		RoleCodes:   validStrings(roleCodes),
	}, nil
}

func (service *Service) Current(ctx context.Context, userID int32) (CurrentUser, error) {
	user, err := service.repository.GetByID(ctx, userID)
	if err != nil {
		return CurrentUser{}, err
	}
	return mapUser(user), nil
}

func (service *Service) ListUsers(ctx context.Context, search string, page pagination.Page) ([]UserListItem, int64, error) {
	users, count, err := service.repository.List(ctx, search, page)
	if err != nil {
		return nil, 0, err
	}
	result := make([]UserListItem, 0, len(users))
	for _, user := range users {
		roles, err := service.repository.ListRoles(ctx, user.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("list roles for user %d: %w", user.ID, err)
		}
		item := UserListItem{CurrentUser: mapUser(user), Roles: make([]Role, 0, len(roles))}
		for _, role := range roles {
			item.Roles = append(item.Roles, mapRole(role))
		}
		result = append(result, item)
	}
	return result, count, nil
}

func (service *Service) writeAudit(ctx context.Context, username string, userID *int32, status string, clientIP string, userAgent string, message string) {
	params := db.CreateLoginAuditParams{
		Username: username, Status: status, ClientIp: clientIP,
		UserAgent: userAgent, Message: message, LoginTime: time.Now().UTC(),
	}
	if userID != nil {
		params.UserID = sql.NullInt32{Int32: *userID, Valid: true}
	}
	_ = service.repository.CreateLoginAudit(ctx, params)
}

func buildMenuTree(rows []db.SysMenu) []Menu {
	byParent := make(map[int32][]db.SysMenu)
	for _, row := range rows {
		if row.ParentID.Valid {
			byParent[row.ParentID.Int32] = append(byParent[row.ParentID.Int32], row)
		}
	}
	for parentID := range byParent {
		sort.SliceStable(byParent[parentID], func(left, right int) bool {
			leftOrder, rightOrder := byParent[parentID][left].OrderNum, byParent[parentID][right].OrderNum
			if leftOrder.Valid && rightOrder.Valid && leftOrder.Int32 != rightOrder.Int32 {
				return leftOrder.Int32 < rightOrder.Int32
			}
			return byParent[parentID][left].ID < byParent[parentID][right].ID
		})
	}
	var visit func(int32, map[int32]bool) []Menu
	visit = func(parentID int32, ancestors map[int32]bool) []Menu {
		result := make([]Menu, 0, len(byParent[parentID]))
		for _, row := range byParent[parentID] {
			if ancestors[row.ID] {
				continue
			}
			nextAncestors := make(map[int32]bool, len(ancestors)+1)
			for id := range ancestors {
				nextAncestors[id] = true
			}
			nextAncestors[row.ID] = true
			menu := mapMenu(row)
			menu.Children = visit(row.ID, nextAncestors)
			result = append(result, menu)
		}
		return result
	}
	return visit(0, map[int32]bool{})
}

func mapUser(user db.SysUser) CurrentUser {
	return CurrentUser{
		ID: user.ID, Username: user.Username, Avatar: nullString(user.Avatar),
		Phonenumber: nullString(user.Phonenumber), LoginDate: dateTimeString(user.LoginDate),
		Status: user.Status, CreateTime: dateString(user.CreateTime), UpdateTime: dateString(user.UpdateTime),
		Remark: nullString(user.Remark), Timezone: user.Timezone,
	}
}

func mapMenu(row db.SysMenu) Menu {
	return Menu{
		ID: row.ID, Name: row.Name, Icon: nullString(row.Icon), ParentID: nullInt32(row.ParentID),
		OrderNum: nullInt32(row.OrderNum), Path: nullString(row.Path), Component: nullString(row.Component),
		MenuType: nullString(row.MenuType), IsExpanded: row.IsExpanded, Perms: nullString(row.Perms),
		CreateTime: dateString(row.CreateTime), UpdateTime: dateString(row.UpdateTime),
		Remark: nullString(row.Remark), Location: row.Location,
	}
}

func mapRole(role db.SysRole) Role {
	return Role{
		ID: role.ID, Name: nullString(role.Name), Code: nullString(role.Code),
		CreateTime: dateString(role.CreateTime), UpdateTime: dateString(role.UpdateTime),
		Remark: nullString(role.Remark),
	}
}

func validStrings(values []sql.NullString) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value.Valid {
			result = append(result, value.String)
		}
	}
	return result
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullInt32(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}

func dateString(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.DateOnly)
	return &formatted
}

func dateTimeString(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format("2006-01-02T15:04:05.999999Z")
	return &formatted
}
