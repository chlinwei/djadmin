package monitor

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

const maxSoftwarePackageSize = 200 << 20

func (handler *Handler) UploadSoftwarePackage(context *gin.Context) {
	item, err := handler.loadSoftwarePackage(context, parseID(context.Param("id")))
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "software package not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, maxSoftwarePackageSize+(1<<20))
	upload, err := context.FormFile("file")
	if err != nil {
		response.BusinessError(context, 400, "file is required", nil)
		return
	}
	fileName := filepath.Base(strings.TrimSpace(upload.Filename))
	expectedSuffix := map[string]string{"tar.gz": ".tar.gz", "rpm": ".rpm", "deb": ".deb"}[item.PackageFormat]
	if fileName == "." || fileName == "" || expectedSuffix == "" || !strings.HasSuffix(strings.ToLower(fileName), expectedSuffix) {
		response.BusinessError(context, 400, "uploaded file format does not match the package record", nil)
		return
	}
	relativePath, err := softwarePackageRelativePath(item, fileName)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	targetPath := filepath.Join(handler.packageRoot, filepath.FromSlash(relativePath))
	if err = os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		response.Error(context, err)
		return
	}
	source, err := upload.Open()
	if err != nil {
		response.Error(context, err)
		return
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(targetPath), ".package-*")
	if err != nil {
		response.Error(context, err)
		return
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(source, maxSoftwarePackageSize+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		response.BusinessError(context, 400, "failed to store uploaded package", nil)
		return
	}
	if written > maxSoftwarePackageSize {
		response.BusinessError(context, 400, "software package exceeds 200 MiB", nil)
		return
	}
	if err = os.Rename(temporaryPath, targetPath); err != nil {
		response.Error(context, err)
		return
	}
	if item.File != "" && item.File != relativePath {
		_ = handler.deleteSoftwarePackageFile(item.File)
	}
	_, err = handler.db.ExecContext(context, `UPDATE monitor_software_package SET file=?,sha256=?,size_bytes=?,update_time=? WHERE id=?`, relativePath, fmt.Sprintf("%x", hasher.Sum(nil)), written, time.Now().UTC(), item.ID)
	if err != nil {
		response.Error(context, err)
		return
	}
	handler.respondSoftwarePackage(context, item.ID)
}

func softwarePackageRelativePath(item softwarePackage, fileName string) (string, error) {
	packageDirectory := item.Name
	if item.PackageType == "fluent_bit" {
		packageDirectory = "fluentBit"
	}
	if packageDirectory == "" || filepath.Base(packageDirectory) != packageDirectory || filepath.Base(item.Arch) != item.Arch {
		return "", fmt.Errorf("invalid package path metadata")
	}
	platformDirectory := item.OS
	if item.PlatformFamily != "any" {
		platformDirectory = item.PlatformFamily + item.PlatformMajor
	}
	if platformDirectory == "" || filepath.Base(platformDirectory) != platformDirectory {
		return "", fmt.Errorf("invalid package platform metadata")
	}
	return filepath.ToSlash(filepath.Join("monitor_packages", packageDirectory, item.Arch, platformDirectory, fileName)), nil
}

func (handler *Handler) deleteSoftwarePackageFile(relativePath string) error {
	if strings.TrimSpace(relativePath) == "" {
		return nil
	}
	target := filepath.Join(handler.packageRoot, filepath.FromSlash(relativePath))
	relative, err := filepath.Rel(handler.packageRoot, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("package file path escapes media root")
	}
	if err = os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (handler *Handler) DeleteSoftwarePackage(context *gin.Context) {
	item, err := handler.loadSoftwarePackage(context, parseID(context.Param("id")))
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "software package not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if err = handler.deleteSoftwarePackageFile(item.File); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	if _, err = handler.db.ExecContext(context, `DELETE FROM monitor_software_package WHERE id=?`, item.ID); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, gin.H{"deleted": true})
}
