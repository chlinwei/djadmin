package assets

import (
	"context"
	"database/sql"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
)

type DependencyTopologyScope struct {
	ProjectID     int64 `json:"project_id"`
	EnvironmentID int64 `json:"environment_id"`
	BusinessID    int64 `json:"business_id,omitempty"`
	ServiceID     int64 `json:"service_id,omitempty"`
}

type DependencyRuntimeSummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Error   int `json:"error"`
	Unknown int `json:"unknown"`
}

type DependencyTopologyInstance struct {
	ID                  int64   `json:"id"`
	InstanceName        string  `json:"instance_name"`
	HostID              int64   `json:"host_id"`
	HostIP              *string `json:"host_ip"`
	RuntimeStatus       string  `json:"runtime_status"`
	RuntimeStatusOutput string  `json:"runtime_status_output"`
	LastStatusCheckTime *string `json:"last_status_check_time"`
	HaRole              string  `json:"ha_role"`
}

type DependencyTopologyNode struct {
	db.ListDependencyTopologyNodesRow
	ExternalToSelection bool                         `json:"external_to_selection"`
	RuntimeSummary      DependencyRuntimeSummary     `json:"runtime_summary"`
	Instances           []DependencyTopologyInstance `json:"instances"`
}

type DependencyTopology struct {
	Scope        DependencyTopologyScope         `json:"scope"`
	SnapshotTime time.Time                       `json:"snapshot_time"`
	StateSource  string                          `json:"state_source"`
	Nodes        []DependencyTopologyNode        `json:"nodes"`
	Edges        []db.ListServiceDependenciesRow `json:"edges"`
}

func (s *Service) GetServiceDependencyTopology(ctx context.Context, scope DependencyTopologyScope) (DependencyTopology, error) {
	result := DependencyTopology{Scope: scope, SnapshotTime: time.Now().UTC(), StateSource: "stored_runtime_snapshot", Nodes: []DependencyTopologyNode{}, Edges: []db.ListServiceDependenciesRow{}}
	if scope.ProjectID < 1 || scope.EnvironmentID < 1 || scope.BusinessID < 0 || scope.ServiceID < 0 {
		return result, ErrInvalid
	}
	// Nodes, edges and memberships share one read snapshot; GET never executes status scripts.
	tx, err := s.repository.pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, translate(err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.GetProject(ctx, scope.ProjectID); err != nil {
		return result, translate(err)
	}
	if _, err = q.GetBusinessEnvironment(ctx, scope.EnvironmentID); err != nil {
		return result, translate(err)
	}
	if scope.BusinessID > 0 {
		business, err := q.GetBusinessSystem(ctx, scope.BusinessID)
		if err != nil {
			return result, translate(err)
		}
		if !business.ProjectID.Valid || business.ProjectID.Int64 != scope.ProjectID {
			return result, ErrDependencyScope
		}
	}
	nodes, err := q.ListDependencyTopologyNodes(ctx, db.ListDependencyTopologyNodesParams{ProjectID: dependencyID(scope.ProjectID), EnvironmentID: dependencyID(scope.EnvironmentID)})
	if err != nil {
		return result, translate(err)
	}
	if len(nodes) > 1000 {
		return result, topologyLimitError()
	}
	focusFound := scope.ServiceID == 0
	for _, node := range nodes {
		if node.ID == scope.ServiceID {
			focusFound = true
			if scope.BusinessID > 0 && node.BusinessSystemID != scope.BusinessID {
				return result, ErrDependencyScope
			}
		}
	}
	if !focusFound {
		return result, ErrNotFound
	}
	edges, err := q.ListServiceDependencies(ctx, db.ListServiceDependenciesParams{
		ProjectID: dependencyID(scope.ProjectID), EnvironmentID: dependencyID(scope.EnvironmentID),
		BusinessID: dependencyID(scope.BusinessID), ServiceID: dependencyID(scope.ServiceID), Limit: 5001,
	})
	if err != nil {
		return result, translate(err)
	}
	if len(edges) > 5000 {
		return result, topologyLimitError()
	}
	instances, err := q.ListDependencyTopologyInstances(ctx, db.ListDependencyTopologyInstancesParams{ProjectID: dependencyID(scope.ProjectID), EnvironmentID: dependencyID(scope.EnvironmentID)})
	if err != nil {
		return result, translate(err)
	}
	if len(instances) > 10000 {
		return result, topologyLimitError()
	}
	if err = tx.Commit(); err != nil {
		return result, translate(err)
	}
	result.Nodes = assembleDependencyTopology(nodes, edges, instances, scope)
	result.Edges = edges
	return result, nil
}

func topologyLimitError() error {
	return apperror.New(apperror.CodeInvalidArgument, "项目环境拓扑超过安全上限（1000 服务、5000 关系或 10000 条实例关联），请拆分项目或环境后查询")
}

func assembleDependencyTopology(nodes []db.ListDependencyTopologyNodesRow, edges []db.ListServiceDependenciesRow, instances []db.ListDependencyTopologyInstancesRow, scope DependencyTopologyScope) []DependencyTopologyNode {
	roots := map[int64]bool{}
	included := map[int64]bool{}
	for _, node := range nodes {
		if (scope.BusinessID == 0 || node.BusinessSystemID == scope.BusinessID) && (scope.ServiceID == 0 || node.ID == scope.ServiceID) {
			roots[node.ID] = true
			included[node.ID] = true
		}
	}
	for _, edge := range edges {
		included[edge.SourceServiceID] = true
		included[edge.TargetServiceID] = true
	}
	grouped := map[int64][]DependencyTopologyInstance{}
	for _, instance := range instances {
		if !included[instance.ServiceID] {
			continue
		}
		status := instance.RuntimeStatus
		// A persisted status without a check timestamp is not evidence that an instance is running.
		if !instance.LastStatusCheckTime.Valid || (status != "running" && status != "stopped" && status != "error") {
			status = "unknown"
		}
		grouped[instance.ServiceID] = append(grouped[instance.ServiceID], DependencyTopologyInstance{
			ID: instance.ID, InstanceName: instance.InstanceName, HostID: instance.HostID, HostIP: nullStringPtr(instance.HostIp),
			RuntimeStatus: status, RuntimeStatusOutput: instance.RuntimeStatusOutput,
			LastStatusCheckTime: nullableTime(instance.LastStatusCheckTime), HaRole: instance.HaRole,
		})
	}
	result := make([]DependencyTopologyNode, 0, len(included))
	for _, node := range nodes {
		if !included[node.ID] {
			continue
		}
		items := grouped[node.ID]
		if items == nil {
			items = []DependencyTopologyInstance{}
		}
		summary := DependencyRuntimeSummary{Total: len(items)}
		for _, instance := range items {
			switch instance.RuntimeStatus {
			case "running":
				summary.Running++
			case "stopped":
				summary.Stopped++
			case "error":
				summary.Error++
			default:
				summary.Unknown++
			}
		}
		result = append(result, DependencyTopologyNode{ListDependencyTopologyNodesRow: node, ExternalToSelection: !roots[node.ID], RuntimeSummary: summary, Instances: items})
	}
	return result
}
