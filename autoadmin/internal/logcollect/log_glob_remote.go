package logcollect

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"autoadmin/internal/shared/apperror"
)

// 通配路径的后端展开：用 agent 既有的 ListFiles 逐层列出目录、按路径段做 filepath.Match。
//
// 为什么放在后端而不是 agent：路径通配（`/var/log/*.log`、`/var/log/*/*/*.log`）是日志采集
// 的常规写法，而主机上跑的 agent 未必是最新版本。把展开放在后端只依赖 ListFiles（早已存在），
// 已部署的 agent **不用升级**即可让「实例抽样认证」与界面「解析后」列的展开生效。
//
// 语义与 Filebeat/`filepath.Glob` 对齐：每个路径段内的 `*` / `?` / `[]` 在**单层内**匹配；
// 段与段之间的 `/` 是字面量。**不支持 `**`**（现场约定不会出现；Filebeat 的 recursive_glob
// 若开启会展开 `**`，届时这里是已知差异）。
const (
	// maxGlobDirListings 单次展开最多列多少个目录，防止通配踩进超大目录树把 agent 拖垮。
	maxGlobDirListings = 200
	// maxGlobMatches 单次展开最多返回多少个文件；超出即截断（展示与抽样都够用）。
	maxGlobMatches = 500
)

// remoteGlobFile 一次通配匹配到的文件（信息直接来自 ListFiles 的目录项）。
type remoteGlobFile struct {
	Path  string
	Size  int64
	Mtime int64
}

// hasGlobMeta 判断路径是否含通配元字符。
func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

// resolveRemoteGlob 展开一个**绝对**通配路径，返回匹配到的普通文件（按路径排序）。
//
// 路径不含通配时直接 `StatFile` 返回这一个文件（调用方无需区分）；匹配不到返回
// `路径未匹配到任何日志文件` 的可读错误，而不是静默空清单。
func (handler *Handler) resolveRemoteGlob(ctx context.Context, agentID, pattern string) ([]remoteGlobFile, error) {
	if !hasGlobMeta(pattern) {
		stat, err := handler.gateway.StatFile(ctx, agentID, pattern)
		if err != nil {
			return nil, apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败（主机 agent 可能离线）："+err.Error())
		}
		if stat == nil {
			return nil, apperror.New(apperror.CodeInvalidArgument, "读取远端日志文件失败：agent 未返回结果")
		}
		if remoteError := stat.GetError(); remoteError != "" {
			return nil, apperror.New(apperror.CodeInvalidArgument, "远端日志文件不可读："+remoteError+"（路径 "+pattern+"）")
		}
		if stat.GetIsDir() {
			return nil, apperror.New(apperror.CodeInvalidArgument, "日志路径指向的是目录，不是文件："+pattern)
		}
		return []remoteGlobFile{{Path: stat.GetNormalizedPath(), Size: stat.GetSize(), Mtime: stat.GetMtime()}}, nil
	}
	if !strings.HasPrefix(pattern, "/") {
		return nil, apperror.New(apperror.CodeInvalidArgument, "通配日志路径必须是绝对路径："+pattern)
	}

	segments := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	firstMeta := 0
	for index, segment := range segments {
		if hasGlobMeta(segment) {
			firstMeta = index
			break
		}
	}
	baseDir := "/" + strings.Join(segments[:firstMeta], "/")
	baseDir = strings.TrimSuffix(baseDir, "/")
	if baseDir == "" {
		baseDir = "/"
	}

	results := make([]remoteGlobFile, 0, 8)
	listings := 0
	if err := handler.walkRemoteGlob(ctx, agentID, baseDir, segments[firstMeta:], &results, &listings); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "路径未匹配到任何日志文件："+pattern)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results, nil
}

// walkRemoteGlob 递归列目录并匹配路径段。
//
// 目录列不出来（不存在/无权限）时直接跳过这一支——通配的意义就是"能匹配到的都要，匹配不到
// 的不算错"；只有"整棵树一个都没命中"才由调用方报 no-match。
func (handler *Handler) walkRemoteGlob(ctx context.Context, agentID, dir string, segments []string, results *[]remoteGlobFile, listings *int) error {
	if len(segments) == 0 || len(*results) >= maxGlobMatches || *listings >= maxGlobDirListings {
		return nil
	}
	*listings++
	response, err := handler.gateway.ListFiles(ctx, agentID, dir)
	if err != nil {
		// agent 离线/超时：让上层报"主机不可达"，而不是含糊的"没匹配到"。
		return apperror.New(apperror.CodeInvalidArgument, "读取远端日志目录失败（主机 agent 可能离线）："+err.Error())
	}
	if response == nil || response.GetError() != "" {
		return nil
	}
	current := response.GetCurrentPath()
	if current == "" {
		current = dir
	}
	last := len(segments) == 1
	for _, entry := range response.GetEntries() {
		if len(*results) >= maxGlobMatches || *listings >= maxGlobDirListings {
			return nil
		}
		matched, matchErr := filepath.Match(segments[0], entry.GetName())
		if matchErr != nil || !matched {
			continue
		}
		fullPath := filepath.Join(current, entry.GetName())
		switch {
		case last && !entry.GetIsDir():
			*results = append(*results, remoteGlobFile{Path: fullPath, Size: entry.GetSize(), Mtime: entry.GetMtime()})
		case !last && entry.GetIsDir():
			if err = handler.walkRemoteGlob(ctx, agentID, fullPath, segments[1:], results, listings); err != nil {
				return err
			}
		}
	}
	return nil
}
