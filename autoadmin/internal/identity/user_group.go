package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 用户组管理（/sys/user-groups）：独立于角色的"人群"实体。
// 角色管权限（sys_role_menu），用户组管接收/协作成员；后续通知接收组按用户组寻址。
// 成员为整表替换语义（create/update 带 user_ids 全量提交），删除仅保留批量接口。

type userGroupItem struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Remark      string            `json:"remark"`
	MemberCount int               `json:"member_count"`
	Members     []userGroupMember `json:"members"`
	CreateTime  time.Time         `json:"create_time"`
	UpdateTime  time.Time         `json:"update_time"`
}

type userGroupMember struct {
	UserID   int32  `json:"user_id"`
	Username string `json:"username"`
}

func (handler *Handler) pool() *sql.DB { return handler.service.repository.Pool() }

func (handler *Handler) ListUserGroups(context *gin.Context) {
	items, err := listUserGroups(context.Request.Context(), handler.pool())
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"items": items})
}

func listUserGroups(ctx context.Context, pool *sql.DB) ([]userGroupItem, error) {
	queries := db.New(pool)
	rows, err := queries.ListUserGroups(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]userGroupItem, 0, len(rows))
	indexByID := make(map[int64]int, len(rows))
	for _, row := range rows {
		indexByID[row.ID] = len(items)
		items = append(items, userGroupItem{ID: row.ID, Name: row.Name, Remark: row.Remark,
			Members: []userGroupMember{}, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime})
	}
	if len(items) == 0 {
		return items, nil
	}
	members, err := queries.ListUserGroupMembers(ctx)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		index, ok := indexByID[member.GroupID]
		if !ok {
			continue
		}
		items[index].Members = append(items[index].Members, userGroupMember{UserID: member.UserID, Username: member.Username})
	}
	for index := range items {
		items[index].MemberCount = len(items[index].Members)
	}
	return items, nil
}

type saveUserGroupInput struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Remark  string  `json:"remark"`
	UserIDs []int32 `json:"user_ids"`
}

func (handler *Handler) CreateUserGroup(context *gin.Context) {
	var input saveUserGroupInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	id, errMsg, err := handler.saveUserGroup(context.Request.Context(), 0, &input)
	if err != nil {
		response.Error(context, err)
		return
	}
	if errMsg != "" {
		response.BusinessError(context, 400, errMsg, nil)
		return
	}
	handler.respondUserGroup(context, id)
}

func (handler *Handler) UpdateUserGroup(context *gin.Context) {
	var input saveUserGroupInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if input.ID < 1 {
		response.BusinessError(context, 400, "id 必填", nil)
		return
	}
	_, errMsg, err := handler.saveUserGroup(context.Request.Context(), input.ID, &input)
	if err != nil {
		response.Error(context, err)
		return
	}
	if errMsg != "" {
		response.BusinessError(context, 400, errMsg, nil)
		return
	}
	handler.respondUserGroup(context, input.ID)
}

// saveUserGroup 创建/更新用户组（成员整表替换）；返回 (id, 用户可读错误文案, 内部错误)。
func (handler *Handler) saveUserGroup(ctx context.Context, id int64, input *saveUserGroupInput) (int64, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return 0, "name 不能为空", nil
	}
	// 成员去重，并校验用户存在。
	userIDs := make([]int32, 0, len(input.UserIDs))
	seen := map[int32]bool{}
	for _, userID := range input.UserIDs {
		if userID < 1 || seen[userID] {
			continue
		}
		seen[userID] = true
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
	queries := db.New(handler.pool())
	for _, userID := range userIDs {
		// 用户不存在即拒绝（原实现用 SELECT COUNT(*) 判存在，这里取行更精确）。
		if _, err := queries.GetUserByID(ctx, userID); err != nil {
			if err == sql.ErrNoRows {
				return 0, fmt.Sprintf("用户 %d 不存在", userID), nil
			}
			return 0, "", err
		}
	}

	now := time.Now().UTC()
	tx, err := handler.pool().BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	txQueries := db.New(tx)
	if id == 0 {
		result, execErr := txQueries.CreateUserGroup(ctx, db.CreateUserGroupParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(&input.Remark), Name: name,
		})
		if execErr != nil {
			if isDuplicateEntry(execErr) {
				return 0, "用户组名称已存在", nil
			}
			return 0, "", execErr
		}
		id, _ = result.LastInsertId()
	} else {
		// 更新用受影响行数判存在：id 不存在时命中 0 行（原实现先 COUNT(*) 再 UPDATE）。
		affected, execErr := txQueries.UpdateUserGroup(ctx, db.UpdateUserGroupParams{
			UpdateTime: now, Remark: nullableString(&input.Remark), Name: name, ID: id,
		})
		if execErr != nil {
			if isDuplicateEntry(execErr) {
				return 0, "用户组名称已存在", nil
			}
			return 0, "", execErr
		}
		if affected == 0 {
			return 0, "用户组不存在", nil
		}
	}
	if err = txQueries.DeleteUserGroupMembers(ctx, id); err != nil {
		return 0, "", err
	}
	for _, userID := range userIDs {
		if err = txQueries.CreateUserGroupMember(ctx, db.CreateUserGroupMemberParams{
			CreateTime: now, GroupID: id, UserID: userID,
		}); err != nil {
			return 0, "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, "", err
	}
	return id, "", nil
}

// isDuplicateEntry 判唯一约束冲突（用户组名重复要回业务文案而不是 500）：
// MySQL 是错误号 1062，PostgreSQL 是 SQLSTATE 23505（见 SQL_DESIGN §2.4）。
func isDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "Error 1062") {
		return true
	}
	var sqlState interface{ SQLState() string }
	if errors.As(err, &sqlState) && sqlState.SQLState() == "23505" {
		return true
	}
	return false
}

func (handler *Handler) respondUserGroup(context *gin.Context, id int64) {
	items, err := listUserGroups(context.Request.Context(), handler.pool())
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		if item.ID == id {
			response.Success(context, item)
			return
		}
	}
	response.BusinessError(context, 404, "用户组不存在", nil)
}

func (handler *Handler) BatchDeleteUserGroups(context *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	deleted := 0
	queries := db.New(handler.pool())
	for _, id := range input.IDs {
		if id < 1 {
			continue
		}
		affected, err := queries.DeleteUserGroup(context.Request.Context(), id)
		if err != nil {
			response.Error(context, err)
			return
		}
		if affected > 0 {
			deleted++
		}
	}
	response.Success(context, gin.H{"deleted": deleted})
}
