package automation

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// 自动化作业日志 WebSocket（GET /ws/automation/jobs/:id/logs/?token=），
// 对应 Django AutomationJobLogConsumer。消息协议（前端 logs/center/controller.js 依赖）：
//   - {type:'snapshot', data:{job_id,status,data:<全量文本>}}   连接建立时
//   - {type:'output',   data:{job_id,status,data:<增量文本>}}   有新日志时
//   - {type:'status',   data:{job_id,status}}                  无新日志但状态变化时
//   - {type:'completed',data:{job_id,status}}                  终态推送后服务端关闭
// 鉴权走 Authenticate 中间件的 ?token= 查询参数；token 无效/作业不存在时直接拒绝升级。
// Django 按「行级 offset」做增量；Go 简化为全量重建+前缀比较：能前缀续传就发增量，
// 历史行被原地改写导致前缀断裂时退回 snapshot 重同步，对前端语义等价。

var jobLogUpgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(request *http.Request) bool {
	return true // 同源由 Cookie 无关的 token 鉴权保障，与 webssh Upgrader 策略一致。
}}

type jobLogSession struct {
	socket  *websocket.Conn
	jobID   int64
	sent    string
	status  string
	polling time.Duration
}

func (handler *Handler) JobLogStream(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	if _, err := handler.jobByID(context, id); err != nil {
		automationResourceError(context, err)
		return
	}
	socket, err := jobLogUpgrader.Upgrade(context.Writer, context.Request, nil)
	if err != nil {
		return
	}
	session := &jobLogSession{socket: socket, jobID: id, polling: handler.jobLogPollInterval(context)}
	// 读协程：消费对端控制帧/关闭事件，写协程通过写错误感知连接断开。
	go func() {
		socket.SetReadLimit(512)
		for {
			if _, _, err := socket.ReadMessage(); err != nil {
				_ = socket.Close()
				return
			}
		}
	}()
	defer socket.Close()
	session.stream(context, handler)
}

func (session *jobLogSession) stream(context *gin.Context, handler *Handler) {
	output, status, err := handler.jobLogTextAndStatus(context, session.jobID)
	if err != nil {
		return
	}
	session.sent, session.status = output, status
	if err := session.send("snapshot", gin.H{"job_id": session.jobID, "status": status, "data": output}); err != nil {
		return
	}
	for {
		time.Sleep(session.polling)
		output, status, err := handler.jobLogTextAndStatus(context, session.jobID)
		if err != nil {
			return
		}
		session.status = status
		switch {
		case output != session.sent && strings.HasPrefix(output, session.sent):
			if err := session.send("output", gin.H{"job_id": session.jobID, "status": status, "data": output[len(session.sent):]}); err != nil {
				return
			}
			session.sent = output
		case output != session.sent:
			// 历史行被原地改写（status/stdout 更新），前缀断裂，整包重同步。
			if err := session.send("snapshot", gin.H{"job_id": session.jobID, "status": status, "data": output}); err != nil {
				return
			}
			session.sent = output
		case status != "":
			if err := session.send("status", gin.H{"job_id": session.jobID, "status": status}); err != nil {
				return
			}
		}
		if status == "success" || status == "failed" || status == "cancelled" {
			_ = session.send("completed", gin.H{"job_id": session.jobID, "status": status})
			_ = session.socket.Close()
			return
		}
	}
}

func (session *jobLogSession) send(eventType string, payload gin.H) error {
	session.socket.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return session.socket.WriteJSON(gin.H{"type": eventType, "data": payload})
}

// jobLogTextAndStatus 按 Django consumer 的拼装格式重建作业统一日志（stdout 去尾换行、
// stderr/error 独立小节），与 HTTP 兜底的 JobLog 略有差异但前端展示等价。
func (handler *Handler) jobLogTextAndStatus(context *gin.Context, id int64) (string, string, error) {
	job, err := handler.jobByID(context, id)
	if err != nil {
		return "", "", err
	}
	rows, err := handler.db.QueryContext(context, `SELECT host_id_snapshot,host_ip_snapshot,status,agent_job_id,stdout,stderr,error_message FROM automation_execution_host_log WHERE job_id=? ORDER BY id`, id)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	var log strings.Builder
	for rows.Next() {
		var hostID sql.NullInt64
		var hostIP, status, agentJobID, stdout, stderr, message string
		if err = rows.Scan(&hostID, &hostIP, &status, &agentJobID, &stdout, &stderr, &message); err != nil {
			return "", "", err
		}
		fmt.Fprintf(&log, "\n\n===== Agent Host #%v (%s) | status=%s | job=%s =====\n", nullableInt(hostID), hostIP, status, agentJobID)
		if stdout != "" {
			log.WriteString(strings.TrimRight(stdout, "\n") + "\n")
		}
		if stderr != "" {
			log.WriteString("[stderr]\n" + strings.TrimRight(stderr, "\n") + "\n")
		}
		if message != "" {
			log.WriteString("[error]\n" + strings.TrimRight(message, "\n") + "\n")
		}
	}
	if err = rows.Err(); err != nil {
		return "", "", err
	}
	status := ""
	if raw, ok := job["status"]; ok && raw != nil {
		status = fmt.Sprint(raw)
	}
	return log.String(), status, nil
}

// jobLogPollInterval 读取轮询间隔配置（秒），缺省 0.5，允许范围 [0.2, 10]。
// Django 会 get_or_create 配置行，Go 只读：未配置时用缺省值，不在读路径写库。
func (handler *Handler) jobLogPollInterval(context *gin.Context) time.Duration {
	interval := 0.5
	var value string
	if err := handler.db.QueryRowContext(context, "SELECT value FROM sys_config WHERE `key`=?", "sys.automation.websocket.job_log_poll_interval_seconds").Scan(&value); err == nil {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(value), 64); parseErr == nil {
			interval = parsed
		}
	}
	if interval < 0.2 {
		interval = 0.2
	}
	if interval > 10 {
		interval = 10
	}
	return time.Duration(interval * float64(time.Second))
}
