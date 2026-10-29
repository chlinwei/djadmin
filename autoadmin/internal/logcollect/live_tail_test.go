package logcollect

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLiveTailQuotasAtomicAndAliases(t *testing.T) {
	state := &liveTailState{slots: map[string]liveSlot{}}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 1; i <= 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if state.reserve(fmt.Sprint(i), liveSlot{user: int32(i), host: "1", path: fmt.Sprint("/alias", i), identity: "1:123"}) == nil {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 5 {
		t.Fatalf("accepted %d users, want 5", accepted.Load())
	}
	state = &liveTailState{slots: map[string]liveSlot{}}
	for i := 0; i < 5; i++ {
		if err := state.reserve(fmt.Sprint(i), liveSlot{user: 1, host: "1", path: fmt.Sprint("/log", i), identity: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.reserve("six", liveSlot{user: 1, host: "2", path: "/six"}); err == nil {
		t.Fatal("sixth user file admitted")
	}
	state.release("0")
	if err := state.reserve("six", liveSlot{user: 1, host: "2", path: "/six"}); err != nil {
		t.Fatal(err)
	}
	if err := state.reserve("duplicate", liveSlot{user: 1, host: "1", path: "/other-alias", identity: "1"}); err == nil {
		t.Fatal("same user physical alias admitted")
	}
}

func TestLiveTailFullFileCapacityAllowsExistingViewersAndRotation(t *testing.T) {
	state := &liveTailState{slots: map[string]liveSlot{}}
	for i := 0; i < 50; i++ {
		slot := liveSlot{user: int32(i + 1), host: fmt.Sprint(i / 10), path: fmt.Sprint("/log", i), identity: fmt.Sprint(i)}
		if err := state.reserve(fmt.Sprint(i), slot); err != nil {
			t.Fatal(err)
		}
	}
	slot := liveSlot{user: 51, host: "0", path: "/log0"}
	if err := state.reserve("viewer", slot); err != nil {
		t.Fatalf("existing path should not need a new file slot: %v", err)
	}
	slot.identity = "rotated"
	if err := state.reserve("viewer", slot); err != nil {
		t.Fatalf("rotation should retain logical file slot: %v", err)
	}
	if err := state.reserve("new-file", liveSlot{user: 52, host: "6", path: "/new"}); err == nil {
		t.Fatal("51st distinct file admitted")
	}
}

func TestResolveLiveSourceFailsClosed(t *testing.T) {
	source := liveSource{ServiceID: "7", HostIP: "10.0.0.1", Instance: "app", LogName: "main", Path: "/logs/app.log"}
	entry := hostLogRenderInput{ServiceID: 7, LogName: "main", ResolvedPath: "/logs/*.log", Pipeline: "rule", Tier: "std"}
	instance := hostInstanceInput{ServiceID: 7, Instance: "app"}
	if err := resolveLiveSource(source, []hostLogRenderInput{entry}, []hostInstanceInput{instance}); err != nil {
		t.Fatal(err)
	}
	cases := []func(*liveSource){
		func(s *liveSource) { s.ServiceID = "07" },
		func(s *liveSource) { s.ServiceID = "" },
		func(s *liveSource) { s.HostIP = "" },
		func(s *liveSource) { s.Instance = "" },
		func(s *liveSource) { s.Instance = "other" },
		func(s *liveSource) { s.LogName = "" },
		func(s *liveSource) { s.Path = "/etc/passwd" },
		func(s *liveSource) { s.Path = "/logs/../logs/app.log" },
		func(s *liveSource) { s.SharedID = "2" },
	}
	for _, mutate := range cases {
		invalid := source
		mutate(&invalid)
		if err := resolveLiveSource(invalid, []hostLogRenderInput{entry}, []hostInstanceInput{instance}); err == nil {
			t.Fatalf("accepted invalid source %+v", invalid)
		}
	}
	if err := resolveLiveSource(source, []hostLogRenderInput{entry, entry}, []hostInstanceInput{instance}); err == nil {
		t.Fatal("ambiguous path admitted")
	}
}

func TestLiveTailOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://not-the-injected-origin.example")
	for _, tc := range []struct {
		name, origin string
		allowed      []string
		want         bool
	}{
		{name: "wildcard LAN", origin: "http://10.25.66.150:5173", allowed: []string{"*"}, want: true},
		{name: "wildcard localhost", origin: "http://localhost:5173", allowed: []string{"*"}, want: true},
		{name: "listed", origin: "https://ui.example", allowed: []string{"https://ui.example"}, want: true},
		{name: "unlisted", origin: "https://other.example", allowed: []string{"https://ui.example"}},
		{name: "same host", origin: "http://api.example", want: true},
		{name: "empty", allowed: []string{"*"}},
		{name: "opaque", origin: "null", allowed: []string{"*"}},
		{name: "missing host", origin: "http://", allowed: []string{"*"}},
		{name: "credentials", origin: "http://user:pass@ui.example", allowed: []string{"*"}},
		{name: "path", origin: "http://ui.example/path", allowed: []string{"*"}},
		{name: "query", origin: "http://ui.example?q=1", allowed: []string{"*"}},
		{name: "invalid scheme", origin: "file://ui.example", allowed: []string{"*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://api.example/ws/monitor/log-tail/", nil)
			req.Header.Set("Origin", tc.origin)
			state := &liveTailState{origins: tc.allowed}
			if got := state.allowOrigin(req); got != tc.want {
				t.Fatalf("origin %q got %v", tc.origin, got)
			}
		})
	}
}

func TestLiveTailMetadataRendered(t *testing.T) {
	rendered := renderHostLogConfig([]hostLogRenderInput{{
		ServiceID: 7, Service: "svc", LogName: "main", ResolvedPath: "/logs/app.log", Pipeline: "rule", Tier: "std",
	}}, []hostInstanceInput{{ServiceID: 7, Instance: "app"}}, "")
	if len(rendered.Fragments) != 1 {
		t.Fatalf("fragments: %+v", rendered)
	}
	content := rendered.Fragments[0].Content
	if !strings.Contains(content, "    service_id: '7'") || !strings.Contains(content, "    instance: 'app'") {
		t.Fatal("missing service/instance association fields")
	}
	if strings.Contains(content, "host_id:") {
		t.Fatal("host ID must be resolved from deployment, not injected into logs")
	}
}
