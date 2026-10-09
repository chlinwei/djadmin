package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
//	改规则（pipeline_body、输入格式、多行参数、正则） → RuleParsingSignature
//	换挂另一条规则                           → RuleID
//	应用/中间件版本升级                       → ApplicationVersion
//
// 另加服务级 MacroValues：改宏等于换了一个文件在采，格式可能不同。
// 规则描述、备注、名称、样例及更新时间不是解析配置，不参与指纹。
// 实例级 runtime_variables 不进指纹（逐实例而异），改它需要人工重新认证。
type logFormatFingerprintInput struct {
	LogDefinition        int64
	LogName              string
	PathPattern          string
	RuleID               int64
	RuleParsingSignature string
	MacroValues          string
	ApplicationVersion   int64
}

type logFormatRuleInput struct {
	InputFormat         string          `json:"input_format"`
	MultilineEnabled    bool            `json:"multiline_enabled"`
	StartPattern        string          `json:"start_pattern"`
	ContinuationPattern string          `json:"continuation_pattern"`
	FlushTimeout        uint32          `json:"flush_timeout"`
	PipelineBody        json.RawMessage `json:"pipeline_body"`
}

func logFormatRuleSignature(input logFormatRuleInput) (string, error) {
	// JSON 对象键顺序/排版不影响解析；保留处理器数组顺序和数值精度。
	decoder := json.NewDecoder(bytes.NewReader(input.PipelineBody))
	decoder.UseNumber()
	var pipeline map[string]any
	if err := decoder.Decode(&pipeline); err != nil {
		return "", fmt.Errorf("解析规则 pipeline_body 无效: %w", err)
	}
	if pipeline == nil {
		return "", fmt.Errorf("解析规则 pipeline_body 必须是对象")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", fmt.Errorf("解析规则 pipeline_body 必须是单个 JSON 对象")
	}
	// Pipeline 顶层元数据不改变执行行为，其余字段全部保留，避免遗漏失败处理等行为配置。
	delete(pipeline, "description")
	delete(pipeline, "_meta")
	delete(pipeline, "version")
	body, err := json.Marshal(pipeline)
	if err != nil {
		return "", fmt.Errorf("序列化解析规则 pipeline_body: %w", err)
	}
	input.PipelineBody = body
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("序列化解析规则配置: %w", err)
	}
	return string(encoded), nil
}

// logFormatFingerprintOf 计算认证指纹（sha256 前 32 位十六进制，够用且能塞进 varchar(64)）。
func logFormatFingerprintOf(input logFormatFingerprintInput) string {
	payload := strings.Join([]string{
		fmt.Sprintf("log_definition=%d", input.LogDefinition),
		"name=" + input.LogName,
		"path_pattern=" + input.PathPattern,
		fmt.Sprintf("rule=%d", input.RuleID),
		"rule_parsing=" + input.RuleParsingSignature,
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
