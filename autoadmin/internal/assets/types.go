package assets

import (
	"database/sql"
	"strings"
	"time"
)

const secretMask = "******"

type Project struct {
	ID         int64   `json:"id"`
	CreateTime string  `json:"create_time"`
	UpdateTime string  `json:"update_time"`
	Remark     *string `json:"remark"`
	Name       string  `json:"name"`
	Code       string  `json:"code"`
	Owner      string  `json:"owner"`
	Enabled    bool    `json:"enabled"`
}

type BusinessSystem struct {
	ID          int64   `json:"id"`
	CreateTime  string  `json:"create_time"`
	UpdateTime  string  `json:"update_time"`
	Remark      *string `json:"remark"`
	Name        string  `json:"name"`
	Code        string  `json:"code"`
	Owner       string  `json:"owner"`
	Enabled     bool    `json:"enabled"`
	Project     *int64  `json:"project"`
	ProjectName string  `json:"project_name"`
	ProjectCode string  `json:"project_code"`
}

type BusinessEnvironment struct {
	ID         int64   `json:"id"`
	CreateTime string  `json:"create_time"`
	UpdateTime string  `json:"update_time"`
	Remark     *string `json:"remark"`
	Name       string  `json:"name"`
	Code       string  `json:"code"`
	Order      uint32  `json:"order"`
	Owner      string  `json:"owner"`
	Enabled    bool    `json:"enabled"`
}

type Credential struct {
	ID         int64   `json:"id"`
	CreateTime string  `json:"create_time"`
	UpdateTime string  `json:"update_time"`
	Remark     *string `json:"remark"`
	Name       *string `json:"name"`
	Password   *string `json:"password"`
	PrivateKey *string `json:"private_key"`
	AuthType   int32   `json:"auth_type"`
	Username   string  `json:"username"`
	Port       uint32  `json:"port"`
}

type HostGroup struct {
	ID         int64       `json:"id"`
	CreateTime string      `json:"create_time"`
	UpdateTime string      `json:"update_time"`
	Remark     *string     `json:"remark"`
	Name       string      `json:"name"`
	Parent     *int64      `json:"parent"`
	ParentID   *int64      `json:"parent_id"`
	ParentName string      `json:"parent_name"`
	HostCount  int64       `json:"host_count"`
	Children   []HostGroup `json:"children,omitempty"`
}

type Host struct {
	ID                    int64   `json:"id"`
	CreateTime            string  `json:"create_time"`
	UpdateTime            string  `json:"update_time"`
	Remark                *string `json:"remark"`
	Status                string  `json:"status"`
	InstanceID            *string `json:"instance_id"`
	IP                    *string `json:"ip"`
	IsDeletedInCloud      bool    `json:"is_deleted_in_cloud"`
	CloudAccount          *int64  `json:"cloud_account"`
	Group                 *int64  `json:"group"`
	GroupName             string  `json:"group_name"`
	InstanceName          *string `json:"instance_name"`
	CollectStatus         string  `json:"collect_status"`
	CollectMessage        string  `json:"collect_message"`
	CollectTime           *string `json:"collect_time"`
	AgentOnline           bool    `json:"agent_online"`
	AgentOnlineTime       *string `json:"agent_online_time"`
	WebSSHDefaultUsername string  `json:"webssh_default_username"`
	WebSSHLoginUsers      string  `json:"webssh_login_users"`
	AgentID               *string `json:"agent_id"`
	Environment           *int64  `json:"environment"`
	EnvironmentName       string  `json:"environment_name"`
}

type ProjectInput struct {
	Name    string  `json:"name"`
	Code    string  `json:"code"`
	Owner   string  `json:"owner"`
	Enabled *bool   `json:"enabled"`
	Remark  *string `json:"remark"`
}
type BusinessSystemInput struct {
	Name    string  `json:"name"`
	Code    string  `json:"code"`
	Owner   string  `json:"owner"`
	Enabled *bool   `json:"enabled"`
	Project *int64  `json:"project"`
	Remark  *string `json:"remark"`
}
type EnvironmentInput struct {
	Name    string  `json:"name"`
	Code    string  `json:"code"`
	Order   uint32  `json:"order"`
	Owner   string  `json:"owner"`
	Enabled *bool   `json:"enabled"`
	Remark  *string `json:"remark"`
}
type CredentialInput struct {
	Name       *string `json:"name"`
	Username   string  `json:"username"`
	Password   *string `json:"password"`
	PrivateKey *string `json:"private_key"`
	Port       uint32  `json:"port"`
	AuthType   int32   `json:"auth_type"`
	Remark     *string `json:"remark"`
}
type HostGroupInput struct {
	Name     string  `json:"name"`
	ParentID *int64  `json:"parent_id"`
	Remark   *string `json:"remark"`
}
type HostInput struct {
	InstanceName          *string `json:"instance_name"`
	AgentID               *string `json:"agent_id"`
	IP                    *string `json:"ip"`
	InstanceID            *string `json:"instance_id"`
	Environment           *int64  `json:"environment"`
	CloudAccount          *int64  `json:"cloud_account"`
	GroupID               *int64  `json:"group_id"`
	Status                string  `json:"status"`
	IsDeletedInCloud      bool    `json:"is_deleted_in_cloud"`
	WebSSHDefaultUsername string  `json:"webssh_default_username"`
	WebSSHLoginUsers      string  `json:"webssh_login_users"`
	Remark                *string `json:"remark"`
}

func timestamp(value time.Time) string { return value.UTC().Format("2006-01-02T15:04:05.999999Z") }
func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
func stringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func nullInt(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}
func intValue(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func timeValue(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	result := timestamp(value.Time)
	return &result
}
func enabled(value *bool) bool     { return value == nil || *value }
func pattern(search string) string { return "%" + strings.TrimSpace(search) + "%" }
