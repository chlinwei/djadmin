package logcollect

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type liveSource struct {
	ServiceID string `json:"service_id"`
	HostIP    string `json:"host_ip"`
	Instance  string `json:"instance"`
	LogName   string `json:"log_name"`
	Path      string `json:"log_path"`
	SharedID  string `json:"shared_source_id"`
}

type liveSlot struct {
	user     int32
	host     string
	hostIP   string
	path     string
	identity string
}

const (
	liveTailMaxFilesPerUser   = 5
	liveTailMaxViewersPerFile = 5
)

type liveTailState struct {
	mu      sync.Mutex
	slots   map[string]liveSlot
	pending chan struct{}
	window  time.Time
	bytes   int
	tokens  *identity.TokenManager
	origins []string
}

// A path retains its file slot across rotation; inode aliases share that slot.
// Unioning both identities avoids counting old/new rotation descriptors twice,
// or treating an additional viewer of an already admitted path as a new file.
func liveFileGroups(slots []liveSlot) []string {
	parent := map[string]string{}
	root := func(key string) string {
		if _, ok := parent[key]; !ok {
			parent[key] = key
		}
		for parent[key] != key {
			parent[key] = parent[parent[key]]
			key = parent[key]
		}
		return key
	}
	pathKey := func(slot liveSlot) string { return "path:" + slot.host + ":" + slot.path }
	for _, slot := range slots {
		pathRoot := root(pathKey(slot))
		if slot.identity != "" {
			inodeRoot := root("inode:" + slot.host + ":" + slot.identity)
			parent[inodeRoot] = pathRoot
		}
	}
	groups := make([]string, len(slots))
	for i, slot := range slots {
		groups[i] = root(pathKey(slot))
	}
	return groups
}

func liveFileCounts(slots []liveSlot) (int, map[string]int) {
	seen := map[string]bool{}
	hosts := map[string]int{}
	for i, key := range liveFileGroups(slots) {
		if !seen[key] {
			seen[key] = true
			hosts[slots[i].host]++
		}
	}
	return len(seen), hosts
}

func (handler *Handler) ConfigureLiveTail(tokens *identity.TokenManager, allowedOrigins []string) {
	handler.liveTail = &liveTailState{
		tokens: tokens, origins: append([]string(nil), allowedOrigins...),
		slots: map[string]liveSlot{}, pending: make(chan struct{}, 32),
	}
}

// Reservations include starting readers. Both the configured path and physical
// identity participate: aliases cannot evade limits, nor can rotation free the
// logical path while another viewer still owns it.
func (state *liveTailState) reserve(id string, slot liveSlot) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	userCount, fileCount, hostCount := 0, 0, 0
	allSlots := make([]liveSlot, 0, len(state.slots)+1)
	for currentID, current := range state.slots {
		if currentID == id {
			continue
		}
		allSlots = append(allSlots, current)
		if current.user == slot.user {
			userCount++
		}
		if current.host == slot.host {
			hostCount++
		}
	}
	allSlots = append(allSlots, slot)
	// Use the same transitive path/inode group as status and file capacity:
	// rotation can otherwise let an alias of the old inode evade the file limit.
	groups := liveFileGroups(allSlots)
	for i, current := range allSlots[:len(allSlots)-1] {
		if groups[i] == groups[len(groups)-1] {
			fileCount++
			if current.user == slot.user {
				return fmt.Errorf("你已在其他窗口跟踪该文件")
			}
		}
	}
	fileCountTotal, hostFiles := liveFileCounts(allSlots)
	if fileCount >= liveTailMaxViewersPerFile {
		return fmt.Errorf("该文件跟踪已满：最多 %d 个用户同时跟踪", liveTailMaxViewersPerFile)
	}
	if userCount >= liveTailMaxFilesPerUser {
		return fmt.Errorf("已达个人跟踪上限：最多同时跟踪 %d 个文件", liveTailMaxFilesPerUser)
	}
	_, updating := state.slots[id]
	if hostCount >= 20 || hostFiles[slot.host] > 10 ||
		(!updating && len(state.slots) >= 100) || fileCountTotal > 50 {
		return fmt.Errorf("实时跟踪容量已满")
	}
	state.slots[id] = slot
	return nil
}

func (state *liveTailState) release(id string) {
	state.mu.Lock()
	delete(state.slots, id)
	state.mu.Unlock()
}

func (state *liveTailState) allow(n int) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if time.Since(state.window) >= time.Second {
		state.window, state.bytes = time.Now(), 0
	}
	// JSON base64 expansion remains below the 10 MiB/s outbound ceiling.
	if state.bytes+n > 5*1024*1024 {
		return false
	}
	state.bytes += n
	return true
}

func (state *liveTailState) allowOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	parsed, err := url.Parse(origin)
	if err != nil || origin == "" || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Host == request.Host {
		return true
	}
	// Use the same parsed configuration as HTTP CORS, including its wildcard.
	// This only admits the handshake: JWT and raw-file authorization still run
	// before starting any reader, and no cookie or URL token authenticates it.
	for _, allowed := range state.origins {
		if strings.TrimSpace(allowed) == "*" || strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}

func (handler *Handler) authorizeLiveTail(ctx context.Context, claims *identity.Claims) error {
	if claims == nil || claims.ExpiresAt == nil || time.Now().After(claims.ExpiresAt.Time) {
		return fmt.Errorf("登录已过期")
	}
	queries := db.New(handler.db)
	user, err := queries.GetUserByID(ctx, claims.UserID)
	if err != nil || user.Status == 0 || user.Username != claims.Username {
		return fmt.Errorf("账号不存在或已停用")
	}
	if user.Username == "admin" {
		return nil
	}
	perms, err := queries.ListPermissionCodesByUserID(ctx, claims.UserID)
	if err != nil {
		return err
	}
	// Reuse the log-center grant instead of requiring unrelated asset menus.
	// Source/deployment/path checks still run independently before file access.
	for _, perm := range perms {
		if perm.Valid && perm.String == "monitor:view" {
			return nil
		}
	}
	return fmt.Errorf("缺少日志中心查看权限（monitor:view）")
}

func validateLiveSource(source liveSource) (int64, error) {
	serviceID, err := strconv.ParseInt(source.ServiceID, 10, 64)
	if err != nil || serviceID < 1 || strconv.FormatInt(serviceID, 10) != source.ServiceID ||
		strings.TrimSpace(source.HostIP) == "" || strings.TrimSpace(source.Instance) == "" ||
		strings.TrimSpace(source.LogName) == "" || source.SharedID != "" || !filepath.IsAbs(source.Path) ||
		filepath.Clean(source.Path) != source.Path || hasGlobMeta(source.Path) || len(source.Path) > 4096 {
		return 0, fmt.Errorf("无法可靠定位源文件：缺少服务/实例/主机 IP/日志名称、路径无效或记录带有共享源标记")
	}
	return serviceID, nil
}

func resolveLiveSource(source liveSource, entries []hostLogRenderInput, instances []hostInstanceInput) error {
	serviceID, err := validateLiveSource(source)
	if err != nil {
		return err
	}
	matches, authorized := 0, false
	for _, instance := range instances {
		for _, entry := range entries {
			if entry.ServiceID != instance.ServiceID || strings.TrimSpace(entry.Pipeline) == "" || strings.TrimSpace(entry.Tier) == "" {
				continue
			}
			pattern := resolveMacros(entry.ResolvedPath, entry.Macros, instance.Macros)
			if strings.Contains(pattern, "${") || strings.Contains(pattern, "**") || filepath.Clean(pattern) != pattern {
				continue
			}
			matched, matchErr := filepath.Match(pattern, source.Path)
			if matchErr != nil || !matched {
				continue
			}
			matches++
			if entry.ServiceID == serviceID && instance.Instance == source.Instance && entry.LogName == source.LogName {
				authorized = true
			}
		}
	}
	if matches != 1 || !authorized {
		return fmt.Errorf("无法可靠定位源文件：当前部署/采集配置不存在、已改变或归属有歧义")
	}
	return nil
}

func (handler *Handler) resolveLiveTail(ctx context.Context, source liveSource) (db.ListLiveTailHostTargetsRow, error) {
	var target db.ListLiveTailHostTargetsRow
	serviceID, err := validateLiveSource(source)
	if err != nil {
		return target, err
	}
	queries := db.New(handler.db)
	// Application instance names are unique per host, not globally. Resolve the
	// existing event fields through the active deployment instead of trusting a
	// browser-supplied host ID or confusing the app name with the Agent identity.
	targets, err := queries.ListLiveTailHostTargets(ctx, db.ListLiveTailHostTargetsParams{
		ServiceID: serviceID, InstanceName: source.Instance,
		HostIp: sql.NullString{String: source.HostIP, Valid: true},
	})
	if err != nil {
		return target, fmt.Errorf("关联日志部署实例失败：%w", err)
	}
	if len(targets) != 1 {
		return target, fmt.Errorf("无法可靠定位源文件：服务、实例名与主机 IP 未唯一匹配当前启用的部署")
	}
	target = targets[0]
	if target.HostID < 1 || target.HostInstanceName == "" {
		return target, fmt.Errorf("无法可靠定位源文件：主机未配置 agent 身份")
	}
	duplicates, err := queries.CountOtherHostsByInstanceName(ctx, db.CountOtherHostsByInstanceNameParams{
		InstanceName: sql.NullString{String: target.HostInstanceName, Valid: true}, ExcludeID: target.HostID,
	})
	if err != nil || duplicates != 0 {
		return target, fmt.Errorf("无法可靠定位源文件：主机 agent 路由身份有歧义或查询失败")
	}
	entries, instances, err := handler.loadHostLogRenderInput(ctx, target.HostID)
	if err != nil {
		return target, err
	}
	if err = resolveLiveSource(source, entries, instances); err != nil {
		return target, err
	}
	return target, nil
}

func (handler *Handler) liveTailUnavailableReason() string {
	// Quotas remain process-local: multi-API deployments must explicitly disable
	// this feature until session ownership and global admission are implemented.
	mode := strings.TrimSpace(os.Getenv("LOG_LIVE_TAIL_MODE"))
	switch mode {
	case "", "single-api-experimental":
	case "disabled":
		return "源日志实时跟踪已关闭"
	default:
		return "LOG_LIVE_TAIL_MODE 配置无效：允许留空、single-api-experimental 或 disabled"
	}
	if handler.liveTail == nil {
		return "源日志实时跟踪尚未初始化"
	}
	return ""
}

func (handler *Handler) LiveTail(socketContext *gin.Context) {
	if unavailable := handler.liveTailUnavailableReason(); unavailable != "" {
		socketContext.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": unavailable, "data": nil})
		return
	}
	state := handler.liveTail
	select {
	case state.pending <- struct{}{}:
	default:
		socketContext.AbortWithStatus(http.StatusTooManyRequests)
		return
	}
	pending := true
	defer func() {
		if pending {
			<-state.pending
		}
	}()
	upgrader := websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 32768, CheckOrigin: state.allowOrigin}
	socket, err := upgrader.Upgrade(socketContext.Writer, socketContext.Request, nil)
	if err != nil {
		return
	}
	defer socket.Close()
	write := func(event any) error {
		_ = socket.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return socket.WriteJSON(event)
	}
	fail := func(err error) {
		_ = write(gin.H{"type": "error", "message": err.Error()})
	}
	// JWT travels only in the first bounded frame, never in URLs or access logs.
	socket.SetReadLimit(8192)
	_ = socket.SetReadDeadline(time.Now().Add(5 * time.Second))
	var auth struct {
		Token  string     `json:"token"`
		Source liveSource `json:"source"`
	}
	if err = socket.ReadJSON(&auth); err != nil {
		return
	}
	claims, err := state.tokens.Parse(strings.TrimPrefix(auth.Token, "Bearer "))
	auth.Token = ""
	if err != nil {
		fail(fmt.Errorf("登录已失效"))
		return
	}
	ctx, cancel := context.WithTimeout(socketContext.Request.Context(), 30*time.Minute)
	defer cancel()
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	err = handler.authorizeLiveTail(checkCtx, claims)
	var target db.ListLiveTailHostTargetsRow
	if err == nil {
		target, err = handler.resolveLiveTail(checkCtx, auth.Source)
	}
	checkCancel()
	if err != nil {
		fail(err)
		return
	}
	id := uuid.NewString()
	slot := liveSlot{user: claims.UserID, host: strconv.FormatInt(target.HostID, 10), hostIP: auth.Source.HostIP, path: auth.Source.Path}
	if err = state.reserve(id, slot); err != nil {
		fail(err)
		return
	}
	defer state.release(id)
	<-state.pending
	pending = false
	conn, err := handler.gateway.OpenTail(target.HostInstanceName, id, auth.Source.Path)
	if err != nil {
		fail(err)
		return
	}
	defer conn.Close()
	reason := "browser disconnected"
	slog.Info("live_tail_start", "id", id, "user_id", claims.UserID, "host_id", slot.host, "path", slot.path)
	defer func() {
		slog.Info("live_tail_end", "id", id, "user_id", claims.UserID, "host_id", slot.host, "path", slot.path, "reason", reason)
	}()
	// Transport ping/pong is independent of page timers and background-tab
	// rendering. The reader never writes; all writes belong to this goroutine.
	_ = socket.SetReadDeadline(time.Now().Add(45 * time.Second))
	socket.SetPongHandler(func(string) error {
		return socket.SetReadDeadline(time.Now().Add(45 * time.Second))
	})
	stopReason := make(chan string, 1)
	go func() {
		defer cancel()
		for {
			var message struct {
				Type string `json:"type"`
			}
			if socket.ReadJSON(&message) != nil {
				stopReason <- "browser disconnected"
				return
			}
			if message.Type == "stop" {
				stopReason <- "user stopped"
				return
			}
		}
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	startTimer := time.NewTimer(8 * time.Second)
	defer startTimer.Stop()
	ackTimer := time.NewTimer(45 * time.Second)
	defer ackTimer.Stop()
	for {
		select {
		case <-ctx.Done():
			select {
			case reason = <-stopReason:
			default:
				if ctx.Err() == context.DeadlineExceeded {
					reason = "session duration expired"
				} else {
					reason = "request cancelled"
				}
			}
			_ = write(gin.H{"type": "end", "message": "跟踪已停止或达到 30 分钟上限；断线期间可能有缺口"})
			return
		case <-startTimer.C:
			reason = "agent start timeout (agent upgrade may be required)"
			fail(fmt.Errorf("%s", reason))
			return
		case <-ackTimer.C:
			reason = "agent lease acknowledgement timeout; possible gap"
			fail(fmt.Errorf("%s", reason))
			return
		case <-ticker.C:
			checkCtx, checkCancel = context.WithTimeout(ctx, 5*time.Second)
			err = handler.authorizeLiveTail(checkCtx, claims)
			var current db.ListLiveTailHostTargetsRow
			if err == nil {
				current, err = handler.resolveLiveTail(checkCtx, auth.Source)
			}
			checkCancel()
			if err != nil || current != target {
				reason = "authorization or source changed"
				fail(fmt.Errorf("授权或日志源配置已失效，已停止"))
				return
			}
			if err = conn.Command("renew", "", ""); err == nil {
				err = socket.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			}
			if err != nil {
				reason = err.Error()
				return
			}
		case event, ok := <-conn.Events:
			if !ok {
				reason = "agent disconnected or output queue full; possible gap"
				fail(fmt.Errorf("%s", reason))
				return
			}
			if len(event.Data) > 32*1024 || !state.allow(len(event.Data)) {
				reason = "live-tail rate exceeded; possible gap"
				fail(fmt.Errorf("%s", reason))
				return
			}
			if event.Type == "lease" {
				ackTimer.Reset(45 * time.Second)
				continue
			}
			if event.Type == "ready" {
				startTimer.Stop()
				ackTimer.Reset(45 * time.Second)
				slot.identity = event.Identity
				if slot.identity == "" {
					err = fmt.Errorf("agent did not supply file identity")
				} else {
					err = state.reserve(id, slot)
				}
				if err == nil {
					err = conn.Command("start", "", slot.identity)
				}
				if err != nil {
					reason = err.Error()
					fail(err)
					return
				}
			}
			if err = write(gin.H{"type": event.Type, "data": event.Data, "message": event.Message}); err != nil {
				reason = "browser send timeout or disconnect"
				return
			}
			if event.Type == "end" || event.Type == "error" {
				reason = event.Message
				return
			}
		}
	}
}
