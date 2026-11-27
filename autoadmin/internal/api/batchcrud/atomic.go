package batchcrud

import (
	"errors"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/batchwrite"
)

// AtomicResults is called after the transaction finishes. Only the failing item
// gets the specific cause; other items were rolled back or never attempted.
func AtomicResults[T ~int32 | ~int64](ids []T, err error) []response.BatchResult {
	results := make([]response.BatchResult, 0, len(ids))
	var failure *batchwrite.ItemError
	result := response.BatchItem(0, nil)
	if errors.As(err, &failure) {
		result = response.BatchItem(failure.ID, failure.Cause)
	} else if err != nil {
		// Begin/commit failures apply to the transaction; log the cause only once.
		result = response.BatchItem(0, err)
	}
	for _, id := range ids {
		item := result
		item.ID = int64(id)
		if failure != nil && item.ID != failure.ID {
			item.Code = apperror.CodeBatchFailed
			item.Message = "整批操作未提交"
		}
		results = append(results, item)
	}
	return results
}
