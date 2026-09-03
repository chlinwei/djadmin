package assets

import (
	"encoding/json"
	"testing"
)

// 回归用例：服务树"资源占比"按 部署→服务→业务系统 归集 CPU/内存，
// Go 版 ListApplicationDeployments 曾不返回 application_service_ids（M2M 关联），
// 导致前端 linkedServices 恒为空、两个饼图无数据。这里固化 application_service_ids
// 字段的 JSON 契约（前端 ServiceTree.vue 以 application_service_ids 读取）。
func TestApplicationDeploymentServiceIDsJSONContract(t *testing.T) {
	item := ApplicationDeployment{ID: 15, Host: 228, ApplicationServiceIDs: []int64{10}}
	payload := string(mustMarshal(item))
	for _, want := range []string{`"application_service_ids":[10]`, `"host":228`, `"id":15`} {
		if !containsSubstring(payload, want) {
			t.Fatalf("payload missing %s: %s", want, payload)
		}
	}
	// 仓库层对未关联服务的部署统一赋 []int64{}（ListApplicationDeployments/attachApplicationServiceIDs），
	// 输出空数组而不是 null，前端 ServiceTree.vue 的 linkedServices 过滤依赖可迭代值。
	item2 := ApplicationDeployment{ID: 1, ApplicationServiceIDs: []int64{}}
	if payload2 := string(mustMarshal(item2)); !containsSubstring(payload2, `"application_service_ids":[]`) {
		t.Fatalf("empty association should marshal to []: %s", payload2)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
