package logcollect

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"autoadmin/internal/assets"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
)

// 日志格式认证的"取样例 + 跑 ES 校验"执行器（架构文档 §4.8）。
//
// 编排（指纹比对、写回 format_verified_*）在 assets 侧；这里只负责回答一个问题：
// **这一份样例日志，能不能被规则解析出必备字段**。分两个依据：
//   - instance：从部署实例所在主机上取日志文件的最近若干行（与 Filebeat 采的是同一个文件）；
//   - sample_log：用处理规则自带的样例日志（规则编辑页里粘的那段）。
//
// 判定口径与规则调试页完全一致——两处都走 simulatePipeline + missingRequiredDocumentFields，
// 否则会出现"调试页看起来通过、认证却不通过"。
const (
	// logFormatSampleLines 参与认证的最大原始行数（取文件尾部若干行）。
	logFormatSampleLines = 50
	// logFormatSampleWindow 反向读取的字节窗口：ReadFileChunk 一次最多回 1MiB，
	// 所以窗口不放大到一次调用之外，否则要自己循环拼字节。
	logFormatSampleWindow = int64(1 << 20)
	// logFormatSampleTimeout 单次认证的总超时（含 agent 文件通道与 ES 往返）。
	// 认证是同步接口（人要等结果），不能让离线主机把它拖到 HTTP 超时。
	logFormatSampleTimeout = 20 * time.Second
)

// VerifyLogFormat 实现 assets.LogFormatVerifier。
//
// 返回缺失的必备字段（空 = 通过）。**取不到样例必须返回 error**，不能返回空切片：
// 认证编排层把"空缺失 = 通过"当作写入认证结果的依据，返回空切片会把"没验成"记成"已验证"。
func (handler *Handler) VerifyLogFormat(ctx context.Context, request assets.LogFormatVerifyRequest) ([]string, error) {
	sampleContext, cancel := context.WithTimeout(ctx, logFormatSampleTimeout)
	defer cancel()

	// 复用展示口径的查询取日志定义与规则：认证写入的指纹、弹窗比对的指纹必须同源。
	rows, err := assets.NewRepository(handler.db).ListServiceTemplateLogs(sampleContext, request.ServiceID)
	if err != nil {
		return nil, err
	}
	var definition *assets.ServiceTemplateLog
	for index := range rows {
		if rows[index].LogDefinition == request.LogDefinitionID {
			definition = &rows[index]
			break
		}
	}
	if definition == nil {
		return nil, assets.ErrNotFound
	}
	if definition.TemplateProcessingRuleID == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "该日志定义未关联解析规则，无法校验格式")
	}
	rule, err := db.New(handler.db).GetLogProcessingRule(sampleContext, *definition.TemplateProcessingRuleID)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "解析规则不存在或已被删除")
	}

	var sampleText string
	switch request.Source {
	case assets.LogFormatSourceInstance:
		sampleText, err = handler.sampleInstanceLogText(sampleContext, request, definition.PathPattern)
		if err != nil {
			return nil, err
		}
	case assets.LogFormatSourceSampleLog:
		if strings.TrimSpace(rule.SampleLog) == "" {
			return nil, apperror.New(apperror.CodeInvalidArgument, "解析规则未配置样例日志，无法用它认证；请改用实例抽样")
		}
		sampleText = rule.SampleLog
	default:
		return nil, assets.ErrLogFormatSourceInvalid
	}

	docs, err := logSampleDocs(sampleText, rule.MultilineEnabled, rule.StartPattern)
	if err != nil {
		return nil, err
	}
	if len(rule.PipelineBody) == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "解析规则未配置 pipeline，无法校验格式")
	}
	pipeline := map[string]any{}
	if err = json.Unmarshal(rule.PipelineBody, &pipeline); err != nil || len(pipeline) == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "解析规则的 pipeline 不是合法的 JSON 对象")
	}
	cluster, err := handler.loadElasticsearchClusterByID(sampleContext, rule.ClusterID)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "解析规则所属的 Elasticsearch 集群不可用："+err.Error())
	}
	// 用规则里的 pipeline body 走 inline _simulate：不依赖集群上是否已经发布过同名 pipeline
	// （发布发生在"保存规则"时，认证不应该因为发布顺序而误判）。与调试页同一做法。
	data, err := handler.simulatePipeline(sampleContext, cluster, "", pipeline, docs)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "Elasticsearch 校验失败："+err.Error())
	}
	missing, _ := data["missing_fields"].([]string)
	return missing, nil
}

// sampleInstanceLogText 取部署实例上日志文件的尾部内容（instance 依据）。
func (handler *Handler) sampleInstanceLogText(ctx context.Context, request assets.LogFormatVerifyRequest, pathPattern string) (string, error) {
	// 只有这条依据需要 agent 文件通道（sample_log 依据只用规则样例，不受数据面影响）。
	if handler.gateway == nil {
		return "", apperror.New(apperror.CodeInvalidArgument, "按实例抽样认证不可用：agent 数据面未接线")
	}
	instance, err := db.New(handler.db).GetLogVerifyInstanceContext(ctx, db.GetLogVerifyInstanceContextParams{
		DeploymentID: request.DeploymentID, ServiceID: request.ServiceID, LogDefinitionID: request.LogDefinitionID,
	})
	if err != nil {
		return "", apperror.New(apperror.CodeInvalidArgument, "部署实例不存在，或该实例没有绑定到这个逻辑服务")
	}
	if strings.TrimSpace(instance.HostInstanceName) == "" {
		return "", apperror.New(apperror.CodeInvalidArgument, "实例所在主机未配置实例名，无法定位 dj-agent 会话")
	}
	// 与采集下发同一套宏展开：模板 macro_definitions 作默认值 → 服务 macro_values → 实例 runtime_variables
	//（APP_HOME 取部署模板 app_home）。展开不出来就必须报错，绝不能拿带 ${...} 的路径去读文件。
	path := resolveMacros(pathPattern,
		mergeMacroValues(
			templateMacroDefaults(string(instance.MacroDefinitions)),
			parseMacroJSON(string(instance.MacroValues)),
		),
		instanceMacros(string(instance.RuntimeVariables), instance.AppHome),
	)
	if strings.Contains(path, "${") {
		return "", apperror.New(apperror.CodeInvalidArgument, "日志路径含未展开的宏，无法定位文件："+path)
	}
	return handler.tailRemoteFile(ctx, instance.HostInstanceName, path)
}

// tailRemoteFile 反向读取远端文件的尾部窗口。
//
// 用 StatFile 拿大小再按 offset 读最后 1MiB，而不是"读到 EOF"：ReadFileChunk 只回一个
// chunk，length<=0 会从 offset 一路读到文件末尾，反而拿不到尾部内容。
func (handler *Handler) tailRemoteFile(ctx context.Context, agentID, path string) (string, error) {
	stat, err := handler.gateway.StatFile(ctx, agentID, path)
	if err != nil {
		// ErrAgentOffline / ctx 超时都落这里：主机不在线时认证做不了，要让人看见原因。
		return "", apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败（主机 agent 可能离线）："+err.Error())
	}
	if stat == nil {
		return "", apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败：agent 未返回结果")
	}
	if remoteError := stat.GetError(); remoteError != "" {
		return "", apperror.New(apperror.CodeInvalidArgument, "远端日志文件不可读："+remoteError+"（路径 "+path+"）")
	}
	if stat.GetIsDir() {
		return "", apperror.New(apperror.CodeInvalidArgument, "日志路径指向的是目录，不是文件："+path)
	}
	if stat.GetSize() <= 0 {
		return "", apperror.New(apperror.CodeInvalidArgument, "远端日志文件为空，取不到样例："+path)
	}
	offset := stat.GetSize() - logFormatSampleWindow
	if offset < 0 {
		offset = 0
	}
	// 后续调用必须用 agent 解析后的绝对路径（相对路径/软链接在 agent 侧展开过）。
	chunk, err := handler.gateway.ReadFileChunk(ctx, agentID, stat.GetNormalizedPath(), offset, stat.GetSize()-offset)
	if err != nil {
		return "", apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败："+err.Error())
	}
	if chunk == nil {
		return "", apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败：agent 未返回内容")
	}
	if remoteError := chunk.GetError(); remoteError != "" {
		return "", apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败："+remoteError)
	}
	data := chunk.GetData()
	if len(data) == 0 {
		return "", apperror.New(apperror.CodeInvalidArgument, "远端日志文件内容为空，取不到样例："+path)
	}
	// 反向读取的窗口起点几乎必然落在某行中间：首行是残行，丢掉它。
	// 不丢的话会把半行日志当成一条完整记录喂进 pipeline，与真实采集不一致。
	if offset > 0 {
		if index := bytes.IndexByte(data, '\n'); index >= 0 {
			data = data[index+1:]
		}
	}
	return string(data), nil
}

// logSampleDocs 把样例文本还原成 Filebeat 会送进 pipeline 的文档序列。
//
// 与规则调试页的 buildRawDocs 同一语义：默认逐行成一条记录；开启多行后
// `negate: true, match: after`（"不以首行正则开头的行并入上一行"），不需要续行正则。
// 不做这一步的话，多行日志会被当成一条单行文档，校验结果与真实采集不符。
//
// 与前端唯一的有意差异：正则在服务端用 Go 的 RE2 编译（Filebeat 也是 RE2），
// 而不是浏览器的 JS 正则——认证要测的是"主机上真正会跑的那套规则"。
func logSampleDocs(text string, multiline bool, startPattern string) ([]any, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	// 只取尾部若干行：窗口是 1MiB，可能包含上万行，全部送进 _simulate 没有必要。
	if len(lines) > logFormatSampleLines {
		lines = lines[len(lines)-logFormatSampleLines:]
	}
	docs := make([]any, 0, len(lines))
	if !multiline {
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			docs = append(docs, map[string]any{"message": line})
		}
		if len(docs) == 0 {
			return nil, apperror.New(apperror.CodeInvalidArgument, "样例日志为空，取不到可校验的内容")
		}
		return docs, nil
	}
	pattern := strings.TrimSpace(startPattern)
	if pattern == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "规则启用了多行合并但未配置首行正则，无法还原样例")
	}
	startExpression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "首行正则不合法："+err.Error())
	}
	buffer := make([]string, 0, 8)
	flush := func() {
		if len(buffer) == 0 {
			return
		}
		docs = append(docs, map[string]any{"message": strings.Join(buffer, "\n")})
		buffer = buffer[:0]
	}
	for _, line := range lines {
		switch {
		case startExpression.MatchString(line):
			flush()
			buffer = append(buffer, line)
		case len(buffer) > 0:
			buffer = append(buffer, line)
		case strings.TrimSpace(line) != "":
			// 首行之前的前导行（横幅、启动日志）单独成一条。
			docs = append(docs, map[string]any{"message": line})
		}
	}
	flush()
	if len(docs) == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "样例日志未命中首行正则，还原不出任何记录")
	}
	return docs, nil
}

// 编译期断言：logcollect 的 Handler 就是认证编排层要的执行器（接口定义在 assets 侧）。
var _ assets.LogFormatVerifier = (*Handler)(nil)
