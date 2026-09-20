package logcollect

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"sort"
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
	// logFormatSampleLines 参与认证的最大记录数：单行模式下即尾部行数；多行模式下是
	// **尾部记录数**（不能按行截断，否则可能只截到某条超长堆栈的续行、误报未命中首行正则）。
	logFormatSampleLines = 50
	// logFormatSampleWindow 反向读取的字节窗口：ReadFileChunk 一次最多回 1MiB，
	// 所以窗口不放大到一次调用之外，否则要自己循环拼字节。
	logFormatSampleWindow = int64(1 << 20)
	// logFormatSampleTimeout 单次认证的总超时（含 agent 文件通道与 ES 往返）。
	// 认证是同步接口（人要等结果），不能让离线主机把它拖到 HTTP 超时。
	logFormatSampleTimeout = 20 * time.Second
	// logFormatAllDeploymentsTimeout 认证**全部承载实例**时的总超时：逐台主机取样，
	// 实例越多越慢，给足时间但仍设上限（HTTP 侧是 120s）。
	logFormatAllDeploymentsTimeout = 90 * time.Second
)

// VerifyLogFormat 实现 assets.LogFormatVerifier。
//
// 认证范围：source=instance 时认证一个或全部承载实例（`request.AllDeployments`），且
// **每个实例上所有匹配到的日志文件都要能解析出必备字段**（通配展开出的每个文件都验）。
// source=sample_log 时只用规则自带样例。
//
// 返回完整报告。**规则/pipeline/集群本身不可用返回 error**（这是配置错误，不是"验不过"）；
// 逐实例的软失败（主机离线、文件取不到）记在 Target.Error 上，不阻断其他实例。
func (handler *Handler) VerifyLogFormat(ctx context.Context, request assets.LogFormatVerifyRequest) (assets.LogFormatVerifyReport, error) {
	timeout := logFormatSampleTimeout
	if request.AllDeployments {
		timeout = logFormatAllDeploymentsTimeout
	}
	sampleContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 复用展示口径的查询取日志定义与规则：认证写入的指纹、弹窗比对的指纹必须同源。
	rows, err := assets.NewRepository(handler.db).ListServiceTemplateLogs(sampleContext, request.ServiceID)
	if err != nil {
		return assets.LogFormatVerifyReport{}, err
	}
	var definition *assets.ServiceTemplateLog
	for index := range rows {
		if rows[index].LogDefinition == request.LogDefinitionID {
			definition = &rows[index]
			break
		}
	}
	if definition == nil {
		return assets.LogFormatVerifyReport{}, assets.ErrNotFound
	}
	if definition.TemplateProcessingRuleID == nil {
		return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "该日志定义未关联解析规则，无法校验格式")
	}
	rule, err := db.New(handler.db).GetLogProcessingRule(sampleContext, *definition.TemplateProcessingRuleID)
	if err != nil {
		return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "解析规则不存在或已被删除")
	}
	if len(rule.PipelineBody) == 0 {
		return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "解析规则未配置 pipeline，无法校验格式")
	}
	pipeline := map[string]any{}
	if err = json.Unmarshal(rule.PipelineBody, &pipeline); err != nil || len(pipeline) == 0 {
		return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "解析规则的 pipeline 不是合法的 JSON 对象")
	}

	report := assets.LogFormatVerifyReport{Targets: []assets.LogFormatVerifyTarget{}}
	switch request.Source {
	case assets.LogFormatSourceInstance:
		// 只有这条依据需要 agent 文件通道（sample_log 依据只用规则样例，不受数据面影响）。
		if handler.gateway == nil {
			return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "按实例抽样认证不可用：agent 数据面未接线")
		}
		// cluster 用规则里的 pipeline body 走 inline _simulate：不依赖集群上是否已发布过同名
		// pipeline（发布发生在"保存规则"时，认证不应该因为发布顺序而误判）。与调试页同一做法。
		cluster, clusterErr := handler.loadElasticsearchClusterByID(sampleContext, rule.ClusterID)
		if clusterErr != nil {
			return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "解析规则所属的 Elasticsearch 集群不可用："+clusterErr.Error())
		}
		deploymentIDs, listErr := handler.resolveVerifyDeployments(sampleContext, request)
		if listErr != nil {
			return assets.LogFormatVerifyReport{}, listErr
		}
		if len(deploymentIDs) == 0 {
			return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "该服务没有可认证的部署实例（请先绑定实例）")
		}
		missingSet := map[string]bool{}
		for _, deploymentID := range deploymentIDs {
			target := handler.verifyDeployment(sampleContext, request, deploymentID, rule.MultilineEnabled, rule.StartPattern, cluster, pipeline)
			report.Targets = append(report.Targets, target)
			for _, field := range target.MissingFields {
				missingSet[field] = true
			}
		}
		report.MissingFields = sortedKeys(missingSet)
	case assets.LogFormatSourceSampleLog:
		if strings.TrimSpace(rule.SampleLog) == "" {
			return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "解析规则未配置样例日志，无法用它认证；请改用实例抽样")
		}
		docs, docErr := logSampleDocs(rule.SampleLog, rule.MultilineEnabled, rule.StartPattern)
		if docErr != nil {
			return assets.LogFormatVerifyReport{}, docErr
		}
		cluster, clusterErr := handler.loadElasticsearchClusterByID(sampleContext, rule.ClusterID)
		if clusterErr != nil {
			return assets.LogFormatVerifyReport{}, apperror.New(apperror.CodeInvalidArgument, "解析规则所属的 Elasticsearch 集群不可用："+clusterErr.Error())
		}
		missing, simErr := handler.simulateMissingFields(sampleContext, cluster, pipeline, docs)
		if simErr != nil {
			return assets.LogFormatVerifyReport{}, simErr
		}
		report.MissingFields = missing
	default:
		return assets.LogFormatVerifyReport{}, assets.ErrLogFormatSourceInvalid
	}
	return report, nil
}

// resolveVerifyDeployments 认证要覆盖的部署实例：全部承载实例，或请求指定的那一个。
func (handler *Handler) resolveVerifyDeployments(ctx context.Context, request assets.LogFormatVerifyRequest) ([]int64, error) {
	if !request.AllDeployments {
		return []int64{request.DeploymentID}, nil
	}
	return db.New(handler.db).ListServiceDeploymentIDs(ctx, request.ServiceID)
}

// verifyDeployment 认证一台部署实例：展开该实例上的日志路径，**逐个匹配文件取样**并跑 pipeline，
// 汇总缺失字段。任何一步取不到样例都写进 Target.Error（软失败，不影响其他实例）。
func (handler *Handler) verifyDeployment(ctx context.Context, request assets.LogFormatVerifyRequest, deploymentID int64, multiline bool, startPattern string, cluster elasticsearchCluster, pipeline map[string]any) assets.LogFormatVerifyTarget {
	target := assets.LogFormatVerifyTarget{DeploymentID: deploymentID, LogFiles: []string{}}
	instance, err := db.New(handler.db).GetLogVerifyInstanceContext(ctx, db.GetLogVerifyInstanceContextParams{
		DeploymentID: deploymentID, ServiceID: request.ServiceID, LogDefinitionID: request.LogDefinitionID,
	})
	if err != nil {
		target.Error = "部署实例不存在，或该实例没有绑定到这个逻辑服务"
		return target
	}
	target.DeploymentName = instance.DeploymentInstanceName
	target.HostInstanceName = instance.HostInstanceName
	if strings.TrimSpace(instance.HostInstanceName) == "" {
		target.Error = "实例所在主机未配置实例名，无法定位 dj-agent 会话"
		return target
	}
	// 与采集下发同一套宏展开：模板 macro_definitions 作默认值 → 服务 macro_values → 实例 runtime_variables
	//（APP_HOME 取部署模板 app_home）。展开不出来就必须报错，绝不能拿带 ${...} 的路径去读文件。
	path := resolveMacros(instance.PathPattern,
		mergeMacroValues(
			templateMacroDefaults(string(instance.MacroDefinitions)),
			parseMacroJSON(string(instance.MacroValues)),
		),
		instanceMacros(string(instance.RuntimeVariables), instance.AppHome),
	)
	if strings.Contains(path, "${") {
		target.Error = "日志路径含未展开的宏，无法定位文件：" + path
		return target
	}
	// Filebeat 会采集 glob 匹配到的**每一个**文件，认证必须逐个验，不能只抽样最新那个。
	files, globErr := handler.resolveRemoteGlob(ctx, instance.HostInstanceName, path)
	if globErr != nil {
		target.Error = globErr.Error()
		return target
	}
	docs := make([]any, 0, logFormatSampleLines)
	for _, file := range files {
		target.LogFiles = append(target.LogFiles, file.Path)
		if file.Size <= 0 {
			// 空文件（如刚轮转出来的）没有可校验内容，跳过而不是判失败。
			continue
		}
		text, readErr := handler.readRemoteTail(ctx, instance.HostInstanceName, file)
		if readErr != nil {
			target.Error = readErr.Error()
			return target
		}
		fileDocs, docErr := logSampleDocs(text, multiline, startPattern)
		if docErr != nil {
			target.Error = "文件 " + file.Path + "：" + docErr.Error()
			return target
		}
		docs = append(docs, fileDocs...)
	}
	if len(docs) == 0 {
		target.Error = "匹配到的日志文件都没有可校验的内容：" + path
		return target
	}
	// missing_fields 是所有文档的并集：任一文件缺必备字段即该实例不通过。
	missing, simErr := handler.simulateMissingFields(ctx, cluster, pipeline, docs)
	if simErr != nil {
		target.Error = simErr.Error()
		return target
	}
	target.MissingFields = missing
	return target
}

// simulateMissingFields 跑一次 inline _simulate 并取回缺失的必备字段。
func (handler *Handler) simulateMissingFields(ctx context.Context, cluster elasticsearchCluster, pipeline map[string]any, docs []any) ([]string, error) {
	data, err := handler.simulatePipeline(ctx, cluster, "", pipeline, docs)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "Elasticsearch 校验失败："+err.Error())
	}
	missing, _ := data["missing_fields"].([]string)
	return missing, nil
}

// sortedKeys 把集合拍成稳定有序的切片（缺失字段并集）。
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// readRemoteTail 反向读取某个远端文件的尾部窗口（1MiB，起点残行丢掉）。
//
// 用目录项给出的 size 再按 offset 读，而不是"读到 EOF"：ReadFileChunk 只回一个 chunk，
// length<=0 会从 offset 一路读到文件末尾，反而拿不到尾部内容。
func (handler *Handler) readRemoteTail(ctx context.Context, agentID string, file remoteGlobFile) (string, error) {
	if file.Size <= 0 {
		return "", apperror.New(apperror.CodeInvalidArgument, "远端日志文件为空，取不到样例："+file.Path)
	}
	offset := file.Size - logFormatSampleWindow
	if offset < 0 {
		offset = 0
	}
	chunk, err := handler.gateway.ReadFileChunk(ctx, agentID, file.Path, offset, file.Size-offset)
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
		return "", apperror.New(apperror.CodeInvalidArgument, "远端日志文件内容为空，取不到样例："+file.Path)
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

// tailRemoteFile 反向读取远端文件的尾部窗口（单文件场景，保留给测试与简单调用）。
//
// 路径含通配（* ? [）时取**最近修改**的普通文件；认证的完整范围由 verifyDeployment 负责。
func (handler *Handler) tailRemoteFile(ctx context.Context, agentID, path string) (string, error) {
	files, err := handler.resolveRemoteGlob(ctx, agentID, path)
	if err != nil {
		// agent 离线/路径匹配不到/指向目录都落这里：认证做不了，要让人看见原因。
		return "", err
	}
	target := files[0]
	if hasGlobMeta(path) {
		for _, file := range files[1:] {
			if file.Mtime > target.Mtime {
				target = file
			}
		}
	}
	return handler.readRemoteTail(ctx, agentID, target)
}

// logSampleDocs 把样例文本还原成 Filebeat 会送进 pipeline 的文档序列。
//
// 与规则调试页的 buildRawDocs 同一语义：默认逐行成一条记录；开启多行后
// `negate: true, match: after`（"不以首行正则开头的行并入上一行"），不需要续行正则。
// **与调试页唯一的口径差异**：多行模式下，第一条命中首行正则的记录之前的行直接丢掉——
// 那是采样窗口切出来的残尾（见下方 default 分支），不是一条真实记录。
// 不做这一步的话，多行日志会被当成一条单行文档，校验结果与真实采集不符。
//
// 与前端唯一的有意差异：正则在服务端用 Go 的 RE2 编译（Filebeat 也是 RE2），
// 而不是浏览器的 JS 正则——认证要测的是"主机上真正会跑的那套规则"。
//
// **每条文档都用 withFilebeatEventFields 补齐 Filebeat 自己的字段**（log/host/agent/…，见
// sample_event.go）：样例只给 `message` 的话，"处理器把 message 覆盖成对象"这类错误在认证里
// 永远看不见（ignore_missing 的 rename 会静默跳过），认证就变成了绿噪声。
func logSampleDocs(text string, multiline bool, startPattern string) ([]any, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if !multiline {
		// 单行模式：一行一条记录，只取尾部若干行（窗口是 1MiB，可能包含上万行）。
		if len(lines) > logFormatSampleLines {
			lines = lines[len(lines)-logFormatSampleLines:]
		}
		docs := make([]any, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			docs = append(docs, withFilebeatEventFields(map[string]any{"message": line}))
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
	if problem := validateRulePattern("首行正则", pattern); problem != "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, problem)
	}
	startExpression, err := regexp.Compile(pattern)
	if err != nil {
		// validateRulePattern 已经编译过一遍，这里理论到不了；留着是为了不给 panic 留口子。
		return nil, apperror.New(apperror.CodeInvalidArgument, "首行正则不合法："+err.Error())
	}

	// 多行模式：先按首行正则把**整段窗口**还原成记录，再只保留尾部 N 条记录。
	//
	// **不能先按行截断**（2026-09-20 现场）：Tomcat/Nacos 的 log_error.log 尾部经常是
	// 一条堆栈超长的记录（几十上百行 `\tat …`）。若先取尾 50 行，这 50 行全是续行、没有一条
	// 命中首行正则，就会误报"未命中首行正则，还原不出任何记录"——与规则本身无关。
	// 按记录截断则保证留下的每条记录都从首行开始，语义与 Filebeat 一致。
	records := make([]string, 0, logFormatSampleLines)
	buffer := make([]string, 0, 8)
	flush := func() {
		if len(buffer) == 0 {
			return
		}
		record := strings.Join(buffer, "\n")
		if len(records) < logFormatSampleLines {
			records = append(records, record)
		} else {
			// 只保留尾部 N 条：整体左移一位再追加，顺序不变、内存有界。
			records = append(records[1:], record)
		}
		buffer = buffer[:0]
	}
	for _, line := range lines {
		switch {
		case startExpression.MatchString(line):
			flush()
			buffer = append(buffer, line)
		case len(buffer) > 0:
			buffer = append(buffer, line)
		default:
			// 第一条记录之前、且不匹配首行正则的行：**丢掉**。
			//
			// 这些行是反向读取窗口切出来的"上一条记录的尾巴"（窗口起点落在某条多行记录的中间，
			// 于是堆栈的 `\tat …` 成了采样文本的开头）。真实采集时 Filebeat 的
			// `negate: true, match: after` 会把它们**并入上一条记录**，根本不会产生"只有 log_message、
			// 没有 log_level"的独立记录；可一旦当成独立记录送进认证判定，就会让判定报
			// "规则解析不出这些必备字段 —— log_level"（判定要求每一条记录都齐必备字段）。
			// 2026-09-19 现场：tomcat 的 catalina.out 认证一直被这条挡着，跟规则本身无关。
			continue
		}
	}
	flush()
	if len(records) == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"样例日志未命中首行正则，还原不出任何记录（首行正则：/"+pattern+"/，样例开头："+previewSampleText(text, 3)+"）")
	}
	docs := make([]any, 0, len(records))
	for _, record := range records {
		docs = append(docs, withFilebeatEventFields(map[string]any{"message": record}))
	}
	return docs, nil
}

// previewSampleText 摘出样例开头若干行（每行截断），用于认证失败时让人一眼看出
// "主机上到底采到了什么"，而不是只给一句"未命中首行正则"。
func previewSampleText(text string, maxLines int) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	parts := make([]string, 0, maxLines)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) > 120 {
			line = line[:120] + "…"
		}
		parts = append(parts, line)
		if len(parts) >= maxLines {
			break
		}
	}
	if len(parts) == 0 {
		return "（空）"
	}
	return strings.Join(parts, " | ")
}

// 编译期断言：logcollect 的 Handler 就是认证编排层要的执行器（接口定义在 assets 侧）。
var _ assets.LogFormatVerifier = (*Handler)(nil)
