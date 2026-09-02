package monitor

import (
	"database/sql"

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
	var item softwarePackage
	err := handler.db.QueryRowContext(context, `SELECT id,package_type,name,version,os,arch,platform_family,platform_major,package_format,file,sha256,size_bytes,default_port,enabled,install_playbook_template_id,uninstall_playbook_template_id,work_directory,service_file_content,service_run_as_user,service_run_as_group FROM monitor_software_package WHERE id=?`, id).Scan(
		&item.ID, &item.PackageType, &item.Name, &item.Version, &item.OS, &item.Arch,
		&item.PlatformFamily, &item.PlatformMajor, &item.PackageFormat, &item.File, &item.SHA256,
		&item.SizeBytes, &item.DefaultPort, &item.Enabled, &item.InstallTemplateID,
		&item.UninstallTemplateID, &item.WorkDirectory, &item.ServiceFileContent,
		&item.ServiceRunAsUser, &item.ServiceRunAsGroup,
	)
	return item, err
}
