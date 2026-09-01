package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"autoadmin/internal/identity"

	"github.com/gin-gonic/gin"
)

type memoryRecorder struct {
	entries []Entry
}

func (recorder *memoryRecorder) Record(_ context.Context, entry Entry) error {
	recorder.entries = append(recorder.entries, entry)
	return nil
}

func TestCaptureRecordsAndRedactsMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &memoryRecorder{}
	engine := gin.New()
	engine.Use(Capture(recorder))
	engine.POST("/sys/users/:id/", func(ginContext *gin.Context) {
		ginContext.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 7, Username: "operator"})
		var request map[string]any
		if err := ginContext.ShouldBindJSON(&request); err != nil {
			ginContext.Status(http.StatusBadRequest)
			return
		}
		ginContext.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": gin.H{
			"name": request["name"], "token": "response-secret",
		}})
	})

	request := httptest.NewRequest(http.MethodPost, "/sys/users/7/?token=query-secret", strings.NewReader(`{"name":"visible","password":"body-secret","nested":{"private_key":"key-secret"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusOK || len(recorder.entries) != 1 {
		t.Fatalf("status=%d audit entries=%d", response.Code, len(recorder.entries))
	}
	entry := recorder.entries[0]
	if entry.UserID != 7 || entry.Username != "operator" || entry.RouteName != "/sys/users/:id/" {
		t.Fatalf("unexpected audit identity/route: %+v", entry)
	}
	combined := entry.RequestData + entry.ResponseData
	for _, secret := range []string{"query-secret", "body-secret", "key-secret", "response-secret"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("audit payload leaked %q: %s", secret, combined)
		}
	}
	if !strings.Contains(combined, "******") || !strings.Contains(entry.RequestData, "visible") {
		t.Fatalf("audit payload was not correctly redacted: %s", combined)
	}
	var responsePayload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &responsePayload); err != nil || responsePayload["code"] != float64(200) {
		t.Fatalf("business response was changed: %s", response.Body.String())
	}
}

func TestCaptureSkipsExcludedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &memoryRecorder{}
	engine := gin.New()
	engine.Use(Capture(recorder))
	authenticated := func(ginContext *gin.Context) {
		ginContext.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 1, Username: "admin"})
		ginContext.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success"})
	}
	engine.GET("/read", authenticated)
	engine.POST("/sys/login", authenticated)
	engine.POST("/sys/audit/operation-logs/", authenticated)

	for _, test := range []struct{ method, path string }{
		{method: http.MethodGet, path: "/read"},
		{method: http.MethodPost, path: "/sys/login"},
		{method: http.MethodPost, path: "/sys/audit/operation-logs/"},
		{method: http.MethodPost, path: "/missing"},
	} {
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(test.method, test.path, nil))
	}
	if len(recorder.entries) != 0 {
		t.Fatalf("excluded requests created %d audit entries", len(recorder.entries))
	}
}
