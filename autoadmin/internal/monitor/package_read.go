package monitor

import (
	"database/sql"
	"strings"

	"autoadmin/internal/api/response"

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

func (handler *Handler) GetSoftwarePackage(context *gin.Context) {
	handler.respondSoftwarePackage(context, parseID(context.Param("id")))
}

func (handler *Handler) respondSoftwarePackage(context *gin.Context, id int64) {
	rows, err := handler.db.QueryContext(context, `SELECT p.*,COALESCE(i.name,'') AS install_playbook_template_name,COALESCE(i.content,'') AS install_playbook_content,COALESCE(u.name,'') AS uninstall_playbook_template_name,COALESCE(u.content,'') AS uninstall_playbook_content FROM monitor_software_package p LEFT JOIN automation_playbook_template i ON i.id=p.install_playbook_template_id LEFT JOIN automation_playbook_template u ON u.id=p.uninstall_playbook_template_id WHERE p.id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	if len(items) == 0 {
		response.BusinessError(context, 404, "software package not found", nil)
		return
	}
	decorateSoftwarePackage(items[0])
	response.Success(context, items[0])
}

func decorateSoftwarePackage(item gin.H) {
	fileName := stringValue(item["file"])
	item["download_url"] = ""
	item["file_name"] = ""
	if fileName != "" {
		item["download_url"] = "/media/" + strings.TrimLeft(fileName, "/")
		parts := strings.Split(fileName, "/")
		item["file_name"] = parts[len(parts)-1]
	}
	item["synced"] = fileName != "" && stringValue(item["sha256"]) != ""
}
