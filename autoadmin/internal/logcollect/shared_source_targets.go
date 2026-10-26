package logcollect

import (
	"context"
	"sort"

	db "autoadmin/internal/platform/database/generated"
)

func listServiceLogApplyTargets(ctx context.Context, pool db.DBTX, serviceID int64) ([]db.ListServiceLogApplyTargetsRow, error) {
	q := db.New(pool)
	rows, err := q.ListServiceLogApplyTargets(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	shared, err := q.ListServiceSharedLogApplyTargets(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, row := range rows {
		seen[row.HostID] = true
	}
	for _, row := range shared {
		if seen[row.HostID] {
			continue
		}
		seen[row.HostID] = true
		rows = append(rows, db.ListServiceLogApplyTargetsRow{
			HostID: row.HostID, HostIp: row.HostIp, HostInstanceName: row.HostInstanceName,
			TargetID: row.TargetID, ConfigFingerprint: row.ConfigFingerprint, RuntimeStatus: row.RuntimeStatus,
			ServiceFingerprints: row.ServiceFingerprints, ServiceID: row.ServiceID, ServiceCode: row.ServiceCode,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].HostID < rows[j].HostID })
	return rows, nil
}
