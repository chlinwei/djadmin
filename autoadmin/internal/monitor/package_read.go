package monitor

import (
	"database/sql"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type softwarePackage struct {
	ID                                           int64
	PackageType, Name, Version, OS, Arch         string
	PlatformFamily, PlatformMajor, PackageFormat string
	File, SHA256                                 string
	SizeBytes                                    int64
	DefaultPort                                  int
	Enabled                                      bool
	InstallTemplateID, UninstallTemplateID       sql.NullInt64
	WorkDirectory, ServiceFileContent            string
	ServiceRunAsUser, ServiceRunAsGroup          string
}

func (handler *Handler) loadSoftwarePackage(context *gin.Context, id int64) (softwarePackage, error) {
	// 复用已有的类型化查询（SELECT * 的产物字段与本 DTO 一一对应）。
	row, err := db.New(handler.db).GetSoftwarePackageTyped(context, id)
	if err != nil {
		return softwarePackage{}, err
	}
	return softwarePackage{
		ID: row.ID, PackageType: row.PackageType, Name: row.Name, Version: row.Version,
		OS: row.Os, Arch: row.Arch, PlatformFamily: row.PlatformFamily, PlatformMajor: row.PlatformMajor,
		PackageFormat: row.PackageFormat, File: row.File, SHA256: row.Sha256, SizeBytes: row.SizeBytes,
		DefaultPort: int(row.DefaultPort), Enabled: row.Enabled,
		InstallTemplateID: row.InstallPlaybookTemplateID, UninstallTemplateID: row.UninstallPlaybookTemplateID,
		WorkDirectory: row.WorkDirectory, ServiceFileContent: row.ServiceFileContent,
		ServiceRunAsUser: row.ServiceRunAsUser, ServiceRunAsGroup: row.ServiceRunAsGroup,
	}, nil
}
