package logcollect

import (
	"context"
	"sort"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type liveFileStatus struct {
	HostIP      string `json:"host_ip"`
	Path        string `json:"log_path"`
	Viewers     int    `json:"viewers"`
	CurrentUser bool   `json:"current_user"`
}

type liveTailStatus struct {
	MaxViewers int              `json:"max_viewers"`
	MaxFiles   int              `json:"max_files"`
	UserFiles  int              `json:"user_files"`
	Files      []liveFileStatus `json:"files"`
}

func (state *liveTailState) status(user int32) liveTailStatus {
	state.mu.Lock()
	slots := make([]liveSlot, 0, len(state.slots))
	for _, slot := range state.slots {
		slots = append(slots, slot)
	}
	state.mu.Unlock()

	status := liveTailStatus{
		MaxViewers: liveTailMaxViewersPerFile, MaxFiles: liveTailMaxFilesPerUser,
		Files: make([]liveFileStatus, 0),
	}
	groups := liveFileGroups(slots)
	viewers := map[string]map[int32]bool{}
	for i, slot := range slots {
		if viewers[groups[i]] == nil {
			viewers[groups[i]] = map[int32]bool{}
		}
		viewers[groups[i]][slot.user] = true
		if slot.user == user {
			status.UserFiles++
		}
	}
	seen := map[[2]string]bool{}
	for i, slot := range slots {
		key := [2]string{slot.hostIP, slot.path}
		if seen[key] {
			continue
		}
		seen[key] = true
		status.Files = append(status.Files, liveFileStatus{
			HostIP: slot.hostIP, Path: slot.path,
			Viewers: len(viewers[groups[i]]), CurrentUser: viewers[groups[i]][user],
		})
	}
	sort.Slice(status.Files, func(i, j int) bool {
		if status.Files[i].HostIP != status.Files[j].HostIP {
			return status.Files[i].HostIP < status.Files[j].HostIP
		}
		return status.Files[i].Path < status.Files[j].Path
	})
	return status
}

// LiveTailStatus shares the log center's global monitor:view grant and rechecks
// the account and permissions like the reader. Only already-authorized active
// paths/counts are exposed: no reader IDs, other users' identities or contents.
// One bounded snapshot avoids resolving every historical ES row on each poll.
func (handler *Handler) LiveTailStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	claims, ok := identity.ClaimsFromContext(c)
	if !ok {
		response.Error(c, apperror.ErrTokenInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := handler.authorizeLiveTail(ctx, claims); err != nil {
		response.Error(c, apperror.New(apperror.CodePermissionDenied, err.Error()))
		return
	}
	if reason := handler.liveTailUnavailableReason(); reason != "" {
		response.Error(c, apperror.New(apperror.CodeInvalidArgument, reason))
		return
	}
	response.Success(c, handler.liveTail.status(claims.UserID))
}
