package assets

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
	gateway *agent.Gateway
}

func NewHandler(service *Service, gateway *agent.Gateway) *Handler {
	return &Handler{service: service, gateway: gateway}
}

func (h *Handler) applyAgentPresence(host *Host) {
	if host == nil {
		return
	}
	agentID := ""
	if host.AgentID != nil {
		agentID = strings.TrimSpace(*host.AgentID)
	}
	// The active gRPC session is authoritative; database heartbeat state may be stale.
	host.AgentOnline = h.gateway.IsOnline(agentID)
}

func page(c *gin.Context) (pagination.Page, error) {
	number, err := strconv.ParseInt(defaultValue(c.Query("page"), "1"), 10, 32)
	if err != nil || number < 1 {
		return pagination.Page{}, apperror.ErrPageInvalid
	}
	sizeRaw := c.Query("page_size")
	if sizeRaw == "" {
		sizeRaw = c.Query("size")
	}
	size, err := strconv.ParseInt(defaultValue(sizeRaw, "10"), 10, 32)
	if err != nil || size < 1 {
		return pagination.Page{}, apperror.ErrPageSizeInvalid
	}
	return pagination.New(int32(number), int32(size)), nil
}
func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func resourceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.Error(c, apperror.ErrIDInvalid)
		return 0, false
	}
	return id, true
}
func queryID(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" || raw == "0" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, apperror.ErrInvalidRequest
	}
	return value, nil
}
func bind[T any](c *gin.Context) (T, bool) {
	var input T
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, apperror.ErrInvalidRequest)
		return input, false
	}
	return input, true
}
func respond(c *gin.Context, data any, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		if _, ok := apperror.As(err); !ok {
			err = apperror.WithCause(apperror.ErrInternal, err)
		}
		response.Error(c, err)
		return
	}
	response.Success(c, data)
}

func (h *Handler) ListProjects(c *gin.Context) {
	page, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListProjects(c.Request.Context(), c.Query("search"), page)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) GetProject(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetProject(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateProject(c *gin.Context) {
	input, ok := bind[ProjectInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateProject(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateProject(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[ProjectInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateProject(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteProject(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteProject(c.Request.Context(), id))
}

func (h *Handler) ListBusinessSystems(c *gin.Context) {
	page, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListBusinessSystems(c.Request.Context(), c.Query("search"), page)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) GetBusinessSystem(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetBusinessSystem(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateBusinessSystem(c *gin.Context) {
	input, ok := bind[BusinessSystemInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateBusinessSystem(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateBusinessSystem(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[BusinessSystemInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateBusinessSystem(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteBusinessSystem(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteBusinessSystem(c.Request.Context(), id))
}

func (h *Handler) ListEnvironments(c *gin.Context) {
	page, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListEnvironments(c.Request.Context(), c.Query("search"), page)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) GetEnvironment(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetEnvironment(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateEnvironment(c *gin.Context) {
	input, ok := bind[EnvironmentInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateEnvironment(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateEnvironment(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[EnvironmentInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateEnvironment(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteEnvironment(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteEnvironment(c.Request.Context(), id))
}

func (h *Handler) ListCredentials(c *gin.Context) {
	page, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListCredentials(c.Request.Context(), c.Query("search"), page)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) GetCredential(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetCredential(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateCredential(c *gin.Context) {
	input, ok := bind[CredentialInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateCredential(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateCredential(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[CredentialInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateCredential(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteCredential(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteCredential(c.Request.Context(), id))
}

func (h *Handler) ListHostGroups(c *gin.Context) {
	page, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	rows, count, err := h.service.ListHostGroups(c.Request.Context(), c.Query("search"), page)
	if err != nil {
		respond(c, nil, err)
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) GetHostGroup(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetHostGroup(c.Request.Context(), id)
	respond(c, item, err)
}
func (h *Handler) CreateHostGroup(c *gin.Context) {
	input, ok := bind[HostGroupInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateHostGroup(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateHostGroup(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	input, ok := bind[HostGroupInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateHostGroup(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteHostGroup(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteHostGroup(c.Request.Context(), id))
}

func (h *Handler) ListHosts(c *gin.Context) {
	page, err := page(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	groupID, err := queryID(c, "group_id")
	if err != nil {
		response.Error(c, err)
		return
	}
	environmentID, err := queryID(c, "environment")
	if err != nil {
		response.Error(c, err)
		return
	}
	agentStatus := strings.ToLower(strings.TrimSpace(c.Query("agent_status")))
	if agentStatus != "" && agentStatus != "all" && agentStatus != "online" && agentStatus != "offline" {
		response.Error(c, ErrInvalid)
		return
	}
	if agentStatus == "online" || agentStatus == "offline" {
		h.listHostsByAgentStatus(c, page, groupID, environmentID, agentStatus)
		return
	}
	rows, count, err := h.service.ListHosts(c.Request.Context(), c.Query("search"), groupID, environmentID, page)
	if err != nil {
		respond(c, nil, err)
		return
	}
	for index := range rows {
		h.applyAgentPresence(&rows[index])
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}

func (h *Handler) listHostsByAgentStatus(c *gin.Context, page pagination.Page, groupID, environmentID int64, agentStatus string) {
	const batchSize = int32(pagination.MaxSize)
	filtered := make([]Host, 0)
	for batchNumber := int32(1); ; batchNumber++ {
		rows, count, err := h.service.ListHosts(c.Request.Context(), c.Query("search"), groupID, environmentID, pagination.New(batchNumber, batchSize))
		if err != nil {
			respond(c, nil, err)
			return
		}
		for index := range rows {
			h.applyAgentPresence(&rows[index])
			if (agentStatus == "online" && rows[index].AgentOnline) || (agentStatus == "offline" && !rows[index].AgentOnline) {
				filtered = append(filtered, rows[index])
			}
		}
		if int64(batchNumber)*int64(batchSize) >= count {
			break
		}
	}

	start := int(page.Offset)
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(page.Size)
	if end > len(filtered) {
		end = len(filtered)
	}
	response.Paginated(c, filtered[start:end], int64(len(filtered)), page.Number, page.Size)
}

func (h *Handler) GetHost(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	item, err := h.service.GetHost(c.Request.Context(), id)
	if err == nil {
		h.applyAgentPresence(&item)
		detail, detailErr := h.getHostDetail(c.Request.Context(), item)
		respond(c, detail, detailErr)
		return
	}
	respond(c, item, err)
}
func (h *Handler) CreateHost(c *gin.Context) {
	input, ok := bind[HostInput](c)
	if !ok {
		return
	}
	item, err := h.service.CreateHost(c.Request.Context(), input)
	respond(c, item, err)
}
func (h *Handler) UpdateHost(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	if c.Request.Method == http.MethodPatch {
		input, bound := bind[HostPatchInput](c)
		if !bound {
			return
		}
		item, err := h.service.PatchHost(c.Request.Context(), id, input)
		respond(c, item, err)
		return
	}
	input, ok := bind[HostInput](c)
	if !ok {
		return
	}
	item, err := h.service.UpdateHost(c.Request.Context(), id, input)
	respond(c, item, err)
}
func (h *Handler) DeleteHost(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	respond(c, nil, h.service.DeleteHost(c.Request.Context(), id))
}
