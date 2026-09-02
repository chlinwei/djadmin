package monitor

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 软件包"自动更新"（POST /monitor/packages/:id/sync-official/），对应 Django sync_from_official。
// 按官方 GitHub release 命名规则拼接下载地址，下载 tarball 覆盖当前包并更新 version/sha256/size_bytes。
// 仅支持 node_exporter（当前唯一的本地仓库品类）；超时 60s 与前端放宽后的请求超时一致。

var nodeExporterVersionPattern = regexp.MustCompile(`^\d+(\.\d+){1,3}$`)

func buildNodeExporterOfficialURL(version, osName, arch string) string {
	tarball := fmt.Sprintf("node_exporter-%s.%s-%s.tar.gz", version, osName, arch)
	return fmt.Sprintf("https://github.com/prometheus/node_exporter/releases/download/v%s/%s", version, tarball)
}

func (handler *Handler) SyncSoftwarePackageFromOfficial(context *gin.Context) {
	item, err := handler.loadSoftwarePackage(context, parseID(context.Param("id")))
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "software package not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if item.Name != "node_exporter" {
		response.BusinessError(context, 400, "当前仅支持 node_exporter 自动更新", nil)
		return
	}
	var input struct {
		Version string `json:"version"`
	}
	_ = context.ShouldBindJSON(&input)
	targetVersion := strings.TrimLeft(strings.TrimSpace(input.Version), "v")
	if targetVersion == "" {
		targetVersion = strings.TrimLeft(strings.TrimSpace(item.Version), "v")
	}
	if !nodeExporterVersionPattern.MatchString(targetVersion) {
		response.BusinessError(context, 400, "版本号格式不正确，应类似 1.8.2", nil)
		return
	}
	// 目标版本若已被同名 os/arch 的其他记录占用，提前拦截，避免落库时触发唯一约束报错。
	var conflict int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM monitor_software_package
		WHERE name=? AND version=? AND os=? AND arch=? AND platform_family=? AND platform_major=? AND id<>?`,
		item.Name, targetVersion, item.OS, item.Arch, item.PlatformFamily, item.PlatformMajor, item.ID).Scan(&conflict); err != nil {
		response.Error(context, err)
		return
	}
	if conflict > 0 {
		response.BusinessError(context, 400, fmt.Sprintf("版本 %s 已存在相同平台记录，请先删除或更换版本", targetVersion), nil)
		return
	}

	tarballName := fmt.Sprintf("node_exporter-%s.%s-%s.tar.gz", targetVersion, item.OS, item.Arch)
	relativePath, err := softwarePackageRelativePath(item, tarballName)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	checksum, size, err := handler.downloadOfficialPackage(context, buildNodeExporterOfficialURL(targetVersion, item.OS, item.Arch), relativePath)
	if err != nil {
		response.BusinessError(context, 400, "下载官方软件包失败："+err.Error(), nil)
		return
	}
	if item.File != "" && item.File != relativePath {
		_ = handler.deleteSoftwarePackageFile(item.File)
	}
	if _, err = handler.db.ExecContext(context, `UPDATE monitor_software_package SET version=?,file=?,sha256=?,size_bytes=?,update_time=? WHERE id=?`,
		targetVersion, relativePath, checksum, size, time.Now().UTC(), item.ID); err != nil {
		response.Error(context, err)
		return
	}
	handler.respondSoftwarePackage(context, item.ID)
}

// downloadOfficialPackage 流式下载到包目录下的临时文件，校验和边下边算，超过 200MiB 中止，原子改名覆盖。
// 返回 (sha256hex, size, error)。
func (handler *Handler) downloadOfficialPackage(context *gin.Context, downloadURL, relativePath string) (string, int64, error) {
	request, err := http.NewRequestWithContext(context, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", 0, err
	}
	request.Header.Set("User-Agent", "djadmin-monitor-sync/1.0")
	// 独立客户端：包体可达百兆，默认 handler.client（8s）不够，60s 与前端请求超时对齐。
	client := &http.Client{Timeout: 60 * time.Second}
	upstream, err := client.Do(request)
	if err != nil {
		return "", 0, err
	}
	defer upstream.Body.Close()
	if upstream.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("HTTP %s", upstream.Status)
	}
	targetPath := filepath.Join(handler.packageRoot, filepath.FromSlash(relativePath))
	if err = os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", 0, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(targetPath), ".package-*")
	if err != nil {
		return "", 0, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(upstream.Body, maxSoftwarePackageSize+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		return "", 0, fmt.Errorf("写入本地仓库失败")
	}
	if written > maxSoftwarePackageSize {
		return "", 0, fmt.Errorf("软件包体积超出限制")
	}
	if err = os.Rename(temporaryPath, targetPath); err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), written, nil
}
