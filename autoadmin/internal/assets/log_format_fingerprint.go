package assets

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// 日志格式认证（"一次认证 + 指纹失效"）的指纹计算。
//
// 模型：新增服务/开启采集时**抽样校验一次**日志格式能否被该规则解析出必备字段；通过后不再持续
// 检查，只在**格式指纹变化**时要求重新认证。指纹只由库里的配置算出，不依赖主机、不查 ES，
// 所以"认证是否过期"是纯读库比对（见 ListServiceTemplateLogs 的 format_state）。
//
// 进指纹的输入对应四类"会改格式"的变化（架构文档 §4.8 的表格）：
//
//	换模板 / 模板增删日志定义 / 改名 / 改路径 → LogDefinition / LogName / PathPattern
//	改规则（pipeline_body、多行参数、首行正则） → RuleUpdatedAt（规则保存一定会更新它）
//	换挂另一条规则                           → RuleID
//	应用/中间件版本升级                       → ApplicationVersion
//
// 另加服务级 MacroValues：改宏等于换了一个文件在采，格式可能不同。
// 用 RuleUpdatedAt 而不是 pipeline_body 的内容签名，是为了让本包不必依赖 logcollect 的
// pipelineSignature；代价是"只改规则说明"也会让认证失效一次（更保守，可接受）。
// 实例级 runtime_variables 不进指纹（逐实例而异），改它需要人工重新认证。
type logFormatFingerprintInput struct {
	LogDefinition      int64
	LogName            string
	PathPattern        string
	RuleID             int64
	RuleUpdatedAt      time.Time
	MacroValues        string
	ApplicationVersion int64
}

// logFormatFingerprintOf 计算认证指纹（sha256 前 32 位十六进制，够用且能塞进 varchar(64)）。
func logFormatFingerprintOf(input logFormatFingerprintInput) string {
	payload := strings.Join([]string{
		fmt.Sprintf("log_definition=%d", input.LogDefinition),
		"name=" + input.LogName,
		"path_pattern=" + input.PathPattern,
		fmt.Sprintf("rule=%d", input.RuleID),
		// 规则更新时间按微秒截断：MySQL datetime(6) 与 PG timestamp(6) 都是微秒精度，
		// 直接格式化 time.Time 会带上驱动回读时的时区/单调时钟差异。
		fmt.Sprintf("rule_updated_at=%d", input.RuleUpdatedAt.UTC().UnixMicro()),
		"macros=" + strings.TrimSpace(input.MacroValues),
		fmt.Sprintf("application_version=%d", input.ApplicationVersion),
	}, "\n")
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", digest)[:32]
}

// 日志格式认证状态（服务弹窗「格式校验」列）。
const (
	// formatStateUnverified：从未认证过（存量数据、新开启的日志都是这个状态）。
	formatStateUnverified = "unverified"
	// formatStateVerified：认证通过，且指纹与认证时一致。
	formatStateVerified = "verified"
	// formatStateNeedsRecheck：认证过，但模板/规则/宏/应用版本变了 → 要求重新认证。
	formatStateNeedsRecheck = "needs_recheck"
)
