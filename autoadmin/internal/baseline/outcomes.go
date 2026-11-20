package baseline

import (
	"context"

	db "autoadmin/internal/platform/database/generated"
)

func baselineTargetOutcome(status string, unverified bool) string {
	switch status {
	case "success":
		return "compliant"
	case "failed":
		if !unverified {
			return "noncompliant"
		}
		return "execution_error"
	default:
		return status
	}
}

type baselineOutcomeSummary map[string]int64

func (summary baselineOutcomeSummary) add(outcome string, count int64) {
	summary["total"] += count
	if outcome == "interrupted" {
		outcome = "execution_error"
	}
	summary[outcome] += count
}

func (summary baselineOutcomeSummary) status(status string) string {
	if status != "failed" {
		return status
	}
	if summary["execution_error"] > 0 {
		if summary["noncompliant"] > 0 {
			return "mixed"
		}
		return "execution_error"
	}
	if summary["noncompliant"] > 0 {
		return "noncompliant"
	}
	return "execution_error"
}

func listBaselineOutcomeSummaries(ctx context.Context, queries *db.Queries, scans []db.ListScansRow) (map[int64]baselineOutcomeSummary, error) {
	ids := make([]int64, 0, len(scans))
	summaries := make(map[int64]baselineOutcomeSummary, len(scans))
	for _, scan := range scans {
		if scan.ScanType == "baseline" {
			ids = append(ids, scan.ID)
			summaries[scan.ID] = make(baselineOutcomeSummary)
		}
	}
	if len(ids) == 0 {
		return summaries, nil
	}
	rows, err := queries.ListBaselineOutcomeCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		summaries[row.ScanID].add(baselineTargetOutcome(row.Status, row.Unverified != 0), row.TargetCount)
	}
	return summaries, nil
}
