package logcollect

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestElasticsearchRequestClosesIdleConnections(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			closed := make(chan struct{}, 3)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				if !ok || username != "monitor" || password != "test-password" {
					t.Error("Elasticsearch request did not preserve basic authentication")
				}
				if r.URL.Path != "/_cluster/health" {
					t.Errorf("unexpected request path: %s", r.URL.Path)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"status":"green"}`))
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateClosed {
					closed <- struct{}{}
				}
			}
			server.StartTLS()
			defer server.Close()

			handler := &Handler{}
			cluster := elasticsearchCluster{
				Hosts: server.URL, Username: "monitor", Password: "test-password", Timeout: 2,
			}
			for i := 0; i < 3; i++ {
				payload, err := handler.elasticsearchRequestRaw(context.Background(), cluster, http.MethodGet, "/_cluster/health", nil)
				if status == http.StatusOK {
					if err != nil {
						t.Fatal(err)
					}
					if string(payload) != `{"status":"green"}` {
						t.Fatalf("unexpected response: %s", payload)
					}
				} else if err == nil {
					t.Fatal("expected Elasticsearch status error")
				}
				select {
				case <-closed:
				case <-time.After(2 * time.Second):
					t.Fatal("request returned but its private transport kept the connection open")
				}
			}
		})
	}
}
