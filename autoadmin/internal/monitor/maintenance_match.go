package monitor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

type maintenanceWindowHit struct {
	ID     int64     `json:"id"`
	Title  string    `json:"title"`
	EndsAt time.Time `json:"ends_at"`
}

type maintenanceMatcher struct {
	policyMatcher
	BusinessID int64 `json:"business_id,omitempty"`
	ScopeRow   int   `json:"scope_row,omitempty"` // 仅用于编辑回显，不参与匹配。
}

func maintenanceMatchers(raw json.RawMessage) ([]maintenanceMatcher, error) {
	var input []json.RawMessage
	if err := json.Unmarshal(raw, &input); err != nil || len(input) == 0 || len(input) > policyMaxMatchers {
		return nil, fmt.Errorf("matchers 必须包含 1 到 %d 个条件", policyMaxMatchers)
	}
	out := make([]maintenanceMatcher, 0, len(input))
	for _, item := range input {
		var m maintenanceMatcher
		d := json.NewDecoder(bytes.NewReader(item))
		d.DisallowUnknownFields()
		if err := d.Decode(&m); err != nil {
			return nil, fmt.Errorf("无效匹配条件: %w", err)
		}
		if m.ScopeRow < 0 || m.ScopeRow >= policyMaxMatchers || (m.Type == "label" && m.ScopeRow != 0) {
			return nil, errors.New("无效的范围行号")
		}
		if m.Type == "tree" && m.NodeType == "environment" {
			if m.BusinessID <= 0 {
				return nil, errors.New("环境节点必须指定所属业务 ID")
			}
		} else if m.BusinessID != 0 {
			return nil, errors.New("仅环境节点可指定所属业务 ID")
		}
		switch m.Type {
		case "host":
			if m.ID <= 0 || m.Label != "" || m.NodeType != "" || m.Operator != "" || m.Value != "" {
				return nil, errors.New("host 条件仅接受正整数 id")
			}
		case "tree":
			if !policyTreeTypes[m.NodeType] || m.ID <= 0 || m.Label != "" || m.Operator != "" || m.Value != "" {
				return nil, errors.New("服务树条件必须指定有效节点类型和正整数 ID")
			}
		case "label":
			m.Label, m.Value = strings.TrimSpace(m.Label), strings.TrimSpace(m.Value)
			if m.ID != 0 || m.NodeType != "" || m.Label == "" || m.Value == "" {
				return nil, errors.New("标签条件必须指定标签名称和值，不能包含资产字段")
			}
			if m.Operator != "=" && m.Operator != "contains" {
				return nil, errors.New("维护窗口标签条件仅支持等于或包含")
			}
		default:
			return nil, errors.New("维护窗口条件仅支持主机、服务树或标签")
		}
		out = append(out, m)
	}
	return out, nil
}

type maintenanceAlertFacts struct {
	labels       map[string]string
	hostID       int64
	hostName     string
	nodes        map[string]bool
	projectIDs   []int64
	assetsLoaded bool
}

func newMaintenanceFacts(t alertNotificationTarget) *maintenanceAlertFacts {
	return &maintenanceAlertFacts{labels: mergeAlertLabels(t.labels, t.alertname, t.severity, t.instance)}
}

// 严格解析主机 ID；不存在的资产不是当前资产，数据库失败则是错误，不能冒充“未命中”。
func (h *Handler) loadMaintenanceAssets(ctx context.Context, f *maintenanceAlertFacts) error {
	if f.assetsLoaded {
		return nil
	}
	raw := strings.TrimSpace(f.labels["host_id"])
	if raw == "" {
		f.assetsLoaded = true
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("无效的告警 host_id: %q", raw)
	}
	q := db.New(h.db)
	host, err := q.GetMaintenanceHost(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		f.assetsLoaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取告警主机: %w", err)
	}
	rows, err := q.ListHostAlertScopeNodes(ctx, id)
	if err != nil {
		return fmt.Errorf("读取告警主机服务树: %w", err)
	}
	f.hostID, f.hostName, f.nodes = id, host.Name, map[string]bool{}
	for _, r := range rows {
		f.nodes[fmt.Sprintf("service:%d", r.ServiceID)] = true
		f.nodes[fmt.Sprintf("business:%d", r.BusinessSystemID)] = true
		if r.EnvironmentID.Valid {
			f.nodes[fmt.Sprintf("environment:%d:%d", r.EnvironmentID.Int64, r.BusinessSystemID)] = true
		}
		if r.ProjectID.Valid {
			key := fmt.Sprintf("project:%d", r.ProjectID.Int64)
			if !f.nodes[key] {
				f.projectIDs = append(f.projectIDs, r.ProjectID.Int64)
			}
			f.nodes[key] = true
		}
	}
	f.assetsLoaded = true
	return nil
}

func (m maintenanceMatcher) matches(f *maintenanceAlertFacts) bool {
	if m.Type == "label" {
		actual, exists := f.labels[m.Label]
		return exists && ((m.Operator == "=" && actual == m.Value) || (m.Operator == "contains" && strings.Contains(actual, m.Value)))
	}
	if m.Type == "host" {
		return f.hostID == m.ID
	}
	if m.Type == "tree" && m.NodeType == "environment" {
		// 同一台主机可能跨业务部署；业务和环境必须来自同一条服务归属记录。
		return f.nodes[fmt.Sprintf("environment:%d:%d", m.ID, m.BusinessID)]
	}
	ok, _ := policyMatcherMatch(m.policyMatcher, f.labels, f.nodes)
	return ok
}

// 主机和树节点都是独立的范围入口；标签限制整个并集，不属于某一行。
func (h *Handler) maintenanceScopeMatch(ctx context.Context, ms []maintenanceMatcher, f *maintenanceAlertFacts) (bool, error) {
	var hasScope, scopeHit bool
	for _, m := range ms {
		if m.Type == "label" {
			if !m.matches(f) {
				return false, nil
			}
			continue
		}
		if err := h.loadMaintenanceAssets(ctx, f); err != nil {
			return false, err
		}
		hasScope = true
		scopeHit = scopeHit || m.matches(f)
	}
	return len(ms) > 0 && (!hasScope || scopeHit), nil
}

func (h *Handler) matchedMaintenanceWindows(ctx context.Context, t alertNotificationTarget, at time.Time) ([]maintenanceWindowHit, error) {
	rows, err := db.New(h.db).ListActiveMaintenanceWindows(ctx, db.ListActiveMaintenanceWindowsParams{StartBound: at.UTC(), EndBound: at.UTC()})
	if err != nil {
		return nil, err
	}
	hits := []maintenanceWindowHit{}
	f := newMaintenanceFacts(t)
	for _, r := range rows {
		ms, err := maintenanceMatchers(r.Matchers)
		if err != nil {
			return nil, fmt.Errorf("维护窗口 %d 条件损坏: %w", r.ID, err)
		}
		ok, err := h.maintenanceScopeMatch(ctx, ms, f)
		if err != nil {
			return nil, fmt.Errorf("维护窗口 %d: %w", r.ID, err)
		}
		if ok {
			hits = append(hits, maintenanceWindowHit{r.ID, r.Title, r.EndsAt})
		}
	}
	return hits, nil
}
