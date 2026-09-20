package logcollect

import (
	"context"
	"strings"
	"time"

	"autoadmin/internal/assets"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
)

// 日志路径通配展开（按需，架构文档 §4.8 的补充能力）。
//
// 界面「解析后」列对含通配（* ? []）的路径提供「展开」，调 agent 列出该服务各承载实例上
// 真实匹配到的文件。展开只在用户点击时发生（一次一条服务×日志定义），所以这里可以放心地
// 逐实例调 agent——不要把它塞进 log-config 那种表格首屏接口。
//
// 与认证共用同一套宏展开（模板 macro_definitions → 服务 macro_values → 实例 runtime_variables
// + app_home），所以这里展开出来的模式与主机上 Filebeat 采的是同一串；实际文件清单由 agent
// 的 filepath.Glob 给出（见 dj_agent/internal/grpcfile/client.go），与认证抽样同一实现。
const logGlobPreviewTimeout = 20 * time.Second

// PreviewLogGlob 实现 assets.LogGlobPreviewer。
//
// 逐台承载实例展开：某台离线/路径匹配不到只在那一项写 Error，其余实例照常返回——展开是
// 只读展示，不能因为一台机器失败就整条日志都不给看。
func (handler *Handler) PreviewLogGlob(ctx context.Context, request assets.LogGlobPreviewRequest) (assets.LogGlobPreview, error) {
	previewContext, cancel := context.WithTimeout(ctx, logGlobPreviewTimeout)
	defer cancel()

	// 与认证同源：用展示口径的查询确认这条日志定义属于该服务，并取回路径模式。
	rows, err := assets.NewRepository(handler.db).ListServiceTemplateLogs(previewContext, request.ServiceID)
	if err != nil {
		return assets.LogGlobPreview{}, err
	}
	pattern := ""
	for index := range rows {
		if rows[index].LogDefinition == request.LogDefinitionID {
			pattern = rows[index].PathPattern
			break
		}
	}
	if pattern == "" {
		return assets.LogGlobPreview{}, assets.ErrNotFound
	}
	if handler.gateway == nil {
		return assets.LogGlobPreview{}, apperror.New(apperror.CodeInvalidArgument, "通配展开不可用：agent 数据面未接线")
	}

	result := assets.LogGlobPreview{PathPattern: pattern, Instances: []assets.LogGlobInstanceMatches{}}
	deploymentIDs, err := db.New(handler.db).ListServiceDeploymentIDs(previewContext, request.ServiceID)
	if err != nil {
		return assets.LogGlobPreview{}, err
	}
	// 同一主机上多个实例展开成同一路径时只调一次 agent（结果按主机+路径缓存）。
	cache := map[string][]string{}
	for _, deploymentID := range deploymentIDs {
		instance, instanceErr := db.New(handler.db).GetLogVerifyInstanceContext(previewContext, db.GetLogVerifyInstanceContextParams{
			DeploymentID: deploymentID, ServiceID: request.ServiceID, LogDefinitionID: request.LogDefinitionID,
		})
		if instanceErr != nil {
			// 该实例不在这个服务的这条日志的查询范围里（例如日志定义刚被换掉）：跳过。
			continue
		}
		item := assets.LogGlobInstanceMatches{
			HostInstanceName:       instance.HostInstanceName,
			DeploymentInstanceName: instance.DeploymentInstanceName,
		}
		if strings.TrimSpace(instance.HostInstanceName) == "" {
			item.Error = "实例所在主机未配置实例名，无法定位 dj-agent 会话"
			result.Instances = append(result.Instances, item)
			continue
		}
		// 与采集下发同一套宏展开（APP_HOME 取部署模板 app_home）。
		path := resolveMacros(instance.PathPattern,
			mergeMacroValues(
				templateMacroDefaults(string(instance.MacroDefinitions)),
				parseMacroJSON(string(instance.MacroValues)),
			),
			instanceMacros(string(instance.RuntimeVariables), instance.AppHome),
		)
		item.Pattern = path
		if strings.Contains(path, "${") {
			item.Error = "日志路径含未展开的宏，无法定位文件：" + path
			result.Instances = append(result.Instances, item)
			continue
		}
		// 不含通配：这条日志就是这一个文件，无需调 agent。
		if !hasGlobMeta(path) {
			item.Matches = []string{path}
			result.Instances = append(result.Instances, item)
			continue
		}
		cacheKey := instance.HostInstanceName + "\x00" + path
		if cached, ok := cache[cacheKey]; ok {
			item.Matches = cached
			result.Instances = append(result.Instances, item)
			continue
		}
		// 后端用 ListFiles 逐层展开（不依赖 agent 版本），按实例分组回给界面。
		files, globErr := handler.resolveRemoteGlob(previewContext, instance.HostInstanceName, path)
		if globErr != nil {
			item.Error = globErr.Error()
		} else {
			item.Matches = make([]string, 0, len(files))
			for _, file := range files {
				item.Matches = append(item.Matches, file.Path)
			}
		}
		cache[cacheKey] = item.Matches
		result.Instances = append(result.Instances, item)
	}
	return result, nil
}

// 编译期断言：logcollect 的 Handler 就是通配展开编排层要的执行器（接口定义在 assets 侧）。
var _ assets.LogGlobPreviewer = (*Handler)(nil)
