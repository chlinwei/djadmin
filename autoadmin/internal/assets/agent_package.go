package assets

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// dj-agent 安装包管理：上传、激活、批删与列表。上传的激活包优先用于 Agent 安装/更新
// （见 agent_update.go 的 loadAgentBinary 来源选择）；文件落盘在 autoadmin media 目录下的
// agent_packages/<version>/dj-agent，与 monitor 软件包共用同一个媒体根解析逻辑。
// 字节标记校验与构建产物一致：拒绝旧 RabbitMQ 版本、要求含 DJ_AGENT_GRPC_FILE_ADDR。

const (
	maxAgentPackageSize     = 200 << 20
	agentPackageRelativeDir = "agent_packages"
	// 单槽位"当前包"语义：存储目录与记录固定 default，不暴露版本概念。
	agentPackageSlot = "default"
)

type agentPackage struct {
	ID         int64
	File       string
	SHA256     string
	SizeBytes  int64
	IsActive   bool
	CreateTime sql.NullTime
}

// validateAgentBinary 与构建产物加载共用同一组字节标记校验。
func validateAgentBinary(data []byte) error {
	if bytes.Contains(data, []byte("connect rabbitmq failed")) {
		return fmt.Errorf("Agent 二进制仍是旧 RabbitMQ 版本，请先执行 CGO_ENABLED=0 go build -trimpath -o bin/dj-agent ./cmd/agent 后重试")
	}
	if !bytes.Contains(data, []byte("DJ_AGENT_GRPC_FILE_ADDR")) {
		return fmt.Errorf("Agent 二进制缺少当前 gRPC 配置标记，拒绝部署未知版本")
	}
	return nil
}

func (handler *Handler) ListAgentPackages(context *gin.Context) {
	// 单包语义：直接返回当前包对象（无包时 items 为 null），不是列表。
	item, err := db.New(handler.service.repository.pool).GetActiveAgentPackage(context)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		response.Error(context, err)
		return
	}
	data := gin.H{}
	if err == nil {
		data = gin.H{
			"id": item.ID, "file": item.File, "sha256": item.Sha256,
			"size_bytes": item.SizeBytes, "is_active": item.IsActive,
			"create_time": agentPackageCreateTime(item.CreateTime),
		}
	}
	response.Success(context, data)
}

func (handler *Handler) UploadAgentPackage(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, maxAgentPackageSize+(1<<20))
	upload, err := context.FormFile("file")
	if err != nil {
		response.BusinessError(context, 400, "file必填", nil)
		return
	}
	source, err := upload.Open()
	if err != nil {
		response.Error(context, err)
		return
	}
	defer source.Close()
	data, err := io.ReadAll(io.LimitReader(source, maxAgentPackageSize+1))
	if err != nil {
		response.BusinessError(context, 400, "读取上传文件失败", nil)
		return
	}
	if int64(len(data)) > maxAgentPackageSize {
		response.BusinessError(context, 400, "Agent 安装包超过 200 MiB 限制", nil)
		return
	}
	if err = validateAgentBinary(data); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	mediaRoot := handler.mediaRoot
	relativePath := filepath.ToSlash(filepath.Join(agentPackageRelativeDir, agentPackageSlot, "dj-agent"))
	targetPath := filepath.Join(mediaRoot, filepath.FromSlash(relativePath))
	if err = os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		response.Error(context, err)
		return
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	if err = os.WriteFile(targetPath, data, 0o755); err != nil {
		response.Error(context, err)
		return
	}

	// 重复上传覆盖文件并更新记录；上传成功即独占激活。
	pool := handler.service.repository.pool
	tx, err := pool.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	now := time.Now().UTC()
	txQueries := db.New(tx)
	var itemID uint64
	existingID, lookupErr := txQueries.GetAgentPackageIDByVersion(context, agentPackageSlot)
	switch {
	case errors.Is(lookupErr, sql.ErrNoRows):
		insertedID, insertErr := txQueries.CreateAgentPackage(context, db.CreateAgentPackageParams{
			Version: agentPackageSlot, File: relativePath, Sha256: sum,
			SizeBytes: int64(len(data)), CreateTime: sql.NullTime{Time: now, Valid: true},
		})
		if insertErr != nil {
			tx.Rollback()
			response.Error(context, insertErr)
			return
		}
		itemID = uint64(insertedID)
	case lookupErr != nil:
		tx.Rollback()
		response.Error(context, lookupErr)
		return
	default:
		if err = txQueries.UpdateAgentPackageFile(context, db.UpdateAgentPackageFileParams{
			File: relativePath, Sha256: sum, SizeBytes: int64(len(data)), ID: existingID,
		}); err != nil {
			tx.Rollback()
			response.Error(context, err)
			return
		}
		itemID = existingID
	}
	if err = txQueries.DeactivateOtherAgentPackages(context, itemID); err != nil {
		tx.Rollback()
		response.Error(context, err)
		return
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": itemID, "sha256": sum, "size_bytes": len(data), "is_active": true})
}

func (handler *Handler) DownloadAgentPackage(context *gin.Context) {
	pool := handler.service.repository.pool
	file, err := db.New(pool).GetActiveAgentPackageFile(context)
	if errors.Is(err, sql.ErrNoRows) {
		response.BusinessError(context, 404, "尚未上传 Agent 安装包", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	target := filepath.Join(handler.mediaRoot, filepath.FromSlash(file))
	// 路径限制在 mediaRoot 内，防目录穿越。
	relative, relErr := filepath.Rel(handler.mediaRoot, target)
	if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		response.Error(context, fmt.Errorf("package file path escapes media root"))
		return
	}
	if _, statErr := os.Stat(target); statErr != nil {
		response.BusinessError(context, 404, "Agent 安装包文件缺失，请重新上传", nil)
		return
	}
	context.Header("Content-Disposition", `attachment; filename="dj-agent"`)
	context.File(target)
}

func (handler *Handler) ActivateAgentPackage(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	pool := handler.service.repository.pool
	tx, err := pool.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	txQueries := db.New(tx)
	affected, err := txQueries.ActivateAgentPackage(context, uint64(id))
	if err != nil {
		tx.Rollback()
		response.Error(context, err)
		return
	}
	if affected == 0 {
		tx.Rollback()
		response.BusinessError(context, 404, "agent package not found", nil)
		return
	}
	if err = txQueries.DeactivateOtherAgentPackages(context, uint64(id)); err != nil {
		tx.Rollback()
		response.Error(context, err)
		return
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id, "is_active": true})
}

func (handler *Handler) BatchDeleteAgentPackages(context *gin.Context) {
	ids, ok := batchDeleteIDs(context)
	if !ok {
		return
	}
	results, okCount := batchDeleteAgentResults(ids, func(id int64) error {
		return handler.deleteAgentPackageByID(context, id)
	})
	response.Success(context, gin.H{"count": okCount, "results": results})
}

// batchDeleteAgentResults 逐 id 删除并汇总：不存在的 id（sql.ErrNoRows）记 "resource not found"，
// 其余失败只记录 message，不中断批次（项目批删 API 约定，见 docs/architecture/CONVENTIONS.md）。
func batchDeleteAgentResults(ids []int64, deleteOne func(id int64) error) ([]gin.H, int) {
	results := make([]gin.H, 0, len(ids))
	okCount := 0
	for _, id := range ids {
		if err := deleteOne(id); err != nil {
			message := err.Error()
			if errors.Is(err, sql.ErrNoRows) {
				message = "resource not found"
			}
			results = append(results, gin.H{"id": id, "ok": false, "message": message})
			continue
		}
		okCount++
		results = append(results, gin.H{"id": id, "ok": true, "message": ""})
	}
	return results, okCount
}

func (handler *Handler) deleteAgentPackageByID(context *gin.Context, id int64) error {
	queries := db.New(handler.service.repository.pool)
	item, err := queries.GetAgentPackage(context, uint64(id))
	if err != nil {
		return err
	}
	if err = handler.deleteAgentPackageFile(item.File); err != nil {
		return err
	}
	return queries.DeleteAgentPackage(context, uint64(id))
}

// deleteAgentPackageFile 只允许删除 mediaRoot 内的相对路径，防目录穿越。
func (handler *Handler) deleteAgentPackageFile(relativePath string) error {
	if strings.TrimSpace(relativePath) == "" {
		return nil
	}
	target := filepath.Join(handler.mediaRoot, filepath.FromSlash(relativePath))
	relative, err := filepath.Rel(handler.mediaRoot, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("package file path escapes media root")
	}
	if err = os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func agentPackageCreateTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.Format(time.RFC3339Nano)
}
