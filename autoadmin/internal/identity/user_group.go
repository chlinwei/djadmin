package identity

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

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

func listUserGroups(ctx context.Context, db *sql.DB) ([]userGroupItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,COALESCE(remark,''),create_time,update_time FROM sys_user_group ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]userGroupItem, 0)
	indexByID := map[int64]int{}
	for rows.Next() {
		item := userGroupItem{Members: []userGroupMember{}}
		if err = rows.Scan(&item.ID, &item.Name, &item.Remark, &item.CreateTime, &item.UpdateTime); err != nil {
			return nil, err
		}
		indexByID[item.ID] = len(items)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	memberRows, err := db.QueryContext(ctx, `SELECT m.group_id,m.user_id,u.username
FROM sys_user_group_member m JOIN sys_user u ON u.id=m.user_id ORDER BY m.group_id,m.id`)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()
	for memberRows.Next() {
		var groupID int64
		var member userGroupMember
		if err = memberRows.Scan(&groupID, &member.UserID, &member.Username); err != nil {
			return nil, err
		}
		index, ok := indexByID[groupID]
		if !ok {
			continue
		}
		items[index].Members = append(items[index].Members, member)
	}
	if err = memberRows.Err(); err != nil {
		return nil, err
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
	for _, userID := range userIDs {
		var count int
		if err := handler.pool().QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_user WHERE id=?`, userID).Scan(&count); err != nil {
			return 0, "", err
		}
		if count == 0 {
			return 0, fmt.Sprintf("用户 %d 不存在", userID), nil
		}
	}

	now := time.Now().UTC()
	tx, err := handler.pool().BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	if id == 0 {
		result, execErr := tx.ExecContext(ctx, `INSERT INTO sys_user_group(create_time,update_time,remark,name) VALUES(?,?,?,?)`, now, now, input.Remark, name)
		if execErr != nil {
			if isDuplicateEntry(execErr) {
				return 0, "用户组名称已存在", nil
			}
			return 0, "", execErr
		}
		id, _ = result.LastInsertId()
	} else {
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_user_group WHERE id=?`, id).Scan(&exists); err != nil {
			return 0, "", err
		}
		if exists == 0 {
			return 0, "用户组不存在", nil
		}
		result, execErr := tx.ExecContext(ctx, `UPDATE sys_user_group SET update_time=?,remark=?,name=? WHERE id=?`, now, input.Remark, name, id)
		if execErr != nil {
			if isDuplicateEntry(execErr) {
				return 0, "用户组名称已存在", nil
			}
			return 0, "", execErr
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return 0, "用户组不存在", nil
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sys_user_group_member WHERE group_id=?`, id); err != nil {
		return 0, "", err
	}
	for _, userID := range userIDs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO sys_user_group_member(create_time,group_id,user_id) VALUES(?,?,?)`, now, id, userID); err != nil {
			return 0, "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, "", err
	}
	return id, "", nil
}

func isDuplicateEntry(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Error 1062")
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
	for _, id := range input.IDs {
		if id < 1 {
			continue
		}
		result, err := handler.pool().ExecContext(context.Request.Context(), `DELETE FROM sys_user_group WHERE id=?`, id)
		if err != nil {
			response.Error(context, err)
			return
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			deleted++
		}
	}
	response.Success(context, gin.H{"deleted": deleted})
}
