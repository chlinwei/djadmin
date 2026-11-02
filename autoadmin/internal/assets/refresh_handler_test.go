package assets

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func TestRequiredIDListQuery(t *testing.T) {
	tests := []struct {
		query string
		want  []int64
	}{
		{query: "ids=1", want: []int64{1}},
		{query: "ids=1,2,9223372036854775807", want: []int64{1, 2, 9223372036854775807}},
		{query: "ids=%201%20,%202%20", want: []int64{1, 2}},
		{query: ""},
		{query: "ids="},
		{query: "ids=0"},
		{query: "ids=-1"},
		{query: "ids=1,invalid"},
		{query: "ids=1,,2"},
		{query: "ids=1,"},
		{query: "ids=1.5"},
		{query: "ids=9223372036854775808"},
		{query: "ids=1&ids=2"},
		{query: "ids[]=1"},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodGet, "/?"+test.query, nil)
			got, err := requiredIDListQuery(context, "ids")
			if test.want == nil {
				if err == nil {
					t.Fatalf("invalid ID list accepted: %v", got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, %v; want %v", got, err, test.want)
			}
		})
	}
}

func TestBatchRefreshHostInfoRejectsMissingOrInvalidQueryIDs(t *testing.T) {
	for _, query := range []string{"", "?ids=", "?ids=1,bad", "?ids=0", "?ids=1&ids=2"} {
		t.Run(query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			// A GET body must not restore the removed POST query contract.
			context.Request = httptest.NewRequest(http.MethodGet, "/assets/hosts/refresh-info/"+query, strings.NewReader(`{"ids":[1]}`))
			context.Request.Header.Set("Content-Type", "application/json")
			handler := &Handler{}
			handler.BatchRefreshHostInfo(context)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || envelope.Code != 400 {
				t.Fatalf("invalid query was not rejected: %s", recorder.Body.String())
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("live host observation must not be cached")
			}
		})
	}
}
