package assets

import (
	stdcontext "context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

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
	if deploymentGateway == nil {
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
	if host.AgentID == nil || strings.TrimSpace(*host.AgentID) == "" {
		context.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "主机未绑定 Agent", "data": nil})
		return
	}
	socket, err := webSSHUpgrader.Upgrade(context.Writer, context.Request, nil)
	if err != nil {
		return
	}
	defer socket.Close()
	requestID := uuid.NewString()
	targetUser := strings.TrimSpace(context.Query("target_user"))
	events, err := deploymentGateway.OpenTerminal(context.Request.Context(), *host.AgentID, requestID, targetUser, 120, 32)
	if err != nil {
		_ = socket.WriteJSON(gin.H{"type": "error", "message": err.Error()})
		return
	}
	writeMu := make(chan struct{}, 1)
	writeMu <- struct{}{}
	writeJSON := func(value any) error {
		<-writeMu
		defer func() { writeMu <- struct{}{} }()
		return socket.WriteJSON(value)
	}
	ctx, cancel := stdcontext.WithCancel(stdcontext.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-events:
				if !ok {
					return
				}
				switch {
				case frame.GetTerminalOpenResponse() != nil:
					response := frame.GetTerminalOpenResponse()
					if response.Error != "" {
						_ = writeJSON(gin.H{"type": "error", "message": response.Error})
					} else {
						homeDir := "/"
						if response.EffectiveUser == "root" {
							homeDir = "/root"
						}
						_ = writeJSON(gin.H{
							"type": "connected", "requested_user": targetUser,
							"effective_user": response.EffectiveUser, "switch_user_status": "success",
							"instance_name": webSSHString(host.InstanceName), "ip": webSSHString(host.IP),
							"supports_file_ops": true, "home_dir": homeDir,
						})
					}
				case frame.GetTerminalDataResponse() != nil:
					_ = writeJSON(gin.H{"type": "output", "data": string(frame.GetTerminalDataResponse().Data)})
				case frame.GetTerminalExitResponse() != nil:
					exit := frame.GetTerminalExitResponse()
					_ = writeJSON(gin.H{"type": "exit", "exit_code": exit.ExitCode, "error": exit.Error})
					return
				}
			}
		}
	}()
	for {
		var message webSSHMessage
		if err = socket.ReadJSON(&message); err != nil {
			break
		}
		switch strings.ToLower(message.Type) {
		case "resize":
			_ = deploymentGateway.ResizeTerminal(context.Request.Context(), *host.AgentID, requestID, message.Cols, message.Rows)
		case "input", "data":
			_ = deploymentGateway.SendTerminalData(context.Request.Context(), *host.AgentID, requestID, []byte(message.Data))
		case "close":
			_ = deploymentGateway.CloseTerminal(context.Request.Context(), *host.AgentID, requestID)
			return
		}
	}
	closeCtx, cancelClose := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
	defer cancelClose()
	_ = deploymentGateway.CloseTerminal(closeCtx, *host.AgentID, requestID)
}

func webSSHString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
