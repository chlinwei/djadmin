package monitor

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	_ "github.com/go-sql-driver/mysql"
)

// 集成回归用例：纳管目标的 agent 在线守卫。依赖 MYSQL_DSN（无则跳过），
// 测试自建数据并在结束时清理，不影响开发库存量数据。
//
// 核心回归点：BatchCreateTargets 必须在 install_now=true 时立即派发安装
// （此前漏实现，纳管后目标永远停在 unknown）。gateway 为 nil（等价于全部离线），
// 派发应被在线守卫拦截并写入 install_status=failed + install_message。
func TestBatchCreateTargetsInstallNowOfflineGuard(t *testing.T) {
	pool := openMonitorTestDB(t)
	if pool == nil {
		return
	}
	handler := newMonitorTestHandler(t, pool)
	router := gin.New()
	router.POST("/test/targets/batch-create/", handler.BatchCreateTargets)

	hostID := createMonitorTestHost(t, pool)
	defer removeMonitorTestHost(t, pool, hostID)

	body := fmt.Sprintf(`{"host_ids":[%d],"exporter_type":"node_exporter","scrape_port":9100,"install_now":true}`, hostID)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/test/targets/batch-create/", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("http status = %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var targetID int64
	var installStatus, installMessage string
	if err := pool.QueryRow(`SELECT id,install_status,install_message FROM monitor_target WHERE host_id=? AND exporter_type='node_exporter'`, hostID).
		Scan(&targetID, &installStatus, &installMessage); err != nil {
		t.Fatalf("monitor_target row not created: %v", err)
	}
	defer removeMonitorTestTarget(t, pool, targetID)

	// 回归核心：install_now=true 必须真的触发派发（此前漏实现，目标停在 unknown）。
	// gateway=nil（全部离线）时派发被在线守卫拦截：install_status 落 failed 且原因可读；
	// 单条结果 ok=false 并说明原因，便于前端逐台展示。
	if installStatus != "failed" {
		t.Fatalf("install_status = %q, want failed (offline guard)", installStatus)
	}
	if installMessage == "" {
		t.Fatal("install_message should explain why dispatch failed")
	}
	var payload struct {
		Data struct {
			Results []struct {
				OK      bool   `json:"ok"`
				Message string `json:"message"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if len(payload.Data.Results) != 1 || payload.Data.Results[0].OK {
		t.Fatalf("offline host should report ok=false with reason, body=%s", recorder.Body.String())
	}
	if payload.Data.Results[0].Message == "" {
		t.Fatal("result message should explain the offline failure")
	}
}

// dispatchTargetServiceControl：目标主机无 agent（agent_id 为空）时必须拒绝服务控制。
func TestTargetServiceControlRejectsMissingAgent(t *testing.T) {
	pool := openMonitorTestDB(t)
	if pool == nil {
		return
	}
	handler := newMonitorTestHandler(t, pool)
	router := gin.New()
	router.POST("/test/targets/:id/start-service/", handler.StartTargetService)

	hostID := createMonitorTestHost(t, pool)
	defer removeMonitorTestHost(t, pool, hostID)

	targetID := createMonitorTestTarget(t, pool, hostID)
	defer removeMonitorTestTarget(t, pool, targetID)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/test/targets/%d/start-service/", targetID), nil)
	router.ServeHTTP(recorder, request)
	// 无 agent（离线）必须拒绝：业务码 400 + 可读原因，绝不能当作成功下发。
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if payload.Code != 400 {
		t.Fatalf("business code = %d, want 400 (body=%s)", payload.Code, recorder.Body.String())
	}
	if payload.Msg == "" {
		t.Fatal("error message should explain why the control is rejected")
	}
}

func openMonitorTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN not set; skip monitor integration test")
	}
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("cannot open db: %v", err)
	}
	if err = pool.Ping(); err != nil {
		t.Skipf("cannot reach db: %v", err)
	}
	return pool
}

func newMonitorTestHandler(t *testing.T, pool *sql.DB) *Handler {
	gin.SetMode(gin.TestMode)
	// gateway=nil 等价于所有 agent 离线；encryptionKey/djangoSecret 只在加解密路径用到，
	// 这里给一个合法 Fernet key 以通过构造校验。
	handler, err := NewHandler(pool, nil, nil, os.Getenv("ASSETS_CREDENTIAL_ENCRYPTION_KEY"), os.Getenv("DJANGO_SECRET_KEY"))
	if err != nil {
		t.Skipf("cannot construct handler (missing key config): %v", err)
	}
	return handler
}

func createMonitorTestHost(t *testing.T, pool *sql.DB) int64 {
	now := time.Now().UTC()
	result, err := pool.Exec(`INSERT INTO assets_host
		(create_time,update_time,remark,status,ip,is_deleted_in_cloud,instance_name,collect_status,collect_message,agent_online,webssh_default_username,webssh_login_users)
		VALUES(?,?,?,?,?,FALSE,'monitor-guard-test','','test host',FALSE,'root','')`,
		now, now, nil, "active", fmt.Sprintf("10.254.254.%d", now.Nanosecond()%200+2))
	if err != nil {
		t.Fatalf("create test host: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func removeMonitorTestHost(t *testing.T, pool *sql.DB, hostID int64) {
	_, _ = pool.Exec(`DELETE FROM monitor_target WHERE host_id=?`, hostID)
	_, _ = pool.Exec(`DELETE FROM monitor_log_collection_target WHERE host_id=?`, hostID)
	_, _ = pool.Exec(`DELETE FROM assets_host WHERE id=?`, hostID)
}

func createMonitorTestTarget(t *testing.T, pool *sql.DB, hostID int64) int64 {
	now := time.Now().UTC()
	result, err := pool.Exec(`INSERT INTO monitor_target
		(create_time,update_time,remark,host_id,exporter_type,scrape_port,managed_enabled,install_status,install_message,retry_count,last_scrape_status,labels,last_dispatch_manual)
		VALUES(?,?,NULL,?,'node_exporter',9100,TRUE,'success','',0,'unknown','{}',FALSE)`,
		now, now, hostID)
	if err != nil {
		t.Fatalf("create test target: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

func removeMonitorTestTarget(t *testing.T, pool *sql.DB, targetID int64) {
	_, _ = pool.Exec(`DELETE FROM monitor_target_install_history WHERE target_id=?`, targetID)
	_, _ = pool.Exec(`DELETE FROM monitor_target WHERE id=?`, targetID)
}
