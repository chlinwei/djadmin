package monitor

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"

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
	if id == 0 {
		var count int
		if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM monitor_opensearch_cluster`).Scan(&count); err != nil {
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
	if isDefault, ok := values["is_default"].(bool); ok && isDefault {
		_, err = transaction.ExecContext(context, `UPDATE monitor_opensearch_cluster SET is_default=FALSE,update_time=? WHERE is_default=TRUE AND id<>?`, time.Now().UTC(), id)
		if err != nil {
			response.Error(context, err)
			return
		}
	}
	if id == 0 {
		id, err = insertOpenSearchCluster(context, transaction, values)
	} else {
		err = updateOpenSearchCluster(context, transaction, id, values)
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
	handler.respondOpenSearchCluster(context, id)
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

type businessValidationError string

func (err businessValidationError) Error() string { return string(err) }

func insertOpenSearchCluster(context *gin.Context, transaction *sql.Tx, values map[string]any) (int64, error) {
	columns := sortedColumns(values)
	placeholders, arguments := make([]string, 0, len(columns)+2), make([]any, 0, len(columns)+2)
	for _, column := range columns {
		placeholders = append(placeholders, "?")
		arguments = append(arguments, values[column])
	}
	now := time.Now().UTC()
	columns = append(columns, "create_time", "update_time")
	placeholders = append(placeholders, "?", "?")
	arguments = append(arguments, now, now)
	result, err := transaction.ExecContext(context, `INSERT INTO monitor_opensearch_cluster (`+strings.Join(columns, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, arguments...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func updateOpenSearchCluster(context *gin.Context, transaction *sql.Tx, id int64, values map[string]any) error {
	columns := sortedColumns(values)
	assignments, arguments := make([]string, 0, len(columns)+1), make([]any, 0, len(columns)+2)
	for _, column := range columns {
		assignments = append(assignments, column+"=?")
		arguments = append(arguments, values[column])
	}
	assignments = append(assignments, "update_time=?")
	arguments = append(arguments, time.Now().UTC(), id)
	result, err := transaction.ExecContext(context, `UPDATE monitor_opensearch_cluster SET `+strings.Join(assignments, ",")+` WHERE id=?`, arguments...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func sortedColumns(values map[string]any) []string {
	columns := make([]string, 0, len(values))
	for column := range values {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func (handler *Handler) DeleteOpenSearchCluster(context *gin.Context) {
	result, err := handler.db.ExecContext(context, `DELETE FROM monitor_opensearch_cluster WHERE id=?`, parseID(context.Param("id")))
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "OpenSearch cluster not found", nil)
		return
	}
	response.Success(context, gin.H{"deleted": true})
}
