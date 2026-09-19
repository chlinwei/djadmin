package logcollect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	db "autoadmin/internal/platform/database/generated"
)

// 采集过滤（include/exclude）的解析与渲染（2026-09-19）。
//
// 历史：Fluent Bit 时代这是真生效的（`[FILTER] grep`，见 LOG_COLLECTION_ARCHITECTURE §8.4 的迁移说明），
// 换 Filebeat 时只搬了配置、没搬渲染，于是"能建规则、能选，却不生效"了三年。这里把它接回来。
//
// **两层 + 三态**：
//
//	模板日志定义（`filter_include_rule_id` / `filter_exclude_rule_id`）= 默认值，NULL 表示该方向不过滤；
//	服务级覆盖（`collection_filter_rule_id` / `collection_exclude_filter_rule_id`）= NULL 继承模板、
//	0 显式关闭该方向（模板配了也不过滤）、>0 指定规则。
//
// 两个方向各自独立，因为 Filebeat 上它们本来就是同一 input 的两个参数（`include_lines` 先跑、
// `exclude_lines` 后跑，同时命中 → 丢弃），所以"先框白名单、再排噪声"是常规用法。
//
// **宁可多采不可不采**：任何解析不出来的情况（规则被删、规则停用、方向不符、正则编译不过）
// 都按"该方向不过滤"处理并告警，而不是让这条日志不采或让 Filebeat 起不来——过滤是可选优化，
// 忽略它的后果只是采多了；这与平台"不漏采优先"的既定取舍一致（§6）。

// logFilterRuleRef 一条过滤规则在渲染时需要的最小信息。
type logFilterRuleRef struct {
	ID      int64
	Name    string
	Pattern string
	// RuleType include / exclude：方向由规则自己声明，落槽时校验一致，避免反转语义。
	RuleType string
	Enabled  bool
}

// resolvedLogFilter 一条 (服务 × 日志定义) 最终生效的过滤（两个方向各自可空）。
type resolvedLogFilter struct {
	Include *logFilterRuleRef
	Exclude *logFilterRuleRef
	// Warnings 解析过程中的问题（规则不存在/停用/方向不符），随下发告警带给用户。
	Warnings []string
}

// resolveLogFilter 解析两个方向最终生效的规则。
//
// 参数是四个 id（nil = 该层的该方向没配；0 = 服务级显式关闭；>0 = 规则 id），
// 以及按 id 索引的规则表快照。纯函数，便于把三态与各种降级情形钉在单测里。
func resolveLogFilter(templateInclude, templateExclude, serviceInclude, serviceExclude *int64, rules map[int64]logFilterRuleRef) resolvedLogFilter {
	resolved := resolvedLogFilter{}
	resolve := func(direction string, templateID, serviceID *int64) *logFilterRuleRef {
		pick := func(ruleID int64, source string) *logFilterRuleRef {
			rule, exists := rules[ruleID]
			if !exists {
				resolved.Warnings = append(resolved.Warnings, fmt.Sprintf(
					"%s过滤规则 #%d 已不存在（%s），本次该方向不过滤", directionName(direction), ruleID, source))
				return nil
			}
			if !rule.Enabled {
				// 规则停用 = 不生效：比"报错"更符合直觉（停用规则就是想让它不起作用）。
				return nil
			}
			if rule.RuleType != direction {
				// 方向不符必须拦住：白名单落进 exclude 槽会**反转语义**（只采噪声、丢掉正常日志），
				// 那是丢数据级别的误用。忽略该方向并告警，绝不按槽位硬解释。
				resolved.Warnings = append(resolved.Warnings, fmt.Sprintf(
					"过滤规则 %s 的类型是 %s，却用在了 %s 方向（%s），为避免语义反转已忽略该方向",
					rule.Name, rule.RuleType, direction, source))
				return nil
			}
			return &rule
		}
		if serviceID != nil {
			if *serviceID == 0 {
				return nil // 显式关闭
			}
			return pick(*serviceID, "服务级覆盖")
		}
		if templateID != nil && *templateID != 0 {
			return pick(*templateID, "模板默认")
		}
		return nil
	}
	resolved.Include = resolve(logFilterRuleInclude, templateInclude, serviceInclude)
	resolved.Exclude = resolve(logFilterRuleExclude, templateExclude, serviceExclude)
	return resolved
}

func directionName(direction string) string {
	if direction == logFilterRuleExclude {
		return "排除"
	}
	return "保留"
}

// loadLogFilterRules 按 id 批量取规则（渲染前一次性取回，避免每台主机重复查）。
func (handler *Handler) loadLogFilterRules(context context.Context, ids []int64) (map[int64]logFilterRuleRef, error) {
	return loadLogFilterRulesPool(context, handler.db, ids)
}

// loadLogFilterRulesPool 同上，但取数走传入的 pool（保存校验在事务里跑，要读到未提交的规则变更）。
func loadLogFilterRulesPool(context context.Context, pool db.DBTX, ids []int64) (map[int64]logFilterRuleRef, error) {
	rules := map[int64]logFilterRuleRef{}
	queries := db.New(pool)
	for _, id := range ids {
		if _, exists := rules[id]; exists || id <= 0 {
			continue
		}
		row, err := queries.GetLogCollectionFilterRule(context, id)
		if err == sql.ErrNoRows {
			// 规则被删了：不在这里报错，由 resolveLogFilter 按"该方向不过滤 + 告警"处理。
			continue
		}
		if err != nil {
			return nil, err
		}
		rules[id] = logFilterRuleRef{
			ID: row.ID, Name: row.Name, Pattern: row.Pattern, RuleType: row.RuleType, Enabled: row.Enabled,
		}
	}
	return rules, nil
}

// nullableToPointer 把 SQL 可空整数转成"nil = 没配"的指针（渲染输入用的形态）。
func nullableToPointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	converted := value.Int64
	return &converted
}

// validFilterPattern 渲染前对正则再过一遍 RE2，并给出可展示的问题说明。
//
// 为什么还要这一道（保存时已经校验过）：渲染读的是**库里的存量值**——历史数据、别的环境导入、
// 直接改库都可能绕过保存校验；而主机上的 Filebeat 编译的是同一串正则，编译不过会让它起不来。
// 这里不是拦下发，而是"把这一条过滤降级成不过滤 + 告警"（见文件头"宁可多采不可不采"）。
func validFilterPattern(direction string, rule logFilterRuleRef) (string, string) {
	pattern := strings.TrimSpace(rule.Pattern)
	if pattern == "" {
		return "", fmt.Sprintf("%s过滤规则 %s 的正则为空，已忽略（空模式在 Filebeat 里会匹配全部）", directionName(direction), rule.Name)
	}
	if problem := validateRulePattern(directionName(direction)+"过滤正则", pattern); problem != "" {
		return "", fmt.Sprintf("过滤规则 %s 的正则 Filebeat 编译不过，已忽略（否则 Filebeat 无法启动）：%s", rule.Name, problem)
	}
	return pattern, ""
}
