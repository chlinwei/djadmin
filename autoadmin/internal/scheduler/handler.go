package scheduler

import (
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func (h *Handler) List(c *gin.Context) {
	page, err := parsePage(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	filter := TaskFilter{Search: strings.TrimSpace(c.Query("search")), Enabled: optionalBool(c.Query("enabled")), IsRunning: optionalBool(c.Query("is_running"))}
	rows, count, err := h.service.ListTasks(c.Request.Context(), filter, page)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrTaskQueryInternal, err))
		return
	}
	response.Paginated(c, rows, count, page.Number, page.Size)
}
func (h *Handler) Get(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	task, err := h.service.GetTask(c.Request.Context(), id)
	taskResponse(c, task, err, ErrTaskQueryInternal)
}
func (h *Handler) Update(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	current, err := h.service.GetTask(c.Request.Context(), id)
	if err != nil {
		taskResponse(c, current, err, ErrTaskQueryInternal)
		return
	}
	var r struct {
		Name           *string `json:"name"`
		Code           *string `json:"code"`
		Description    *string `json:"description"`
		Enabled        *bool   `json:"enabled"`
		CronExpression *string `json:"cron_expression"`
	}
	if c.ShouldBindJSON(&r) != nil {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	input := TaskInput{Name: current.Name, Code: current.Code, Description: current.Description, Enabled: current.Enabled, CronExpression: current.EffectiveCronExpression}
	if r.Name != nil {
		input.Name = *r.Name
	}
	if r.Code != nil {
		input.Code = *r.Code
	}
	if r.Description != nil {
		input.Description = r.Description
	}
	if r.Enabled != nil {
		input.Enabled = *r.Enabled
	}
	if r.CronExpression != nil {
		input.CronExpression = *r.CronExpression
	}
	task, err := h.service.UpdateTask(c.Request.Context(), id, input)
	taskResponse(c, task, err, ErrTaskSaveInternal)
}
func (h *Handler) Enable(c *gin.Context)  { h.setEnabled(c, true) }
func (h *Handler) Disable(c *gin.Context) { h.setEnabled(c, false) }
func (h *Handler) Toggle(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	current, err := h.service.GetTask(c.Request.Context(), id)
	if err != nil {
		taskResponse(c, current, err, ErrTaskQueryInternal)
		return
	}
	task, err := h.service.SetEnabled(c.Request.Context(), id, !current.Enabled)
	taskResponse(c, task, err, ErrTaskSaveInternal)
}
func (h *Handler) setEnabled(c *gin.Context, enabled bool) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	task, err := h.service.SetEnabled(c.Request.Context(), id, enabled)
	taskResponse(c, task, err, ErrTaskSaveInternal)
}
func (h *Handler) Status(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	status, err := h.service.Status(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, ErrTaskNotFound)
		return
	}
	if err != nil {
		response.Error(c, apperror.WithCause(ErrTaskQueryInternal, err))
		return
	}
	response.Success(c, status)
}
func (h *Handler) RunNow(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	task, err := h.service.RunNow(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, ErrTaskNotFound)
		return
	}
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"task_id": id, "task_name": task.Name, "status": "submitted"})
}
func (h *Handler) StartScheduler(c *gin.Context) { h.setSchedulerEnabled(c, true) }
func (h *Handler) StopScheduler(c *gin.Context)  { h.setSchedulerEnabled(c, false) }
func (h *Handler) setSchedulerEnabled(c *gin.Context, enabled bool) {
	if err := h.service.SetSchedulerEnabled(c.Request.Context(), enabled); err != nil {
		response.Error(c, apperror.WithCause(ErrTaskSaveInternal, err))
		return
	}
	status := "Scheduler disabled"
	if enabled {
		status = "Scheduler enabled"
	}
	response.Success(c, gin.H{"status": status})
}
func (h *Handler) ListLogs(c *gin.Context) {
	page, err := parsePage(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	filter, err := logFilter(c)
	if err != nil {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	rows, count, err := h.service.ListLogs(c.Request.Context(), filter, page)
	if err != nil {
		response.Error(c, apperror.WithCause(ErrLogQueryInternal, err))
		return
	}
	response.Paginated(c, mapListLogs(rows), count, page.Number, page.Size)
}
func (h *Handler) GetLog(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	row, err := h.service.GetLog(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, ErrLogNotFound)
		return
	}
	if err != nil {
		response.Error(c, apperror.WithCause(ErrLogQueryInternal, err))
		return
	}
	response.Success(c, mapLog(row))
}
func parsePage(c *gin.Context) (pagination.Page, error) {
	number, err := positive(c.DefaultQuery("page", "1"))
	if err != nil {
		return pagination.Page{}, apperror.ErrPageInvalid
	}
	sizeRaw := c.Query("page_size")
	if sizeRaw == "" {
		sizeRaw = c.DefaultQuery("size", "10")
	}
	size, err := positive(sizeRaw)
	if err != nil {
		return pagination.Page{}, apperror.ErrPageSizeInvalid
	}
	return pagination.New(number, size), nil
}
func positive(raw string) (int32, error) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 1 {
		return 0, apperror.ErrInvalidRequest
	}
	return int32(value), nil
}
func optionalBool(raw string) *bool {
	switch strings.ToLower(raw) {
	case "true", "1":
		value := true
		return &value
	case "false", "0":
		value := false
		return &value
	}
	return nil
}
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		response.Error(c, apperror.ErrIDInvalid)
		return 0, false
	}
	return id, true
}
func taskResponse(c *gin.Context, data Task, err error, internal *apperror.Error) {
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, ErrTaskNotFound)
		return
	}
	if appErr, ok := apperror.As(err); ok {
		response.Error(c, appErr)
		return
	}
	if err != nil {
		response.Error(c, apperror.WithCause(internal, err))
		return
	}
	response.Success(c, data)
}
func logFilter(c *gin.Context) (LogFilter, error) {
	filter := LogFilter{Content: strings.TrimSpace(c.Query("content"))}
	if raw := c.Query("task_id"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return filter, err
		}
		filter.TaskID = &value
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if status == "success" || status == "succeeded" || status == "ok" {
		filter.Status = "成功"
	} else if status == "failed" || status == "failure" || status == "error" {
		filter.Status = "失败"
	} else if status != "" {
		filter.Status = c.Query("status")
	}
	if raw := c.Query("duration_min"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return filter, err
		}
		filter.DurationMin = &value
	}
	if raw := c.Query("duration_max"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return filter, err
		}
		filter.DurationMax = &value
	}
	return filter, nil
}
