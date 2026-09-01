package monitor

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

type packageMutationInput struct {
	PackageType, Name, Version, OS, Arch         string
	PlatformFamily, PlatformMajor, PackageFormat string
	DefaultPort                                  int
	ServiceRunAsUser                             string
	DefaultPortUpdate                            *int    `json:"default_port"`
	ServiceFileContent                           *string `json:"service_file_content"`
	ServiceRunAsUserUpdate                       *string `json:"service_run_as_user"`
	ServiceRunAsGroup                            *string `json:"service_run_as_group"`
	WorkDirectory                                *string `json:"work_directory"`
	InstallPlaybookContent                       *string `json:"install_playbook_content"`
	UninstallPlaybookContent                     *string `json:"uninstall_playbook_content"`
}

func (handler *Handler) CreateSoftwarePackage(context *gin.Context) {
	var input struct {
		PackageType      string `json:"package_type"`
		Name             string `json:"name"`
		Version          string `json:"version"`
		OS               string `json:"os"`
		Arch             string `json:"arch"`
		PlatformFamily   string `json:"platform_family"`
		PlatformMajor    string `json:"platform_major"`
		PackageFormat    string `json:"package_format"`
		DefaultPort      int    `json:"default_port"`
		ServiceRunAsUser string `json:"service_run_as_user"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	item := softwarePackage{PackageType: strings.TrimSpace(input.PackageType), Name: strings.TrimSpace(input.Name), Version: strings.TrimSpace(input.Version), OS: strings.TrimSpace(input.OS), Arch: strings.TrimSpace(input.Arch), PlatformFamily: strings.TrimSpace(input.PlatformFamily), PlatformMajor: strings.TrimSpace(input.PlatformMajor), PackageFormat: strings.TrimSpace(input.PackageFormat), DefaultPort: input.DefaultPort, ServiceRunAsUser: strings.TrimSpace(input.ServiceRunAsUser)}
	if err := validatePackageMetadata(item); err != nil {
		response.BusinessError(context, 400, "invalid software package configuration", gin.H{"detail": err.Error()})
		return
	}
	now := time.Now().UTC()
	result, err := handler.db.ExecContext(context, `INSERT INTO monitor_software_package(create_time,update_time,remark,package_type,name,version,default_port,os,arch,platform_family,platform_major,package_format,file,sha256,size_bytes,enabled,work_directory,service_file_content,service_run_as_user,service_run_as_group) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, now, now, nil, item.PackageType, item.Name, item.Version, item.DefaultPort, item.OS, item.Arch, item.PlatformFamily, item.PlatformMajor, item.PackageFormat, "", "", 0, true, "/tmp", "", item.ServiceRunAsUser, "dj-agent")
	if err != nil {
		response.BusinessError(context, 400, "invalid software package configuration", gin.H{"detail": err.Error()})
		return
	}
	id, _ := result.LastInsertId()
	handler.respondSoftwarePackage(context, id)
}

func validatePackageMetadata(item softwarePackage) error {
	if item.PackageType != "exporter" && item.PackageType != "fluent_bit" {
		return fmt.Errorf("invalid package_type")
	}
	if item.Name == "" || item.Version == "" || item.ServiceRunAsUser == "" || item.OS != "linux" || (item.Arch != "amd64" && item.Arch != "arm64") || item.DefaultPort < 1 || item.DefaultPort > 65535 {
		return fmt.Errorf("invalid required fields, architecture, or default port")
	}
	if item.PackageType == "fluent_bit" && item.Name != "fluent-bit" {
		return fmt.Errorf("Fluent Bit package name must be fluent-bit")
	}
	if item.PackageFormat == "tar.gz" && item.PlatformFamily == "any" && item.PlatformMajor == "" {
		return nil
	}
	if item.PackageFormat == "rpm" && item.PlatformFamily == "rhel" && item.PlatformMajor != "" {
		return nil
	}
	if item.PackageFormat == "deb" && (item.PlatformFamily == "ubuntu" || item.PlatformFamily == "debian") && item.PlatformMajor != "" {
		return nil
	}
	return fmt.Errorf("package format does not match platform fields")
}

func (handler *Handler) UpdateSoftwarePackage(context *gin.Context) {
	id := parseID(context.Param("id"))
	item, err := handler.loadSoftwarePackage(context, id)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "software package not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	var input packageMutationInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	if input.DefaultPortUpdate != nil {
		item.DefaultPort = *input.DefaultPortUpdate
	}
	if input.ServiceRunAsUserUpdate != nil {
		item.ServiceRunAsUser = strings.TrimSpace(*input.ServiceRunAsUserUpdate)
	}
	if item.DefaultPort < 1 || item.DefaultPort > 65535 || item.ServiceRunAsUser == "" {
		response.BusinessError(context, 400, "default_port and service_run_as_user are invalid", nil)
		return
	}
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer transaction.Rollback()
	_, err = transaction.ExecContext(context, `UPDATE monitor_software_package SET default_port=?,service_file_content=COALESCE(?,service_file_content),service_run_as_user=?,service_run_as_group=COALESCE(?,service_run_as_group),work_directory=COALESCE(?,work_directory),update_time=? WHERE id=?`, item.DefaultPort, input.ServiceFileContent, item.ServiceRunAsUser, input.ServiceRunAsGroup, input.WorkDirectory, time.Now().UTC(), id)
	if err == nil {
		err = syncExistingPackagePlaybook(context, transaction, item, "install", item.InstallTemplateID, input.InstallPlaybookContent)
	}
	if err == nil {
		err = syncExistingPackagePlaybook(context, transaction, item, "uninstall", item.UninstallTemplateID, input.UninstallPlaybookContent)
	}
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	if err = transaction.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	handler.respondSoftwarePackage(context, id)
}

func syncExistingPackagePlaybook(context *gin.Context, transaction *sql.Tx, item softwarePackage, role string, templateID sql.NullInt64, content *string) error {
	if content == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*content)
	if templateID.Valid {
		if trimmed == "" {
			if _, err := transaction.ExecContext(context, `UPDATE monitor_software_package SET `+role+`_playbook_template_id=NULL WHERE id=?`, item.ID); err != nil {
				return err
			}
			_, err := transaction.ExecContext(context, `DELETE FROM automation_playbook_template WHERE id=?`, templateID.Int64)
			return err
		}
		_, err := transaction.ExecContext(context, `UPDATE automation_playbook_template SET content=?,update_time=? WHERE id=?`, trimmed, time.Now().UTC(), templateID.Int64)
		return err
	}
	if trimmed == "" {
		return nil
	}
	now := time.Now().UTC()
	result, err := transaction.ExecContext(context, `INSERT INTO automation_playbook_template(create_time,update_time,remark,name,description,content,category) VALUES(?,?,?,?,?,?,?)`, now, now, nil, fmt.Sprintf("%s-%d-%s", item.Name, item.ID, role), "", trimmed, "software_package")
	if err != nil {
		return err
	}
	newID, _ := result.LastInsertId()
	_, err = transaction.ExecContext(context, `UPDATE monitor_software_package SET `+role+`_playbook_template_id=? WHERE id=?`, newID, item.ID)
	return err
}
