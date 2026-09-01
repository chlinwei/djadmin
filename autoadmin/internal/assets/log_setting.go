package assets

import (
	"context"
	"database/sql"
)

func (r *Repository) ListServiceLogSettings(ctx context.Context, serviceID int64) ([]ServiceLogSettingInput, error) {
	rows, err := r.pool.QueryContext(ctx, `SELECT log_definition_id,retention_tier_id,collection_enabled,collection_filter_rule_id,processing_rule_id FROM assets_application_service_log_setting WHERE service_id=? ORDER BY log_definition_id`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ServiceLogSettingInput, 0)
	for rows.Next() {
		var item ServiceLogSettingInput
		var retention, filterRule, processing sql.NullInt64
		var collection sql.NullBool
		if err = rows.Scan(&item.LogDefinition, &retention, &collection, &filterRule, &processing); err != nil {
			return nil, err
		}
		item.RetentionTier = intPtr(retention)
		item.CollectionFilterRule = intPtr(filterRule)
		item.ProcessingRule = intPtr(processing)
		if collection.Valid {
			item.CollectionEnabled = &collection.Bool
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
