package assets

import (
	"context"
	"encoding/json"
	"fmt"
)

// 服务级后处理器（2026-09-28）：服务 × 日志定义粒度、"解析规则 pipeline 之后"再执行的
// ES ingest 处理器数组。存储在 assets_application_service_log_setting.extra_processors
// （迁移 000050，`[]` = 不启用）。
//
// 为什么要按服务拆：解析规则的 pipeline 是按规则共享的，在规则里写 drop_fields 会影响
// 所有引用该规则的服务。平台按本列的配置为服务生成**包装 pipeline**（先调基础 pipeline、
// 再执行这里的处理器），只有该服务的 Filebeat 片段指向包装——机制见
// internal/logcollect/service_pipeline.go 与 LOG_COLLECTION_ARCHITECTURE.md「服务级后处理」。
//
// 本文件只管三件事：校验（保存路径用）、数据结构字段（响应/入参）、对账钩子（保存后触发
// logcollect 侧的包装同步）。包装的命名、body 构建与 ES 交互都在 logcollect。

const (
	// MaxServiceExtraProcessors 单个服务的后处理器数量上限。drop/rename 这类后处理
	// 正常用量是个位数；上限只是防线（与 MaxBatchLogOverrides 同一取舍）。
	MaxServiceExtraProcessors = 20
	// MaxServiceExtraProcessorsBytes 原始 JSON 体积上限（16KB），防呆不防真需求。
	MaxServiceExtraProcessorsBytes = 16 * 1024
)

// ValidateServiceExtraProcessors 校验服务级后处理器数组（保存路径的入口校验）。
//
// 规则：
//   - 必须是 JSON 数组（或空串/NULL，统一视为不启用 → 归一成 `[]`）；
//   - 每个元素必须是对象，且**只能有一个处理器键**（ES 的处理器形态）；
//   - 禁止 `pipeline` 处理器：包装 pipeline 本身就是靠一个 pipeline 处理器调基础的，
//     用户再嵌一层轻则绕过平台的级联清理（改名/换集群时漏删），重则自引用成环；
//   - 禁止 `script` 处理器对必备字段（log_level/log_message/error_fingerprint）清空/删除
//     做不了静态判定，不拦——写入侧仍有 `<prefix>-mapping-guard` 兜底（tag 模式打标、
//     drop 模式丢弃），与规则自己的 drop 处理器同一兜底。
//
// 返回归一化后的 JSON（数组，紧凑序列化）；raw 为空时返回 `[]`。
func ValidateServiceExtraProcessors(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := trimJSONSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage("[]"), nil
	}
	if len(trimmed) > MaxServiceExtraProcessorsBytes {
		return nil, fmt.Errorf("服务级后处理器过大（上限 %d 字节）", MaxServiceExtraProcessorsBytes)
	}
	var items []map[string]any
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, fmt.Errorf("服务级后处理器必须是 JSON 数组（元素为处理器对象）: %w", err)
	}
	if len(items) > MaxServiceExtraProcessors {
		return nil, fmt.Errorf("服务级后处理器最多 %d 个", MaxServiceExtraProcessors)
	}
	for index, item := range items {
		if len(item) != 1 {
			return nil, fmt.Errorf("第 %d 个处理器必须是恰好一个处理器键的对象（如 {\"drop_fields\":{…}}）", index+1)
		}
		for name := range item {
			if name == "pipeline" {
				return nil, fmt.Errorf("后处理器不允许嵌套 pipeline 处理器：平台生成的包装已自动先执行解析规则，再嵌套会绕过改名/换集群时的级联清理")
			}
		}
	}
	normalized, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// ExtraProcessorsEnabled 判断归一化后的数组是否"配置了后处理"（非空数组）。
// 渲染与对账都认它，不要各自手写 len 判断。
func ExtraProcessorsEnabled(raw json.RawMessage) bool {
	var items []any
	if err := json.Unmarshal(trimJSONSpace(raw), &items); err != nil {
		return false
	}
	return len(items) > 0
}

func trimJSONSpace(raw json.RawMessage) json.RawMessage {
	start, end := 0, len(raw)
	for start < end && (raw[start] == ' ' || raw[start] == '\t' || raw[start] == '\r' || raw[start] == '\n') {
		start++
	}
	for end > start && (raw[end-1] == ' ' || raw[end-1] == '\t' || raw[end-1] == '\r' || raw[end-1] == '\n') {
		end--
	}
	return raw[start:end]
}

// ServiceLogPipelineSyncer 由 logcollect 实现（logcollect 才持有 ES 客户端与集群配置），
// 在 router 注入（与 LogFormatVerifier 同一套反向注入先例，避免 assets 反向依赖 logcollect）。
//
// 语义是**按服务对账**而不是"只增"：读出该服务的全部覆盖行，算出期望的包装集合，
// PUT 缺的/内容变的，DELETE 不再期望的（含通配列举出的历史残留）。幂等，可重复调用。
type ServiceLogPipelineSyncer interface {
	SyncServiceLogPipelines(ctx context.Context, serviceID int64) error
}

// SetServiceLogPipelineSyncer 注入对账执行器（router.NewWithGateway）。未注入时保存仍可用，
// 只是包装不会被同步——渲染会照常指向包装（配置了后处理时），所以未注入属部署错误，
// 不在代码里静默兜底。
func (s *Service) SetServiceLogPipelineSyncer(syncer ServiceLogPipelineSyncer) {
	s.serviceLogPipelineSyncer = syncer
}

// syncServiceLogPipelines 统一的保存后触发点：失败返回带上下文的错误，由调用方决定
// 让整次保存以失败呈现（单条路径）或记进逐条结果（批量路径）。sync 是幂等的，重试保存
// 或重新保存一次即可补齐。
func (s *Service) syncServiceLogPipelines(ctx context.Context, serviceID int64) error {
	if s.serviceLogPipelineSyncer == nil {
		return nil
	}
	if err := s.serviceLogPipelineSyncer.SyncServiceLogPipelines(ctx, serviceID); err != nil {
		return fmt.Errorf("服务级后处理 pipeline 同步失败（配置已保存，重试保存可补齐）: %w", err)
	}
	return nil
}
