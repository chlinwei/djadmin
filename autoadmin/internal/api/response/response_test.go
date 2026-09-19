package response

import (
	"bytes"
	"errors"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

// 回归（2026-09-19 现场）：错误日志必须说出**是哪一件事**，不能只打一个 `<nil>`。
//
// 现场：菜单保存失败，服务端只留下 `[API-ERROR] PATCH /sys/menus/169/: <nil>`——因为日志只打了
// `errors.Unwrap(appError)`（cause），而 apperror.New 造出来的错误（请求参数错误 / 无权限 /
// 资源不存在 / token 失效）**没有 cause**，Unwrap 就是 nil。人拿着这句日志什么都做不了。
func TestErrorLogsMessageWhenThereIsNoCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buffer := &bytes.Buffer{}
	log.SetOutput(buffer)
	defer log.SetOutput(os.Stderr)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("PATCH", "/sys/menus/169/", nil)

	Error(context, apperror.ErrInvalidRequest)

	line := buffer.String()
	if !strings.Contains(line, "请求参数错误") {
		t.Fatalf("日志里必须有给用户看的那句话，得到 %q", line)
	}
	if !strings.Contains(line, "PATCH /sys/menus/169/") {
		t.Fatalf("日志里要有方法与路径，得到 %q", line)
	}
	if strings.Contains(line, "<nil>") {
		t.Fatalf("不能再打出无意义的 <nil>：%q", line)
	}
}

// 有 cause 时根因不能丢（500 的排查关键），且同样带上那句话。
func TestErrorLogsCauseWhenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buffer := &bytes.Buffer{}
	log.SetOutput(buffer)
	defer log.SetOutput(os.Stderr)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/assets/hosts/", nil)

	Error(context, apperror.WithCause(apperror.ErrInternal, errors.New("Unknown column 'agent_id'")))

	line := buffer.String()
	for _, want := range []string{"服务器内部错误", "Unknown column 'agent_id'"} {
		if !strings.Contains(line, want) {
			t.Fatalf("日志里应包含 %q，得到 %q", want, line)
		}
	}
}
