package assets

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"autoadmin/internal/api/batchcrud"
	"autoadmin/internal/api/response"
	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/batchwrite"

	"github.com/gin-gonic/gin"
)

const (
	credentialBatchMaxRows  = 1000
	credentialBatchMaxBytes = 5 << 20
)

// stringPointer：空单元格 → nil（沿用服务端默认值），非空 → 指针。
func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (h *Handler) BatchCreateCredentials(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.BusinessError(c, 400, "请选择 CSV 文件", nil)
		return
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".csv") {
		response.BusinessError(c, 400, "仅支持 .csv 文件", nil)
		return
	}
	if file.Size > credentialBatchMaxBytes {
		response.BusinessError(c, 400, "CSV 不可超过 5 MiB", nil)
		return
	}
	opened, err := file.Open()
	if err != nil {
		response.BusinessError(c, 400, "读取上传文件失败", nil)
		return
	}
	defer opened.Close()
	reader := csv.NewReader(io.LimitReader(opened, credentialBatchMaxBytes+1))
	records, err := reader.ReadAll()
	if err != nil {
		response.BusinessError(c, 400, "CSV 解析失败: "+err.Error(), nil)
		return
	}
	if len(records) > credentialBatchMaxRows+1 {
		response.BusinessError(c, 400, "单次最多导入 1000 条凭证", nil)
		return
	}
	if len(records) < 2 {
		response.BusinessError(c, 400, "CSV 至少需要表头和一行数据", nil)
		return
	}

	columns := make(map[string]int)
	for index, name := range records[0] {
		columns[strings.TrimSpace(strings.ToLower(name))] = index
	}
	if _, ok := columns["username"]; !ok {
		response.BusinessError(c, 400, "CSV 缺少 username 列", nil)
		return
	}

	cell := func(row []string, name string) string {
		index, ok := columns[name]
		if !ok || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	// Rows have no persistent identity until commit. Use record numbers (header = 1)
	// on both paths; never expose credentials or rolled-back database IDs in results.
	rows := make([]int64, len(records)-1)
	for i := range rows {
		rows[i] = int64(i + 2)
	}
	failRow := func(row int64, cause error) {
		response.Batch(c, batchcrud.AtomicResults(rows, &batchwrite.ItemError{ID: row, Cause: cause}))
	}
	params := make([]db.CreateCredentialParams, len(rows))
	for rowNumber, row := range records[1:] {
		input := CredentialInput{
			Name:       stringPointer(cell(row, "name")),
			Username:   cell(row, "username"),
			Password:   stringPointer(cell(row, "password")),
			PrivateKey: stringPointer(cell(row, "private_key")),
			Remark:     stringPointer(cell(row, "remark")),
		}
		if value := cell(row, "port"); value != "" {
			port, parseErr := strconv.ParseUint(value, 10, 32)
			if parseErr != nil {
				failRow(rows[rowNumber], apperror.New(400, "port 必须是数字"))
				return
			}
			input.Port = uint32(port)
		}
		if value := cell(row, "auth_type"); value != "" {
			authType, parseErr := strconv.ParseInt(value, 10, 32)
			if parseErr != nil {
				failRow(rows[rowNumber], apperror.New(400, "auth_type 必须是 1(密码) 或 2(SSH Key)"))
				return
			}
			input.AuthType = int32(authType)
		}
		params[rowNumber], err = h.service.newCredentialParams(input)
		if err != nil {
			failRow(rows[rowNumber], err)
			return
		}
	}
	ctx := c.Request.Context()
	created := make([]int64, len(rows))
	err = database.InTransaction(ctx, h.service.repository.pool, func(q *db.Queries) error {
		return batchwrite.Each(rows, func(row int64) error {
			result, err := q.CreateCredential(ctx, params[row-2])
			if err != nil {
				return translate(err)
			}
			if err := batchwrite.RequireRow(result.RowsAffected()); err != nil {
				return err
			}
			created[row-2], err = result.LastInsertId()
			return err
		})
	})
	results := batchcrud.AtomicResults(rows, err)
	if err == nil {
		for i := range results {
			results[i].Detail = gin.H{"credential_id": created[i]}
		}
	}
	response.Batch(c, results)
}
