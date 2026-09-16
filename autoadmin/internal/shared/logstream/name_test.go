package logstream

import "testing"

// 流名段序：项目-业务系统-环境-服务-档位（业务系统段在环境段前）；
// prefix 为空回落 "logs"。
func TestName(t *testing.T) {
	if got := Name("", "kul", "test", "tib", "tomcat-svc", "hot"); got != "logs-kul-tib-test-tomcat-svc-hot" {
		t.Errorf("Name = %q", got)
	}
	if got := Name("logs", "kul", "test", "tib", "tomcat-svc", "wuhan-test"); got != "logs-kul-tib-test-tomcat-svc-wuhan-test" {
		t.Errorf("Name = %q", got)
	}
}
