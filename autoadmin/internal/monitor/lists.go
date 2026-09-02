package monitor

import (
	"encoding/json"
	"strconv"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListTargets(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	count, err := queries.CountMonitorTargets(context, monitorTargetFilter(context))
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListMonitorTargets(context, db.ListMonitorTargetsParams{
		ExporterType:     optionalStringParam(context, "exporter_type"),
		ManagedEnabled:   optionalBoolParam(context, "managed_enabled"),
		InstallStatus:    optionalStringParam(context, "install_status"),
		LastScrapeStatus: optionalStringParam(context, "last_scrape_status"),
		SearchPattern:    searchPatternParam(context),
		Limit:            int32(size),
		Offset:           int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]monitorTargetResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, monitorTargetResponseFrom(db.ListMonitorTargetsRow(row)))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func monitorTargetFilter(context *gin.Context) db.CountMonitorTargetsParams {
	return db.CountMonitorTargetsParams{
		ExporterType:     optionalStringParam(context, "exporter_type"),
		ManagedEnabled:   optionalBoolParam(context, "managed_enabled"),
		InstallStatus:    optionalStringParam(context, "install_status"),
		LastScrapeStatus: optionalStringParam(context, "last_scrape_status"),
		SearchPattern:    searchPatternParam(context),
	}
}

type monitorTargetResponse struct {
	ID                 int64           `json:"id"`
	CreateTime         time.Time       `json:"create_time"`
	UpdateTime         time.Time       `json:"update_time"`
	Remark             *string         `json:"remark"`
	ExporterType       string          `json:"exporter_type"`
	ManagedEnabled     bool            `json:"managed_enabled"`
	InstallStatus      string          `json:"install_status"`
	InstallMessage     string          `json:"install_message"`
	LastScrapeStatus   string          `json:"last_scrape_status"`
	LastScrapeAt       *time.Time      `json:"last_scrape_at"`
	Labels             json.RawMessage `json:"labels"`
	HostID             int64           `json:"host_id"`
	RetryCount         uint32          `json:"retry_count"`
	LastDispatchManual bool            `json:"last_dispatch_manual"`
	ScrapePort         uint32          `json:"scrape_port"`
	TargetType         string          `json:"target_type"`
	HostName           *string         `json:"host_name"`
	HostIP             *string         `json:"host_ip"`
	HostAgentOnline    bool            `json:"host_agent_online"`
}

func monitorTargetResponseFrom(row db.ListMonitorTargetsRow) monitorTargetResponse {
	var remark *string
	if row.Remark.Valid {
		remark = &row.Remark.String
	}
	var lastScrapeAt *time.Time
	if row.LastScrapeAt.Valid {
		lastScrapeAt = &row.LastScrapeAt.Time
	}
	var hostName, hostIP *string
	if row.HostName.Valid {
		hostName = &row.HostName.String
	}
	if row.HostIp.Valid {
		hostIP = &row.HostIp.String
	}
	return monitorTargetResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: remark,
		ExporterType: row.ExporterType, ManagedEnabled: row.ManagedEnabled,
		InstallStatus: row.InstallStatus, InstallMessage: row.InstallMessage,
		LastScrapeStatus: row.LastScrapeStatus, LastScrapeAt: lastScrapeAt,
		Labels: row.Labels, HostID: row.HostID, RetryCount: row.RetryCount,
		LastDispatchManual: row.LastDispatchManual, ScrapePort: row.ScrapePort,
		TargetType: row.TargetType, HostName: hostName, HostIP: hostIP,
		HostAgentOnline: row.HostAgentOnline,
	}
}

type exporterTargetResponse struct {
	ID               int64  `json:"id"`
	ExporterType     string `json:"exporter_type"`
	ScrapePort       uint32 `json:"scrape_port"`
	ManagedEnabled   bool   `json:"managed_enabled"`
	InstallStatus    string `json:"install_status"`
	InstallMessage   string `json:"install_message"`
	LastScrapeStatus string `json:"last_scrape_status"`
}

func exporterTargetResponseFrom(row db.ListMonitorTargetsByHostRow) exporterTargetResponse {
	return exporterTargetResponse{
		ID: row.ID, ExporterType: row.ExporterType, ScrapePort: row.ScrapePort,
		ManagedEnabled: row.ManagedEnabled, InstallStatus: row.InstallStatus,
		InstallMessage: row.InstallMessage, LastScrapeStatus: row.LastScrapeStatus,
	}
}

func pagination(context *gin.Context) (int, int) {
	page, _ := strconv.Atoi(context.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(context.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 30 {
		size = 30
	}
	return page, size
}
