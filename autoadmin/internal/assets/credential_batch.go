package assets

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

// 凭证 CSV 批量导入（POST /assets/credentials/batch-create/），对应 Django batchCreate action。
// CSV 表头与凭证字段同名：name,username,password,private_key,port,auth_type,remark；
// 整批要么全部成功、要么在第一处失败中止并提示行号（与 Django serializer many 校验语义一致）。

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
	created := make([]Credential, 0, len(records)-1)
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
				response.BusinessError(c, 400, "第 "+strconv.Itoa(rowNumber+2)+" 行导入失败: port 必须是数字", nil)
				return
			}
			input.Port = uint32(port)
		}
		if value := cell(row, "auth_type"); value != "" {
			authType, parseErr := strconv.ParseInt(value, 10, 32)
			if parseErr != nil {
				response.BusinessError(c, 400, "第 "+strconv.Itoa(rowNumber+2)+" 行导入失败: auth_type 必须是 1(密码) 或 2(SSH Key)", nil)
				return
			}
			input.AuthType = int32(authType)
		}
		item, err := h.service.CreateCredential(c.Request.Context(), input)
		if err != nil {
			response.BusinessError(c, 400, "第 "+strconv.Itoa(rowNumber+2)+" 行导入失败: "+err.Error(), nil)
			return
		}
		created = append(created, item)
	}
	response.Success(c, gin.H{"count": len(created), "results": created})
}
