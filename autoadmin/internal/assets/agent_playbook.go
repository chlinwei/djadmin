package assets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

// agent 安装模板是 Agent 安装/更新的唯一配置源：install 直接以它执行 ansible-playbook；
// update 从中提取 config.env 与 systemd unit 的 copy 内容下发给 agent 自更新。
// 模板存于 automation_playbook_template 表（category='agent'），通过模板管理页面编辑/
// 上传/下载，无任何磁盘文件兜底；查不到或内容不合法时入口直接报错。

const agentInstallTemplateCategory = "agent"

var (
	agentVarIDPattern   = regexp.MustCompile(`\{\{\s*dj_agent_id\s*\}\}`)
	agentVarAddrPattern = regexp.MustCompile(`\{\{\s*dj_agent_grpc_addr\s*\}\}`)
)

const (
	agentEnvConfigDest   = "/etc/dj-agent/config.env"
	agentSystemdUnitDest = "/usr/lib/systemd/system/dj-agent.service"
)

// loadAgentInstallPlaybook 从模板表读取 Agent 安装专用模板（category='agent'），
// 约定该分类下只有一条模板；查不到或多条时直接报错，不静默兜底。
func (handler *Handler) loadAgentInstallPlaybook() (string, error) {
	var playbook string
	err := handler.service.repository.pool.QueryRowContext(context.Background(),
		`SELECT content FROM automation_playbook_template WHERE category=? ORDER BY id DESC`, agentInstallTemplateCategory).Scan(&playbook)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("未找到 Agent 安装专用模板（automation_playbook_template.category=%s），请先在模板管理中创建", agentInstallTemplateCategory)
	}
	if err != nil {
		return "", fmt.Errorf("读取 Agent 安装专用模板失败: %w", err)
	}
	if strings.TrimSpace(playbook) == "" {
		return "", fmt.Errorf("Agent 安装专用模板内容为空，请先在模板管理中维护该模板")
	}
	return playbook, nil
}

// playbookCopyContents 解析 playbook，收集所有 copy 模块任务的 dest→content。
func playbookCopyContents(playbook string) (map[string]string, error) {
	var parsed any
	if err := yaml.Unmarshal([]byte(playbook), &parsed); err != nil {
		return nil, fmt.Errorf("解析 agent_install.yml 失败: %w", err)
	}
	contents := map[string]string{}
	collectCopyContents(parsed, contents)
	return contents, nil
}

// playbookAgentTemplates 返回 config.env 与 systemd unit 的内容模板（含未渲染的
// Jinja 变量）；playbook 被改坏、缺少对应 copy 任务时直接报错，不静默兜底。
func playbookAgentTemplates(contents map[string]string) (envTemplate, unitTemplate string, err error) {
	envTemplate = strings.TrimSpace(contents[agentEnvConfigDest])
	unitTemplate = strings.TrimSpace(contents[agentSystemdUnitDest])
	if envTemplate == "" || unitTemplate == "" {
		return "", "", errors.New("agent_install.yml 缺少 config.env 或 dj-agent.service 的 copy content 任务，拒绝执行 Agent 安装/更新")
	}
	return envTemplate, unitTemplate, nil
}

// renderAgentEnvTemplate 渲染模板中的 dj_agent_id / dj_agent_grpc_addr 变量。
func renderAgentEnvTemplate(envTemplate, agentID, grpcAddr string) string {
	rendered := agentVarAddrPattern.ReplaceAllString(envTemplate, grpcAddr)
	rendered = agentVarIDPattern.ReplaceAllString(rendered, agentID)
	return rendered + "\n"
}

// collectCopyContents 深度遍历 playbook 结构，收集所有 copy 模块任务的 dest→content。
func collectCopyContents(node any, contents map[string]string) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "ansible.builtin.copy" || key == "copy" {
				if task, ok := child.(map[string]any); ok {
					destination, _ := task["dest"].(string)
					content, _ := task["content"].(string)
					if destination != "" && content != "" {
						contents[destination] = content
					}
				}
			}
			collectCopyContents(child, contents)
		}
	case []any:
		for _, child := range value {
			collectCopyContents(child, contents)
		}
	}
}
