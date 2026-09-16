package monitor

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type openSearchClusterInput struct {
	Name           *string `json:"name"`
	Hosts          *string `json:"hosts"`
	Username       *string `json:"username"`
	Password       *string `json:"password"`
	VerifyTLS      *bool   `json:"verify_tls"`
	CACert         *string `json:"ca_cert"`
	IndexPrefix    *string `json:"index_prefix"`
	RequestTimeout *int    `json:"request_timeout"`
	Enabled        *bool   `json:"enabled"`
	IsDefault      *bool   `json:"is_default"`
	Remark         *string `json:"remark"`
}

func (handler *Handler) CreateOpenSearchCluster(context *gin.Context) {
	handler.saveOpenSearchCluster(context, 0)
}

func (handler *Handler) UpdateOpenSearchCluster(context *gin.Context) {
	handler.saveOpenSearchCluster(context, parseID(context.Param("id")))
}

func (handler *Handler) saveOpenSearchCluster(context *gin.Context, id int64) {
	var input openSearchClusterInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	queries := db.New(handler.db)
	if id == 0 {
		count, err := queries.CountAllOpenSearchClusters(context)
		if err != nil {
			response.Error(context, err)
			return
		}
		if count > 0 {
			response.BusinessError(context, 400, "only one OpenSearch cluster is supported; edit the existing cluster", nil)
			return
		}
	}
	values, err := handler.openSearchClusterValues(input, id == 0)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	if len(values) == 0 {
		response.BusinessError(context, 400, "no fields to update", nil)
		return
	}

	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer transaction.Rollback()
	txQueries := db.New(transaction)
	if isDefault, ok := values["is_default"].(bool); ok && isDefault {
		if err = txQueries.ClearDefaultOpenSearchCluster(context, db.ClearDefaultOpenSearchClusterParams{
			UpdateTime: time.Now().UTC(), ID: id,
		}); err != nil {
			response.Error(context, err)
			return
		}
	}
	if id == 0 {
		// 新建时 openSearchClusterValues(creating=true) 已把未提交的列补上默认值，直接整行插入。
		id, err = txQueries.CreateOpenSearchCluster(context, db.CreateOpenSearchClusterParams{
			CreateTime: time.Now().UTC(), UpdateTime: time.Now().UTC(),
			Name: stringValue(values["name"]), Hosts: stringValue(values["hosts"]),
			Username: stringValue(values["username"]), Password: stringValue(values["password"]),
			VerifyTls: boolValue(values["verify_tls"]), CaCert: stringValue(values["ca_cert"]),
			IndexPrefix: stringValue(values["index_prefix"]), RequestTimeout: uint32(intValue(values["request_timeout"])),
			Enabled: boolValue(values["enabled"]), IsDefault: boolValue(values["is_default"]),
			Remark: stringValue(values["remark"]),
		})
	} else {
		// PATCH 语义：读回整行 → 合并提交的字段 → 整行写
		//（原实现是运行时拼 `SET ` + 列名，sqlc 的语句是编译期固定的）。
		merged, mergeErr := mergedOpenSearchCluster(context, queries, id, values)
		if mergeErr == sql.ErrNoRows {
			response.BusinessError(context, 404, "OpenSearch cluster not found", nil)
			return
		}
		if mergeErr != nil {
			response.Error(context, mergeErr)
			return
		}
		affected, updateErr := txQueries.UpdateOpenSearchCluster(context, db.UpdateOpenSearchClusterParams{
			UpdateTime: time.Now().UTC(),
			Name:       merged.Name, Hosts: merged.Hosts, Username: merged.Username, Password: merged.Password,
			VerifyTls: merged.VerifyTls, CaCert: merged.CaCert, IndexPrefix: merged.IndexPrefix,
			RequestTimeout: merged.RequestTimeout, Enabled: merged.Enabled, IsDefault: merged.IsDefault,
			Remark: merged.Remark, ID: id,
		})
		err = updateErr
		if err == nil && affected == 0 {
			err = sql.ErrNoRows
		}
	}
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "OpenSearch cluster not found", nil)
		return
	}
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	if err = transaction.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	// 集群新增/修改后异步 bootstrap（模板 + ISM 策略），对应 Django sync_log_storage_quietly；
	// 失败只落 storage_sync_* 状态，不阻塞保存。
	go handler.syncClusterLogStorage(id)
	handler.respondOpenSearchCluster(context, id)
}

// mergedOpenSearchCluster 把已提交的字段合并进数据库里的当前行（整行写的输入）。
// 密码列存的是密文，未提交时原样保留（openSearchClusterValues 只把提交的新密码加密后放进来）。
// businessValidationError 标记"入参校验失败"（与库/网络错误区分：调用点按 400 返回原文案）。
type businessValidationError string

func (err businessValidationError) Error() string { return string(err) }

func mergedOpenSearchCluster(context context.Context, queries *db.Queries, id int64, values map[string]any) (db.MonitorOpensearchCluster, error) {
	current, err := queries.GetOpenSearchClusterTyped(context, id)
	if err != nil {
		return db.MonitorOpensearchCluster{}, err
	}
	if value, ok := values["name"]; ok {
		current.Name = stringValue(value)
	}
	if value, ok := values["hosts"]; ok {
		current.Hosts = stringValue(value)
	}
	if value, ok := values["username"]; ok {
		current.Username = stringValue(value)
	}
	if value, ok := values["password"]; ok {
		current.Password = stringValue(value)
	}
	if value, ok := values["verify_tls"]; ok {
		current.VerifyTls = boolValue(value)
	}
	if value, ok := values["ca_cert"]; ok {
		current.CaCert = stringValue(value)
	}
	if value, ok := values["index_prefix"]; ok {
		current.IndexPrefix = stringValue(value)
	}
	if value, ok := values["request_timeout"]; ok {
		current.RequestTimeout = uint32(intValue(value))
	}
	if value, ok := values["enabled"]; ok {
		current.Enabled = boolValue(value)
	}
	if value, ok := values["is_default"]; ok {
		current.IsDefault = boolValue(value)
	}
	if value, ok := values["remark"]; ok {
		current.Remark = stringValue(value)
	}
	return current, nil
}

func (handler *Handler) openSearchClusterValues(input openSearchClusterInput, creating bool) (map[string]any, error) {
	values := map[string]any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, businessValidationError("name is required")
		}
		values["name"] = name
	}
	if input.Hosts != nil {
		hosts := make([]string, 0)
		for _, host := range strings.Split(*input.Hosts, ",") {
			host = strings.TrimSpace(host)
			if host == "" {
				continue
			}
			if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				return nil, businessValidationError("hosts must start with http:// or https://")
			}
			hosts = append(hosts, host)
		}
		if len(hosts) == 0 {
			return nil, businessValidationError("at least one host is required")
		}
		values["hosts"] = strings.Join(hosts, ",")
	}
	if input.IndexPrefix != nil {
		prefix := strings.TrimSpace(*input.IndexPrefix)
		if prefix == "" || prefix != strings.ToLower(prefix) || strings.Contains(prefix, " ") {
			return nil, businessValidationError("index_prefix must be lowercase and contain no spaces")
		}
		values["index_prefix"] = prefix
	}
	if input.RequestTimeout != nil {
		if *input.RequestTimeout < 1 {
			return nil, businessValidationError("request_timeout must be positive")
		}
		values["request_timeout"] = *input.RequestTimeout
	}
	for column, value := range map[string]*string{"username": input.Username, "ca_cert": input.CACert, "remark": input.Remark} {
		if value != nil {
			values[column] = strings.TrimSpace(*value)
		}
	}
	for column, value := range map[string]*bool{"verify_tls": input.VerifyTLS, "enabled": input.Enabled, "is_default": input.IsDefault} {
		if value != nil {
			values[column] = *value
		}
	}
	if input.Password != nil && *input.Password != "******" {
		if *input.Password == "" {
			values["password"] = ""
		} else {
			encrypted, err := handler.secrets.Encrypt(*input.Password)
			if err != nil {
				return nil, err
			}
			values["password"] = encrypted
		}
	}
	if creating {
		if input.Name == nil || input.Hosts == nil {
			return nil, businessValidationError("name and hosts are required")
		}
		for column, value := range map[string]any{"username": "", "password": "", "verify_tls": false, "ca_cert": "", "index_prefix": "logs", "request_timeout": 10, "enabled": true, "is_default": false, "remark": ""} {
			if _, exists := values[column]; !exists {
				values[column] = value
			}
		}
	}
	return values, nil
}

func (handler *Handler) BatchDeleteOpenSearchClusters(context *gin.Context) {
	batchDeleteMonitorRows(context, handler, handler.deleteOpenSearchClusterByID)
}

// deleteOpenSearchClusterByID 找不到行时返回 sql.ErrNoRows，由批删入口记成 ok:false。
func (handler *Handler) deleteOpenSearchClusterByID(context *gin.Context, id int64) error {
	return deleteRowsAffected(db.New(handler.db).DeleteOpenSearchCluster(context, id))
}
