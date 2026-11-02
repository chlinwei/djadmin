package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestCaptureLargeJSONResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		text string
	}{
		{name: "short", text: "visible"},
		{name: "long", text: strings.Repeat("visible", 1500)},
		{name: "unicode", text: strings.Repeat("\u754c", 3000)},
		{name: "xml", text: strings.Repeat("<adb:message>A&B</adb:message>", 300)},
		{name: "over_budget", text: strings.Repeat("x", auditResponseCaptureLimit)},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &memoryRecorder{}
			engine := gin.New()
			engine.Use(Capture(recorder))
			payload := gin.H{"code": 200, "msg": "success", "data": gin.H{
				"access_token": "response-secret", "text": test.text,
				"nested": gin.H{"password": "nested-secret"},
			}}
			expected, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			engine.POST("/simulate", func(c *gin.Context) {
				c.Set(identity.ClaimsContextKey, &identity.Claims{UserID: 7, Username: "operator"})
				c.JSON(http.StatusOK, payload)
			})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/simulate", strings.NewReader(string(expected)))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != string(expected) {
				t.Fatal("audit changed the business response")
			}
			if len(recorder.entries) != 1 {
				t.Fatalf("audit entries=%d", len(recorder.entries))
			}
			entry := recorder.entries[0]
			if test.name == "xml" {
				for _, captured := range []string{entry.RequestData, entry.ResponseData} {
					if !strings.Contains(captured, "<adb:message>A&B</adb:message>") ||
						strings.Contains(captured, `\u003c`) || strings.Contains(captured, `\u003e`) ||
						strings.Contains(captured, `\u0026`) {
						t.Fatal("truncated audit JSON escaped readable XML markup")
					}
					if strings.Contains(captured, "response-secret") || strings.Contains(captured, "nested-secret") {
						t.Fatal("readable XML bypassed audit redaction")
					}
				}
			}
			if test.name == "over_budget" {
				if !json.Valid([]byte(entry.ResponseData)) ||
					!strings.Contains(entry.ResponseData, `"capture_status":"response_too_large"`) ||
					strings.Contains(entry.ResponseData, "response-secret") ||
					strings.Contains(entry.ResponseData, "nested-secret") {
					t.Fatal("oversized business response lacks a safe audit omission summary")
				}
				return
			}
			if entry.ResponseData == "" || entry.Message != "success" {
				t.Fatalf("response capture was lost: %+v", entry)
			}
			if len(entry.ResponseData) > auditTextLimit || !utf8.ValidString(entry.ResponseData) {
				t.Fatal("audit response exceeds the text limit or contains invalid UTF-8")
			}
			if !strings.Contains(entry.ResponseData, "******") ||
				strings.Contains(entry.ResponseData, "response-secret") ||
				strings.Contains(entry.ResponseData, "nested-secret") {
				t.Fatal("audit response was not safely redacted")
			}
			if test.name != "short" && !strings.HasSuffix(entry.ResponseData, auditTruncationNotice) {
				t.Fatal("long audit response lacks a truncation notice")
			}
		})
	}
}

func TestCaptureResponseBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, writeString := range []bool{false, true} {
		for _, excess := range []int{0, 1} {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			writer := &captureWriter{ResponseWriter: context.Writer}
			empty := `{"code":200,"msg":"success","data":"","token":"response-secret"}`
			body := strings.Replace(empty, `"data":""`, `"data":"`+
				strings.Repeat("x", auditResponseCaptureLimit-len(empty))+`"`, 1)
			if excess > 0 {
				body += " "
			}
			for offset := 0; offset < len(body); {
				end := min(offset+4093, len(body))
				var err error
				if writeString {
					_, err = writer.WriteString(body[offset:end])
				} else {
					_, err = writer.Write([]byte(body[offset:end]))
				}
				if err != nil {
					t.Fatal(err)
				}
				offset = end
			}
			if writer.body.Len() != auditResponseCaptureLimit || writer.truncated != (excess > 0) {
				t.Fatalf("string=%v excess=%d: captured=%d truncated=%v",
					writeString, excess, writer.body.Len(), writer.truncated)
			}
			data, message := captureResponse(writer.body.Bytes(), writer.truncated)
			if data == "" || len(data) > auditTextLimit || strings.Contains(data, "response-secret") {
				t.Fatal("capture budget caused an empty, oversized, or unredacted audit response")
			}
			if excess > 0 {
				var summary struct {
					CaptureStatus string `json:"capture_status"`
					LimitBytes    int    `json:"limit_bytes"`
				}
				if err := json.Unmarshal([]byte(data), &summary); err != nil {
					t.Fatal(err)
				}
				if summary.CaptureStatus != "response_too_large" || summary.LimitBytes != auditResponseCaptureLimit {
					t.Fatalf("unexpected omission summary: %+v", summary)
				}
			} else if message != "success" || !strings.HasSuffix(data, auditTruncationNotice) {
				t.Fatal("response at the exact capture limit was incorrectly omitted")
			}
		}
	}
}
