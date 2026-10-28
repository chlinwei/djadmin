package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	db "autoadmin/internal/platform/database/generated"
)

type scanEnvironment struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type scanScope struct {
	ProjectName  string                      `json:"project_name"`
	Environments []scanEnvironment           `json:"environments"`
	Hosts        map[int64][]scanEnvironment `json:"hosts"`
}

func loadScanScope(ctx context.Context, queries *db.Queries, projectID, environmentID sql.NullInt64, projectName, environmentName string, hostIDs []int64) (scanScope, error) {
	rows := []db.ListBaselineScopeEnvironmentsRow{}
	if projectID.Valid {
		var err error
		rows, err = queries.ListBaselineScopeEnvironments(ctx, db.ListBaselineScopeEnvironmentsParams{
			ProjectID: projectID, EnvironmentID: environmentID,
		})
		if err != nil {
			return scanScope{}, err
		}
	}
	scope := scanScope{ProjectName: projectName, Environments: []scanEnvironment{}, Hosts: make(map[int64][]scanEnvironment)}
	for _, id := range hostIDs {
		scope.Hosts[id] = []scanEnvironment{}
	}
	environments := make(map[int64]scanEnvironment)
	if environmentID.Valid {
		environments[environmentID.Int64] = scanEnvironment{ID: environmentID.Int64, Name: environmentName}
		// 选定环境已经由扫描记录确定，即使历史资产关联消失也不能丢失该范围。
		for _, id := range hostIDs {
			scope.Hosts[id] = []scanEnvironment{environments[environmentID.Int64]}
		}
	} else {
		for _, row := range rows {
			if _, scanned := scope.Hosts[row.HostID]; !scanned || !row.EnvironmentID.Valid {
				continue
			}
			environment := scanEnvironment{ID: row.EnvironmentID.Int64, Name: row.EnvironmentName}
			environments[environment.ID] = environment
			scope.Hosts[row.HostID] = append(scope.Hosts[row.HostID], environment)
		}
	}
	for _, environment := range environments {
		scope.Environments = append(scope.Environments, environment)
	}
	sort.Slice(scope.Environments, func(i, j int) bool { return scope.Environments[i].ID < scope.Environments[j].ID })
	return scope, nil
}

func decodeScanScope(raw json.RawMessage) (scanScope, error) {
	var scope scanScope
	if err := json.Unmarshal(raw, &scope); err != nil {
		return scope, fmt.Errorf("扫描范围快照无法解析: %w", err)
	}
	if scope.Hosts == nil || scope.Environments == nil {
		return scope, fmt.Errorf("扫描范围快照缺少主机或环境信息")
	}
	return scope, nil
}
