package assets

import (
	stdcontext "context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var webSSHUpgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Host == request.Host {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}}

type webSSHMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint32 `json:"cols"`
	Rows uint32 `json:"rows"`
}

func (handler *Handler) WebSSH(context *gin.Context) {
	if handler.gateway == nil {
		context.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "Agent Gateway 未启用", "data": nil})
		return
	}
	hostID, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || hostID < 1 {
		context.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "主机 ID 无效", "data": nil})
		return
	}
	host, err := handler.service.GetHost(context.Request.Context(), hostID)
	if err != nil {
		respond(context, nil, err)
		return
	}
	if host.InstanceName == nil || strings.TrimSpace(*host.InstanceName) == "" {
		context.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "主机未配置实例名，无法定位 agent", "data": nil})
		return
	}
	claims, ok := identity.ClaimsFromContext(context)
	if !ok {
		respond(context, nil, apperror.ErrTokenInvalid)
		return
	}
	targetUser := strings.TrimSpace(context.Query("target_user"))
	if len(targetUser) > 100 {
		respond(context, nil, apperror.New(apperror.CodeInvalidArgument, "目标用户名过长"))
		return
	}
	socket, err := webSSHUpgrader.Upgrade(context.Writer, context.Request, nil)
	if err != nil {
		return
	}
	defer socket.Close()
	// Refuse an unaudited terminal: persist the identity before opening the PTY.
	// Final writes use their own deadline because browser disconnect cancels HTTP.
	createCtx, cancel := stdcontext.WithTimeout(context.Request.Context(), webSSHAuditWriteTimeout)
	audit, err := newWebSSHAudit(createCtx, handler.service.repository.queries, db.CreateWebSSHSessionParams{
		HostID: hostID, UserID: sql.NullInt32{Int32: claims.UserID, Valid: true},
		Username: claims.Username, ClientIp: webSSHText(context.ClientIP(), 64),
		UserAgent: webSSHText(context.Request.UserAgent(), 255), StartTime: time.Now().UTC(),
		RequestedUsername: targetUser,
	})
	cancel()
	if err != nil {
		slog.Error("webssh audit create failed", "host_id", hostID, "user_id", claims.UserID, "error", err)
		_ = writeWebSSH(socket, gin.H{"type": "error", "message": "无法创建 WebSSH 会话审计记录，连接已停止"})
		return
	}
	bridgeWebSSH(context.Request.Context(), socket, handler.gateway, host, targetUser, audit)
	audit.finish()
	finalCtx, cancelFinal := stdcontext.WithTimeout(stdcontext.Background(), webSSHAuditWriteTimeout)
	defer cancelFinal()
	if err := audit.flush(finalCtx); err != nil {
		slog.Error("webssh audit finalize failed", "session_id", audit.row.ID, "host_id", hostID, "error", err)
		_ = writeWebSSH(socket, gin.H{"type": "error", "message": "WebSSH 已停止，但会话审计保存失败，请联系管理员"})
	}
}

type webSSHTerminalGateway interface {
	OpenTerminal(stdcontext.Context, string, string, string, uint32, uint32) (<-chan *pb.AgentFrame, error)
	SendTerminalData(stdcontext.Context, string, string, []byte) error
	ResizeTerminal(stdcontext.Context, string, string, uint32, uint32) error
	CloseTerminal(stdcontext.Context, string, string) error
}

type webSSHClientEvent struct {
	message webSSHMessage
	err     error
}

func writeWebSSH(socket *websocket.Conn, value any) error {
	if err := socket.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return socket.WriteJSON(value)
}

// One loop serializes terminal events, input and audit snapshots; only the
// socket reader runs separately. Agent exit must also end the browser session.
func bridgeWebSSH(parent stdcontext.Context, socket *websocket.Conn, gateway webSSHTerminalGateway, host Host, targetUser string, audit *webSSHAudit) {
	ctx, cancel := stdcontext.WithCancel(parent)
	defer cancel()
	fail := func(message string) {
		audit.fail(message)
		_ = writeWebSSH(socket, gin.H{"type": "error", "message": message})
	}
	requestID := uuid.NewString()
	agentName := webSSHString(host.InstanceName)
	events, err := gateway.OpenTerminal(ctx, agentName, requestID, targetUser, 120, 32)
	if err != nil {
		fail(err.Error())
		return
	}
	defer func() {
		closeCtx, cancelClose := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
		defer cancelClose()
		if err := gateway.CloseTerminal(closeCtx, agentName, requestID); err != nil {
			slog.Warn("webssh terminal close failed", "session_id", audit.row.ID, "error", err)
			if audit.row.Status != "failed" {
				fail("关闭远端终端失败：" + err.Error())
			}
		}
	}()
	clientEvents := make(chan webSSHClientEvent)
	go func() {
		for {
			var message webSSHMessage
			err := socket.ReadJSON(&message)
			select {
			case <-ctx.Done():
				return
			case clientEvents <- webSSHClientEvent{message: message, err: err}:
			}
			if err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(webSSHAuditFlushInterval)
	defer ticker.Stop()
	flush := func() bool {
		writeCtx, cancelWrite := stdcontext.WithTimeout(ctx, webSSHAuditWriteTimeout)
		defer cancelWrite()
		if err := audit.flush(writeCtx); err != nil {
			slog.Error("webssh audit update failed", "session_id", audit.row.ID, "error", err)
			fail("WebSSH 会话审计保存失败，连接已停止")
			return false
		}
		return true
	}
	for {
		select {
		case <-ctx.Done():
			fail("WebSSH 请求已取消")
			return
		case <-ticker.C:
			if !flush() {
				return
			}
		case event := <-clientEvents:
			if event.err != nil {
				code := websocket.CloseAbnormalClosure
				if closeError, ok := event.err.(*websocket.CloseError); ok {
					code = closeError.Code
				}
				if code != websocket.CloseNormalClosure && code != websocket.CloseGoingAway {
					audit.fail("WebSSH 客户端连接异常中断")
				}
				audit.row.CloseCode = sql.NullInt32{Int32: int32(code), Valid: true}
				return
			}
			message := event.message
			switch strings.ToLower(message.Type) {
			case "resize":
				err = gateway.ResizeTerminal(ctx, agentName, requestID, message.Cols, message.Rows)
			case "input", "data":
				err = gateway.SendTerminalData(ctx, agentName, requestID, []byte(message.Data))
				if err == nil {
					audit.recordInput(message.Data)
				}
			case "close":
				return
			}
			if err != nil {
				fail("终端操作失败：" + err.Error())
				return
			}
		case frame, ok := <-events:
			if !ok {
				fail("Agent 终端连接中断")
				return
			}
			var message gin.H
			switch {
			case frame.GetTerminalOpenResponse() != nil:
				response := frame.GetTerminalOpenResponse()
				if response.Error != "" {
					fail(response.Error)
					return
				}
				audit.connected(response.EffectiveUser)
				if !flush() {
					return
				}
				homeDir := "/"
				if response.EffectiveUser == "root" {
					homeDir = "/root"
				}
				message = gin.H{
					"type": "connected", "requested_user": targetUser,
					"effective_user": response.EffectiveUser, "switch_user_status": "success",
					"instance_name": agentName, "ip": webSSHString(host.IP),
					"supports_file_ops": true, "home_dir": homeDir,
				}
			case frame.GetTerminalDataResponse() != nil:
				data := frame.GetTerminalDataResponse().Data
				audit.recordOutput(data)
				message = gin.H{"type": "output", "data": string(data)}
			case frame.GetTerminalExitResponse() != nil:
				exit := frame.GetTerminalExitResponse()
				if exit.Error != "" || exit.ExitCode != 0 {
					audit.fail(fmt.Sprintf("终端退出（code=%d）：%s", exit.ExitCode, exit.Error))
				}
				_ = writeWebSSH(socket, gin.H{"type": "exit", "exit_code": exit.ExitCode, "error": exit.Error})
				return
			}
			if message != nil {
				if err := writeWebSSH(socket, message); err != nil {
					audit.fail("WebSSH 终端数据发送失败")
					return
				}
			}
		}
	}
}

func webSSHString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
