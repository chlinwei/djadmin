package logcollect

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// 通用配置资源（保留档位 / 解析规则 / 采集过滤规则）的写路径实现。
//
// 为什么长这样：config_resources.go 原来是"运行时拼表名 + 列名"的 CRUD，而 sqlc 的语句是
// 编译期固定的，所以按表分派到显式语句；"只写提交了的列"这一 PATCH 语义改由
// 「更新前读回整行 → 合并提交的字段 → 整行写」承担（与 inspection 组 PATCH、
// monitor_target 的 PATCH 同一手法），而不是 `COALESCE(narg,col)` ——
// 后者表达不了"显式写入空值"。
//
// 新建走整行插入：请求体缺了"NOT NULL 且无库级默认值"的列时由 saveResource 的
// required 检查先拦下（原实现是把语句交给 MySQL 严格模式报错）。

// ---- monitor_log_retention_tier ----

func createLogRetentionTier(context context.Context, pool db.DBTX, input map[string]any) (int64, error) {
	now := time.Now().UTC()
	return db.New(pool).CreateLogRetentionTier(context, db.CreateLogRetentionTierParams{
		CreateTime: now, UpdateTime: now,
		Code: stringValue(input["code"]), Name: stringValue(input["name"]),
		DailySizeGb:         floatValue(input["daily_size_gb"]),
		RetentionValue:      uint32(intValue(input["retention_value"])),
		RetentionUnit:       retentionUnitOrDefault(input["retention_unit"]),
		RolloverMinIndexAge: stringValue(input["rollover_min_index_age"]),
		Enabled:             boolValue(input["enabled"]), IsDefault: boolValue(input["is_default"]),
		Remark: stringValue(input["remark"]),
	})
}

func updateLogRetentionTier(context context.Context, pool db.DBTX, id int64, input map[string]any) error {
	queries := db.New(pool)
	current, err := queries.GetLogRetentionTier(context, id)
	if err != nil {
		return err
	}
	if value, ok := input["code"]; ok {
		current.Code = stringValue(value)
	}
	if value, ok := input["name"]; ok {
		current.Name = stringValue(value)
	}
	if value, ok := input["daily_size_gb"]; ok {
		current.DailySizeGb = floatValue(value)
	}
	if value, ok := input["retention_value"]; ok {
		current.RetentionValue = uint32(intValue(value))
	}
	if value, ok := input["retention_unit"]; ok {
		current.RetentionUnit = retentionUnitOrDefault(value)
	}
	if value, ok := input["rollover_min_index_age"]; ok {
		current.RolloverMinIndexAge = stringValue(value)
	}
	if value, ok := input["enabled"]; ok {
		current.Enabled = boolValue(value)
	}
	if value, ok := input["is_default"]; ok {
		current.IsDefault = boolValue(value)
	}
	if value, ok := input["remark"]; ok {
		current.Remark = stringValue(value)
	}
	affected, err := queries.UpdateLogRetentionTier(context, db.UpdateLogRetentionTierParams{
		UpdateTime: time.Now().UTC(), Code: current.Code, Name: current.Name,
		DailySizeGb: current.DailySizeGb, RetentionValue: current.RetentionValue,
		RetentionUnit: current.RetentionUnit,
		RolloverMinIndexAge: current.RolloverMinIndexAge, Enabled: current.Enabled,
		IsDefault: current.IsDefault, Remark: current.Remark, ID: id,
	})
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func deleteLogRetentionTier(context context.Context, pool db.DBTX, id int64) error {
	return deleteRowsAffected(db.New(pool).DeleteLogRetentionTier(context, id))
}

// clearDefaultLogRetentionTier 保留档位的"唯一默认"约束：把其它行的 is_default 清掉。
func clearDefaultLogRetentionTier(context context.Context, pool db.DBTX, id int64) error {
	return db.New(pool).ClearDefaultLogRetentionTier(context, id)
}

// ---- monitor_log_processing_rule ----

func createLogProcessingRule(context context.Context, pool db.DBTX, input map[string]any) (int64, error) {
	now := time.Now().UTC()
	body, err := jsonFieldValue(input, "pipeline_body")
	if err != nil {
		return 0, err
	}
	return db.New(pool).CreateLogProcessingRule(context, db.CreateLogProcessingRuleParams{
		CreateTime: now, UpdateTime: now,
		Remark: nullableStringValue(input, "remark"),
		Name:   stringValue(input["name"]), Description: stringValue(input["description"]),
		InputFormat:         stringValue(input["input_format"]),
		MultilineEnabled:    boolValue(input["multiline_enabled"]),
		StartPattern:        stringValue(input["start_pattern"]),
		ContinuationPattern: stringValue(input["continuation_pattern"]),
		SampleLog:           stringValue(input["sample_log"]),
		FlushTimeout:        uint32(intValue(input["flush_timeout"])),
		PipelineBody:        body,
		ClusterID:           intValue(input["cluster"]),
		ApplicationID:       nullableInt64Value(input, "application"),
	})
}

func updateLogProcessingRule(context context.Context, pool db.DBTX, id int64, input map[string]any) error {
	queries := db.New(pool)
	current, err := queries.GetLogProcessingRule(context, id)
	if err != nil {
		return err
	}
	if _, ok := input["remark"]; ok {
		current.Remark = nullableStringValue(input, "remark")
	}
	if value, ok := input["name"]; ok {
		current.Name = stringValue(value)
	}
	if value, ok := input["description"]; ok {
		current.Description = stringValue(value)
	}
	if value, ok := input["input_format"]; ok {
		current.InputFormat = stringValue(value)
	}
	if value, ok := input["multiline_enabled"]; ok {
		current.MultilineEnabled = boolValue(value)
	}
	if value, ok := input["start_pattern"]; ok {
		current.StartPattern = stringValue(value)
	}
	if value, ok := input["continuation_pattern"]; ok {
		current.ContinuationPattern = stringValue(value)
	}
	if value, ok := input["sample_log"]; ok {
		current.SampleLog = stringValue(value)
	}
	if value, ok := input["flush_timeout"]; ok {
		current.FlushTimeout = uint32(intValue(value))
	}
	if _, ok := input["pipeline_body"]; ok {
		body, err := jsonFieldValue(input, "pipeline_body")
		if err != nil {
			return err
		}
		current.PipelineBody = body
	}
	if value, ok := input["cluster"]; ok {
		current.ClusterID = intValue(value)
	}
	if _, ok := input["application"]; ok {
		current.ApplicationID = nullableInt64Value(input, "application")
	}
	affected, err := queries.UpdateLogProcessingRule(context, db.UpdateLogProcessingRuleParams{
		UpdateTime: time.Now().UTC(), Remark: current.Remark, Name: current.Name,
		Description: current.Description, InputFormat: current.InputFormat,
		MultilineEnabled: current.MultilineEnabled, StartPattern: current.StartPattern,
		ContinuationPattern: current.ContinuationPattern, SampleLog: current.SampleLog,
		FlushTimeout: current.FlushTimeout,
		PipelineBody: current.PipelineBody, ClusterID: current.ClusterID,
		ApplicationID: current.ApplicationID, ID: id,
	})
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func deleteLogProcessingRule(context context.Context, pool db.DBTX, id int64) error {
	return deleteRowsAffected(db.New(pool).DeleteLogProcessingRule(context, id))
}

// ---- monitor_log_collection_filter_rule ----

func createLogCollectionFilterRule(context context.Context, pool db.DBTX, input map[string]any) (int64, error) {
	now := time.Now().UTC()
	return db.New(pool).CreateLogCollectionFilterRule(context, db.CreateLogCollectionFilterRuleParams{
		CreateTime: now, UpdateTime: now,
		Remark: nullableStringValue(input, "remark"),
		Name:   stringValue(input["name"]), Description: stringValue(input["description"]),
		Pattern: stringValue(input["pattern"]), RuleType: stringValue(input["rule_type"]),
		Enabled:       boolValue(input["enabled"]),
		ApplicationID: nullableInt64Value(input, "application"),
	})
}

func updateLogCollectionFilterRule(context context.Context, pool db.DBTX, id int64, input map[string]any) error {
	queries := db.New(pool)
	current, err := queries.GetLogCollectionFilterRule(context, id)
	if err != nil {
		return err
	}
	if _, ok := input["remark"]; ok {
		current.Remark = nullableStringValue(input, "remark")
	}
	if value, ok := input["name"]; ok {
		current.Name = stringValue(value)
	}
	if value, ok := input["description"]; ok {
		current.Description = stringValue(value)
	}
	if value, ok := input["pattern"]; ok {
		current.Pattern = stringValue(value)
	}
	if value, ok := input["rule_type"]; ok {
		current.RuleType = stringValue(value)
	}
	if value, ok := input["enabled"]; ok {
		current.Enabled = boolValue(value)
	}
	if _, ok := input["application"]; ok {
		current.ApplicationID = nullableInt64Value(input, "application")
	}
	affected, err := queries.UpdateLogCollectionFilterRule(context, db.UpdateLogCollectionFilterRuleParams{
		UpdateTime: time.Now().UTC(), Remark: current.Remark, Name: current.Name,
		Description: current.Description, Pattern: current.Pattern, RuleType: current.RuleType,
		Enabled:       current.Enabled,
		ApplicationID: current.ApplicationID, ID: id,
	})
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func deleteLogCollectionFilterRule(context context.Context, pool db.DBTX, id int64) error {
	return deleteRowsAffected(db.New(pool).DeleteLogCollectionFilterRule(context, id))
}

// ---- 请求体取值辅助 ----

// jsonFieldValue 把请求体里的 json 字段序列化成列值；键不存在时返回空对象。
func jsonFieldValue(input map[string]any, key string) (json.RawMessage, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return json.RawMessage(`{}`), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// nullableStringValue 键不存在时返回 NULL（remark 这类可空列）。
func nullableStringValue(input map[string]any, key string) sql.NullString {
	value, ok := input[key]
	if !ok || value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: stringValue(value), Valid: true}
}

// nullableInt64Value 键不存在或显式传 null 时返回 NULL（application_id 可空 = 不限应用）。
func nullableInt64Value(input map[string]any, key string) sql.NullInt64 {
	value, ok := input[key]
	if !ok || value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: intValue(value), Valid: true}
}
