package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// TestUserGroupQueriesAgainstRealDatabase 在**真库**上跑用户组的写路径与用户中心的媒介绑定读路径。
//
// 为什么需要：sqlmock 用例验证不了驱动层与方言差异（`:execlastid`/`:execresult` 在 PostgreSQL 上
// 取不回 LastInsertId 就是这么漏掉的），而用户组是"整表替换成员"的复合写（组 + 成员的先后顺序、
// 唯一约束冲突的文案翻译）——只有真库能验。做法与 internal/baseline/smoke_test.go 一致。
//
// 与 baseline 冒烟的唯一差别：这里调的是 handler/service 层方法（它们各自开事务，无法包在一层
// 事务里），所以创建的真实行在结束时显式清理，而不是靠回滚。
//
// 用法：
//
//	IDENTITY_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/identity/ -run RealDatabase -v
//	IDENTITY_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/identity/ -run RealDatabase -v
func TestUserGroupQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("IDENTITY_SMOKE_DSN")
	if dsn == "" {
		t.Skip("IDENTITY_SMOKE_DSN 未设置：跳过真库冒烟（说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, database.Configuration{
		MySQLDSN: dsn, PostgresDSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("connect(%s): %v", redactSmokeDSN(dsn), err)
	}
	defer pool.Close()
	queries := db.New(pool)
	handler := NewHandler(NewService(NewRepository(pool), nil))
	suffix := time.Now().UTC().Format("150405.000000")

	// 成员要用真实用户：建一个临时用户，结束时删掉（用户组级联删除成员行）。
	now := time.Now().UTC()
	createUserResult, err := queries.CreateUser(ctx, db.CreateUserParams{Username: "smoke-group-" + suffix,
		Password: "x", Status: 1, Timezone: "Asia/Shanghai",
		CreateTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: sql.NullTime{Time: now, Valid: true}})
	if err != nil {
		t.Fatalf("创建临时用户：%v", err)
	}
	userIDValue, err := createUserResult.LastInsertId()
	if err != nil {
		t.Fatalf("临时用户取主键（PG 侧说明 INSERT 未派生为 :one + RETURNING）：%v", err)
	}
	userID := int32(userIDValue)
	defer queries.DeleteUserByID(ctx, userID)

	// 建组（带成员）→ 整表替换语义。
	groupID, errMsg, err := handler.saveUserGroup(ctx, 0, &saveUserGroupInput{
		Name: "smoke-group-" + suffix, Remark: "冒烟", UserIDs: []int32{userID}})
	if err != nil || errMsg != "" {
		t.Fatalf("建组失败：err=%v msg=%s", err, errMsg)
	}
	defer queries.DeleteUserGroup(ctx, groupID)
	if groupID <= 0 {
		t.Fatalf("建组返回 id = %d，应为真实自增主键", groupID)
	}

	// 重名必须回业务文案（MySQL 1062 / PG 23505），而不是 500。
	if _, errMsg, err := handler.saveUserGroup(ctx, 0, &saveUserGroupInput{Name: "smoke-group-" + suffix}); err != nil || errMsg != "用户组名称已存在" {
		t.Fatalf("重名应回业务文案：err=%v msg=%q", err, errMsg)
	}

	// 列表（成员按 group_id 归组）。
	items, err := listUserGroups(ctx, pool)
	if err != nil {
		t.Fatalf("列表：%v", err)
	}
	found := false
	for _, item := range items {
		if item.ID == groupID {
			found = true
			if item.MemberCount != 1 || len(item.Members) != 1 || item.Members[0].UserID != userID || item.Members[0].Username == "" {
				t.Fatalf("成员未正确归组：%+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("列表里找不到刚建的组（id=%d）", groupID)
	}

	// 更新：改名 + 清空成员（整表替换）。
	if _, errMsg, err := handler.saveUserGroup(ctx, groupID, &saveUserGroupInput{
		Name: "smoke-group-renamed-" + suffix, Remark: "", UserIDs: []int32{}}); err != nil || errMsg != "" {
		t.Fatalf("更新失败：err=%v msg=%s", err, errMsg)
	}
	items, err = listUserGroups(ctx, pool)
	if err != nil {
		t.Fatalf("更新后列表：%v", err)
	}
	for _, item := range items {
		if item.ID == groupID {
			if item.Name != "smoke-group-renamed-"+suffix || item.MemberCount != 0 || len(item.Members) != 0 {
				t.Fatalf("整表替换成员未生效（改名/清空）：%+v", item)
			}
		}
	}
	if _, errMsg, err := handler.saveUserGroup(ctx, groupID+1_000_000, &saveUserGroupInput{Name: "smoke-x-" + suffix}); err != nil || errMsg != "用户组不存在" {
		t.Fatalf("改不存在的组应回业务文案：err=%v msg=%q", err, errMsg)
	}

	// 不存在的成员必须被拒绝。
	if _, errMsg, err := handler.saveUserGroup(ctx, 0, &saveUserGroupInput{
		Name: "smoke-group-bad-" + suffix, UserIDs: []int32{2_000_000_000}}); err != nil || !strings.Contains(errMsg, "不存在") {
		t.Fatalf("不存在的成员应被拒绝：err=%v msg=%q", err, errMsg)
	}

	// 批量删除（handler 级，走统一删除契约）。
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	body, _ := json.Marshal(map[string]any{"ids": []int64{groupID}})
	context.Request = httptest.NewRequest(http.MethodPost, "/sys/user-groups/batch-delete/", strings.NewReader(string(body)))
	context.Request.Header.Set("Content-Type", "application/json")
	handler.BatchDeleteUserGroups(context)
	var deleted struct {
		Data struct {
			Deleted int `json:"deleted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("解析批量删除响应 %q：%v", recorder.Body.String(), err)
	}
	if deleted.Data.Deleted != 1 {
		t.Fatalf("批量删除 deleted = %d，应为 1（body=%s）", deleted.Data.Deleted, recorder.Body.String())
	}

	// 用户中心：媒介绑定读路径 + 校验失败路径（媒介行由 monitor 域创建，这里不造数据）。
	if _, _, err := handler.service.ListAlertMediaBindings(ctx, userID); err != nil {
		t.Fatalf("列媒介绑定：%v", err)
	}
	if err := handler.service.ReplaceAlertMediaBindings(ctx, userID,
		[]validatedBinding{{MediaID: 2_000_000_000, Recipients: []string{"a@example.com"}, Enabled: true}}); err != sql.ErrNoRows {
		t.Fatalf("不存在的媒介应回 sql.ErrNoRows，实得 %v", err)
	}
}

// redactSmokeDSN 只保留主机与库名，避免把口令写进测试日志。
func redactSmokeDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return "***" + dsn[at:]
	}
	return dsn
}
