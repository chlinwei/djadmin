package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"autoadmin/internal/identity"

	"github.com/gin-gonic/gin"
)

const auditTextLimit = 4000

type captureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (writer *captureWriter) Write(data []byte) (int, error) {
	remaining := auditTextLimit - writer.body.Len()
	if remaining > 0 {
		writer.body.Write(data[:min(len(data), remaining)])
	}
	return writer.ResponseWriter.Write(data)
}

func (writer *captureWriter) WriteString(value string) (int, error) {
	remaining := auditTextLimit - writer.body.Len()
	if remaining > 0 {
		writer.body.WriteString(value[:min(len(value), remaining)])
	}
	return writer.ResponseWriter.WriteString(value)
}

// Capture records completed authenticated mutations; audit persistence is best-effort so it can never replace the business response.
func Capture(recorder Recorder) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		startedAt := time.Now()
		requestData := captureRequest(ginContext)
		writer := &captureWriter{ResponseWriter: ginContext.Writer}
		ginContext.Writer = writer

		ginContext.Next()

		claims, authenticated := identity.ClaimsFromContext(ginContext)
		if !authenticated || shouldSkip(ginContext) {
			return
		}
		responseData, message := captureResponse(writer.body.Bytes())
		duration := time.Since(startedAt).Milliseconds()
		if duration > int64(^uint32(0)>>1) {
			duration = int64(^uint32(0) >> 1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := recorder.Record(ctx, Entry{
			Username: claims.Username, UserID: claims.UserID,
			Method: ginContext.Request.Method, Path: truncate(ginContext.Request.URL.Path, 255),
			RouteName: truncate(ginContext.FullPath(), 255), ClientIP: clientIP(ginContext),
			UserAgent:  truncate(ginContext.GetHeader("User-Agent"), 255),
			StatusCode: int32(ginContext.Writer.Status()), DurationMS: int32(duration),
			Message: truncate(message, 255), RequestData: requestData, ResponseData: responseData,
		}); err != nil {
			slog.Error("write operation audit", "error", err, "route", ginContext.FullPath())
		}
	}
}

func shouldSkip(ginContext *gin.Context) bool {
	method := ginContext.Request.Method
	path := ginContext.Request.URL.Path
	return method == http.MethodGet || method == http.MethodOptions || ginContext.Writer.Status() == http.StatusNotFound ||
		ginContext.FullPath() == "" || path == "/sys/login" || strings.HasPrefix(path, "/sys/audit/") ||
		strings.HasPrefix(path, "/media") || strings.HasPrefix(path, "/static")
}

func captureRequest(ginContext *gin.Context) string {
	payload := map[string]any{}
	if len(ginContext.Request.URL.Query()) > 0 {
		payload["query"] = redactValues(ginContext.Request.URL.Query())
	}
	if len(ginContext.Params) > 0 {
		params := map[string]string{}
		for _, param := range ginContext.Params {
			params[param.Key] = param.Value
		}
		payload["path"] = params
	}
	contentType := strings.ToLower(ginContext.GetHeader("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") && ginContext.Request.Body != nil {
		body, err := io.ReadAll(ginContext.Request.Body)
		if err == nil {
			ginContext.Request.Body = io.NopCloser(bytes.NewReader(body))
			if parsed := parseBody(contentType, body); parsed != nil {
				payload["body"] = parsed
			}
		}
	}
	return encodeLimited(payload)
}

func parseBody(contentType string, body []byte) any {
	if len(body) == 0 {
		return nil
	}
	if strings.Contains(contentType, "application/json") {
		var value any
		if json.Unmarshal(body, &value) == nil {
			return redact(value)
		}
		return nil
	}
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		values, err := url.ParseQuery(string(body))
		if err == nil {
			return redactValues(values)
		}
	}
	return nil
}

func captureResponse(body []byte) (string, string) {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return "", ""
	}
	redacted := redact(value)
	message := ""
	if object, ok := redacted.(map[string]any); ok {
		message, _ = object["msg"].(string)
	}
	return encodeLimited(redacted), message
}

func redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveKey(key) {
				result[key] = "******"
			} else {
				result[key] = redact(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redact(item)
		}
		return result
	default:
		return value
	}
}

func redactValues(values url.Values) map[string]any {
	result := make(map[string]any, len(values))
	for key, items := range values {
		if sensitiveKey(key) {
			result[key] = "******"
		} else {
			result[key] = items
		}
	}
	return result
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, fragment := range []string{"password", "token", "secret", "private_key", "authorization"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func encodeLimited(value any) string {
	if object, ok := value.(map[string]any); ok && len(object) == 0 {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return truncate(string(encoded), auditTextLimit)
}

func clientIP(ginContext *gin.Context) string {
	if forwarded := ginContext.GetHeader("X-Forwarded-For"); forwarded != "" {
		return truncate(strings.TrimSpace(strings.Split(forwarded, ",")[0]), 64)
	}
	return truncate(ginContext.ClientIP(), 64)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
