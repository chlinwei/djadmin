package logcollect

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 采集配置差异（"我知道有差异，但不知道差在哪"）。
//
// **为什么必须读主机**：库里只落**指纹**（`config_fingerprint` / `service_fingerprints`），
// 不落内容，所以"差异内容"只能这样得到——后端渲染出期望片段（与下发同一套 renderHostLogConfig），
// 再用 agent 把主机上**已下发**的 `/etc/filebeat/inputs.d/*.yml` 读回来，逐文件比。
// 指纹仍然有用：它回答"这台机器当前是哪一版"，所以响应里一并给出（不一致 = 待下发）。
//
// 读不到的三种情况都**如实说明**，不给一个猜出来的差异：
//   - agent 数据面未接线 / 主机离线：`read_error` 有话说，期望侧内容照常返回（那是"将要下发什么"，
//     这一步不依赖主机）；
//   - 单文件读失败（权限/被删）：写在那个文件的 `read_error` 上；
//   - 片段超过读取上限：标 `applied_truncated`，界面提示"内容被截断，差异可能不完整"。
const (
	logConfigDiffTimeout = 25 * time.Second
	// 单个已下发片段的读取上限。片段是几十行 YAML（正常几 KB），上限只是防线：
	// 宁可标"截断"，也不把一台主机的配置整份拉进内存。
	logConfigDiffMaxFileBytes = 256 * 1024
)

// 文件状态。语义就是"要让主机的 inputs.d 变成期望的样子，需要做什么"。
const (
	logConfigDiffAdded     = "added"     // 期望有、主机上没有 → 会新增
	logConfigDiffRemoved   = "removed"   // 期望没有、主机上有 → 会被删除（agent 侧是全量替换语义）
	logConfigDiffChanged   = "changed"   // 两边都有但内容不同 → 会被覆盖
	logConfigDiffUnchanged = "unchanged" // 一致
)

// logConfigDiffFile 一个 inputs.d 片段的比对结果。期望侧与已下发侧的**内容都原样给出**，
// 界面自己做行级 diff（渲染样式属于展示问题，不该由后端定死）。
type logConfigDiffFile struct {
	Path     string `json:"path"`
	BaseName string `json:"base_name"`
	Status   string `json:"status"`
	// ServiceID / ServiceCode 从文件名解析（`<app>__<service>__<serviceID>__<log>.yml`），
	// 两侧都能解析——界面据此默认"只看本服务"，并把其他服务的片段折叠起来。
	ServiceID   int64  `json:"service_id"`
	ServiceCode string `json:"service_code"`
	// Expected / Applied 为该侧的完整内容；该侧没有这个文件时为 null（不是空串——
	// 空片段与"文件不存在"是两件事）。
	Expected         *string `json:"expected"`
	Applied          *string `json:"applied"`
	AppliedTruncated bool    `json:"applied_truncated"`
	// ReadError 单个文件读失败的原因（主机离线/权限/文件消失），该文件不参与"一致/不一致"判定。
	ReadError string `json:"read_error,omitempty"`
}

type logConfigDiffSummary struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	// Unread 没能比出来的文件数（读失败/截断），这些既不算一致也不算差异。
	Unread int `json:"unread"`
}

// logConfigDiffService 本次比较针对的那个服务（可选）。Pending 为 true = 这台主机上本服务的
// 配置待下发（子指纹不一致），与日志中心头部的"待下发 N 台"同源。
type logConfigDiffService struct {
	ServiceID                 int64  `json:"service_id"`
	ExpectedFingerprint       string `json:"expected_fingerprint"`
	AppliedFingerprint        string `json:"applied_fingerprint"`
	Pending                   bool   `json:"pending"`
}

// logConfigDiffResponse 配置差异的完整响应。
type logConfigDiffResponse struct {
	TargetID         int64  `json:"target_id"`
	HostID           int64  `json:"host_id"`
	HostIP           string `json:"host_ip"`
	HostInstanceName string `json:"host_instance_name"`
	// State 这台主机的配置态（synced/drift/never/unknown），与日志中心头部同一套判据。
	State               string `json:"state"`
	ExpectedFingerprint string `json:"expected_fingerprint"`
	AppliedFingerprint  string `json:"applied_fingerprint"`
	// AppliedFingerprintFromDB 已下发指纹来自库里（读主机失败时依然能说"库里记的是这一版"）。
	AppliedFingerprintFromDB string                 `json:"applied_fingerprint_from_db"`
	Service                  *logConfigDiffService  `json:"service"`
	Files                    []logConfigDiffFile    `json:"files"`
	Summary                  logConfigDiffSummary   `json:"summary"`
	// ReadError 整体读不到主机上的配置时的原因（agent 未接线/离线）。此时 files 里只有期望侧，
	// 界面要明说"下面是**将要下发**的内容，无法与主机上的现状对比"。
	ReadError string   `json:"read_error"`
	Warnings  []string `json:"warnings,omitempty"`
}

// GetHostLogConfigDiff GET /monitor/log-targets/:id/config-diff/?application_service_id=<可选>
//
// 回答"这次下发会改什么"：期望片段 vs 主机上已下发的片段，逐文件给出 added/removed/changed/unchanged
// 与两侧内容。**只读**：不写库、不下发、不改主机上的任何文件。
// 参数名用 c（而不是本包其他 handler 的 context）：这里要从**包级** context 派生一个只读超时，
// 参数名若叫 context 会把包名遮住。
func (handler *Handler) GetHostLogConfigDiff(c *gin.Context) {
	diffContext, cancel := context.WithTimeout(c.Request.Context(), logConfigDiffTimeout)
	defer cancel()

	targetID := parseID(c.Param("id"))
	if targetID < 1 {
		response.Error(c, apperror.ErrInvalidRequest)
		return
	}
	serviceID := parseID(c.Query("application_service_id"))
	queries := db.New(handler.db)
	target, err := loadLogTarget(diffContext, handler.db, targetID)
	if err != nil {
		response.Error(c, err)
		return
	}
	result := logConfigDiffResponse{
		TargetID: target.ID, HostID: target.HostID, HostIP: target.HostIP, HostInstanceName: target.HostName,
	}
	// 已下发指纹：库里记的那一版（下面读主机失败时也有话说）。
	fingerprints, err := queries.GetLogTargetFingerprints(diffContext, targetID)
	if err != nil {
		response.Error(c, err)
		return
	}
	result.AppliedFingerprintFromDB = strings.TrimSpace(fingerprints.ConfigFingerprint)

	// 期望侧：与下发/预览同一条渲染路径（同前缀、同宏、同排序），否则比出来的差异是假的。
	entries, instances, err := handler.loadHostLogRenderInput(diffContext, target.HostID)
	if err != nil {
		response.Error(c, err)
		return
	}
	cluster, clusterErr := queries.GetDefaultEnabledElasticsearchCluster(diffContext)
	outputIdentity := ""
	if clusterErr == nil {
		for index := range entries {
			entries[index].Prefix = cluster.IndexPrefix
		}
		outputIdentity, _ = filebeatOutputIdentity(cluster.Hosts, cluster.Username, cluster.VerifyTls)
	}
	rendered := renderHostLogConfig(entries, instances, outputIdentity)
	result.ExpectedFingerprint = rendered.Fingerprint
	result.Warnings = rendered.Warnings
	result.State = LogConfigSynced
	switch {
	case result.AppliedFingerprintFromDB == "":
		result.State = LogConfigNever
	case result.AppliedFingerprintFromDB != rendered.Fingerprint:
		result.State = LogConfigDrift
	}

	// 已下发侧：读主机上的 inputs.d。读不到就只回期望侧（并说明原因）。
	deployed, readErr := handler.readDeployedLogConfigFiles(diffContext, target.HostName)
	if readErr != nil {
		result.ReadError = readErr.Error()
	}
	files, summary := diffLogConfigFragments(rendered.Fragments, deployed)
	result.Files = files
	result.Summary = summary

	// 本服务视角：子指纹比对（与 service-config-state 同一条判据），以及本服务这份配置是否待下发。
	if serviceID > 0 {
		service := &logConfigDiffService{ServiceID: serviceID}
		service.ExpectedFingerprint = rendered.ServiceFingerprints[strconv.FormatInt(serviceID, 10)]
		applied := serviceFingerprintOf(fingerprints.ServiceFingerprints, serviceID)
		service.AppliedFingerprint = applied
		service.Pending = serviceConfigStatus(service.ExpectedFingerprint, applied, rendered.Fingerprint, result.AppliedFingerprintFromDB) != LogConfigSynced
		result.Service = service
	}
	response.Success(c, result)
}

// deployedLogConfigFragment 一台主机上已下发的片段（内容是读回来的）。
type deployedLogConfigFragment struct {
	Path      string
	Content   string
	Truncated bool
	ReadError string
}

// readDeployedLogConfigFiles 列出并读取主机上 `/etc/filebeat/inputs.d/*.yml`。
//
// 只读**这个目录下的 .yml**：路径来自 agent 的目录项，逐项校验基名（不允许含路径分隔符与
// 上跳段），所以不存在"读任意文件"的口子——这与下发写入的范围严格对称。
// 返回的 error 只表示"整批读不到"（未接线/离线/目录打不开），单文件失败记在该文件的 ReadError 上。
func (handler *Handler) readDeployedLogConfigFiles(ctx context.Context, agentID string) ([]deployedLogConfigFragment, error) {
	if handler.gateway == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "读不到主机上已下发的配置：agent 数据面未接线")
	}
	if strings.TrimSpace(agentID) == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "读不到主机上已下发的配置：主机未配置实例名，无法定位 agent 会话")
	}
	listing, err := handler.gateway.ListFiles(ctx, agentID, filebeatInputsDir)
	if err != nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "读不到主机上已下发的配置（主机 agent 可能离线）："+err.Error())
	}
	if listing == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument, "读不到主机上已下发的配置：agent 未返回结果")
	}
	if remoteError := listing.GetError(); remoteError != "" {
		return nil, apperror.New(apperror.CodeInvalidArgument, "读不到主机上已下发的配置："+remoteError+"（目录 "+filebeatInputsDir+"）")
	}
	names := make([]string, 0, len(listing.GetEntries()))
	for _, entry := range listing.GetEntries() {
		if entry.GetIsDir() || !strings.HasSuffix(entry.GetName(), ".yml") {
			continue
		}
		if !safeConfigBaseName(strings.TrimSuffix(entry.GetName(), ".yml")) {
			// 目录里有怪名字（含分隔符/上跳段）：不读它，也不让它在差异里出现。
			continue
		}
		names = append(names, entry.GetName())
	}
	sort.Strings(names)

	fragments := make([]deployedLogConfigFragment, 0, len(names))
	for _, name := range names {
		item := deployedLogConfigFragment{Path: filebeatInputsDir + "/" + name}
		stat, statErr := handler.gateway.StatFile(ctx, agentID, item.Path)
		if statErr != nil {
			item.ReadError = "读取失败：" + statErr.Error()
			fragments = append(fragments, item)
			continue
		}
		if stat == nil || stat.GetError() != "" {
			item.ReadError = firstNonEmpty(stat.GetError(), "文件状态未知")
			fragments = append(fragments, item)
			continue
		}
		length := stat.GetSize()
		if length > logConfigDiffMaxFileBytes {
			length = logConfigDiffMaxFileBytes
			item.Truncated = true
		}
		chunk, chunkErr := handler.gateway.ReadFileChunk(ctx, agentID, item.Path, 0, length)
		if chunkErr != nil {
			item.ReadError = "读取失败：" + chunkErr.Error()
			fragments = append(fragments, item)
			continue
		}
		if chunk == nil || chunk.GetError() != "" {
			item.ReadError = firstNonEmpty(chunk.GetError(), "文件内容为空或不可读")
			fragments = append(fragments, item)
			continue
		}
		item.Content = string(chunk.GetData())
		fragments = append(fragments, item)
	}
	return fragments, nil
}

// safeConfigBaseName 片段基名白名单：字母数字与 `_ . -`，且不含路径分隔符与 `..`。
// 下发写入的文件名由 configBaseName 生成（`<app>__<svc>__<id>__<log>`），这里是读回来的那道闸。
func safeConfigBaseName(base string) bool {
	if base == "" || strings.Contains(base, "..") {
		return false
	}
	for _, char := range base {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		case char == '_' || char == '-' || char == '.':
		default:
			return false
		}
	}
	return true
}

// diffLogConfigFragments 纯函数：期望片段 vs 已下发片段 → 逐文件结果 + 摘要。
//
// 抽成纯函数的原因：这是整个功能里唯一"会算错"的地方（把"要删"算成"要加"会让用户误判下发的后果），
// 而它不需要数据库或主机就能直测。
func diffLogConfigFragments(expected []logConfigFragment, deployed []deployedLogConfigFragment) ([]logConfigDiffFile, logConfigDiffSummary) {
	expectedByPath := make(map[string]string, len(expected))
	for _, fragment := range expected {
		expectedByPath[fragment.Path] = fragment.Content
	}
	deployedByPath := make(map[string]deployedLogConfigFragment, len(deployed))
	for _, fragment := range deployed {
		deployedByPath[fragment.Path] = fragment
	}

	paths := make([]string, 0, len(expectedByPath)+len(deployedByPath))
	for path := range expectedByPath {
		paths = append(paths, path)
	}
	for path := range deployedByPath {
		if _, ok := expectedByPath[path]; !ok {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	summary := logConfigDiffSummary{}
	files := make([]logConfigDiffFile, 0, len(paths))
	for _, path := range paths {
		expectedContent, hasExpected := expectedByPath[path]
		applied, hasApplied := deployedByPath[path]
		item := logConfigDiffFile{
			Path: path, BaseName: strings.TrimSuffix(filepath.Base(path), ".yml"),
			ServiceCode: serviceCodeFromBaseName(strings.TrimSuffix(filepath.Base(path), ".yml")),
		}
		item.ServiceID = serviceIDFromBaseName(item.BaseName)
		if hasExpected {
			content := expectedContent
			item.Expected = &content
		}
		if hasApplied {
			content := applied.Content
			item.Applied = &content
			item.AppliedTruncated = applied.Truncated
			item.ReadError = applied.ReadError
		}
		switch {
		case item.ReadError != "" || item.AppliedTruncated:
			// 读不全就不下结论：既不说"一致"（可能已经改了），也不说"不一致"（可能没改）。
			item.Status = logConfigDiffChanged
			summary.Unread++
		case hasExpected && hasApplied:
			if expectedContent == applied.Content {
				item.Status = logConfigDiffUnchanged
				summary.Unchanged++
			} else {
				item.Status = logConfigDiffChanged
				summary.Changed++
			}
		case hasExpected:
			item.Status = logConfigDiffAdded
			summary.Added++
		default:
			item.Status = logConfigDiffRemoved
			summary.Removed++
		}
		files = append(files, item)
	}
	return files, summary
}

// serviceIDFromBaseName 从片段基名 `<app>__<service>__<serviceID>__<logname>` 里取服务 id。
// 取不到（格式异常/历史命名）返回 0：界面据此把这一行归到"无法归属"。
func serviceIDFromBaseName(baseName string) int64 {
	parts := strings.Split(baseName, "__")
	if len(parts) < 4 {
		return 0
	}
	// 服务 id 是第 3 段；日志名自己可能含 `__`，所以从后面数不合适，按固定位置取。
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id < 1 {
		return 0
	}
	return id
}

func serviceCodeFromBaseName(baseName string) string {
	parts := strings.Split(baseName, "__")
	if len(parts) < 4 {
		return ""
	}
	return parts[1]
}
