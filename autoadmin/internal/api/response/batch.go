package response

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type BatchResult struct {
	ID      int64  `json:"id"`
	Code    int    `json:"code"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Host    string `json:"host,omitempty"`
	HostID  int64  `json:"host_id,omitempty"`
	Detail  any    `json:"detail,omitempty"`
}

// BatchItem maps known business errors explicitly; unknown errors must not expose internal details.
func BatchItem(id int64, err error) BatchResult {
	result := BatchResult{ID: id, Code: 200, OK: true}
	if err == nil {
		return result
	}
	result.OK = false
	if appErr, ok := apperror.As(err); ok {
		result.Code, result.Message = appErr.Code(), appErr.Message()
		if cause := errors.Unwrap(appErr); cause != nil {
			log.Printf("[API-BATCH-ERROR] id=%d code=%d: %s: %v", id, result.Code, result.Message, cause)
		}
		return result
	}
	if errors.Is(err, sql.ErrNoRows) {
		result.Code, result.Message = apperror.CodeNotFound, apperror.ErrResourceNotFound.Message()
		return result
	}
	log.Printf("[API-BATCH-ERROR] id=%d: %v", id, err)
	result.Code, result.Message = apperror.CodeInternal, apperror.ErrInternal.Message()
	return result
}

// Batch reports failure at the envelope level while retaining every committed item result.
func Batch(context *gin.Context, results []BatchResult) {
	BatchWith(context, results, nil)
}

// BatchWith preserves resource-specific summary fields alongside the common results.
func BatchWith(context *gin.Context, results []BatchResult, extra gin.H) {
	count := 0
	var firstFailure *BatchResult
	sameCode, sameMessage := true, true
	for i := range results {
		item := &results[i]
		if item.OK {
			count++
			continue
		}
		if firstFailure == nil {
			firstFailure = item
		} else {
			sameCode = sameCode && item.Code == firstFailure.Code
			sameMessage = sameMessage && item.Message == firstFailure.Message
		}
	}
	data := gin.H{"count": count, "results": results}
	for key, value := range extra {
		if _, exists := data[key]; !exists {
			data[key] = value
		}
	}
	if firstFailure == nil {
		Success(context, data)
		return
	}
	if count > 0 {
		BusinessError(context, apperror.CodeBatchPartial, fmt.Sprintf("部分操作失败：成功 %d 项，失败 %d 项", count, len(results)-count), data)
		return
	}
	if sameCode {
		message := fmt.Sprintf("操作失败：%d 项失败", len(results))
		if sameMessage {
			message = firstFailure.Message
		}
		BusinessError(context, firstFailure.Code, message, data)
		return
	}
	BusinessError(context, apperror.CodeBatchFailed, fmt.Sprintf("操作失败：%d 项失败", len(results)), data)
}
