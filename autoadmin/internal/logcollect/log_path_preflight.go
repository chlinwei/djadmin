package logcollect

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"

	"gopkg.in/yaml.v3"
)

type logPathCheckReceipt struct {
	Version           int      `json:"version"`
	Checked           bool     `json:"checked"`
	FilesDigest       string   `json:"files_digest"`
	InputsChecked     int      `json:"inputs_checked"`
	MatchedFiles      int      `json:"matched_files"`
	UnmatchedPatterns []string `json:"unmatched_patterns"`
}

func validateLogPathReceipt(result *pb.AutomationExecuteResponse, fragments []logConfigFragment) (logPathCheckReceipt, error) {
	var envelope struct {
		PathCheck *logPathCheckReceipt `json:"path_check"`
	}
	if result == nil {
		return logPathCheckReceipt{}, fmt.Errorf("Agent 未返回日志文件检查结果")
	}
	if result.Status != "success" || result.ExitCode != 0 {
		return logPathCheckReceipt{}, fmt.Errorf("%s", firstNonEmpty(strings.TrimSpace(result.ErrorMessage),
			strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout), "Agent 未成功完成日志文件检查"))
	}
	if err := json.Unmarshal([]byte(result.ResultDataJson), &envelope); err != nil {
		return logPathCheckReceipt{}, fmt.Errorf("Agent 日志文件检查回执无效，请升级 Agent 后重试: %w", err)
	}
	raw, err := json.Marshal(fragments)
	if err != nil {
		return logPathCheckReceipt{}, err
	}
	expectedInputs := 0
	for _, fragment := range fragments {
		var inputs []struct {
			Enabled *bool `yaml:"enabled"`
		}
		if err := yaml.Unmarshal([]byte(fragment.Content), &inputs); err != nil {
			return logPathCheckReceipt{}, fmt.Errorf("读取待检查 Filebeat input 失败: %w", err)
		}
		for _, input := range inputs {
			if input.Enabled == nil || *input.Enabled {
				expectedInputs++
			}
		}
	}
	receipt := envelope.PathCheck
	if receipt == nil || receipt.Version != 1 || !receipt.Checked ||
		receipt.FilesDigest != fmt.Sprintf("%x", sha256.Sum256(raw)) ||
		receipt.InputsChecked != expectedInputs || receipt.MatchedFiles < 0 ||
		receipt.UnmatchedPatterns == nil || len(receipt.UnmatchedPatterns) > receipt.InputsChecked {
		return logPathCheckReceipt{}, fmt.Errorf("Agent 日志文件检查回执缺失或与本次配置不符，请升级 Agent 后重试")
	}
	return *receipt, nil
}

func (handler *Handler) checkLogTargetPaths(ctx context.Context, row logTargetRow, fragments []logConfigFragment) (logPathCheckReceipt, error) {
	// 网关只等待调用方 context；动作超时与等待超时同时约束，避免离线/旧 Agent 卡住下发。
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	params, err := json.Marshal(struct {
		Files []logConfigFragment `json:"files"`
	}{Files: fragments})
	if err != nil {
		return logPathCheckReceipt{}, err
	}
	result, err := handler.gateway.Execute(checkCtx, row.HostName, &pb.AutomationExecuteRequest{
		JobId: fmt.Sprintf("filebeat-path-check-%d", time.Now().UnixNano()), Type: "custom",
		Action: "check_filebeat_paths", ParamsJson: string(params), TimeoutSeconds: 30,
	})
	if err == nil {
		return validateLogPathReceipt(result, fragments)
	}
	return logPathCheckReceipt{}, err
}

func logPathCheckWarnings(receipt logPathCheckReceipt) []string {
	if len(receipt.UnmatchedPatterns) == 0 {
		return nil
	}
	return []string{"以下日志路径当前未匹配到文件，仅检查现有文件；未来新增文件或链接变化后需重新检查：" +
		strings.Join(receipt.UnmatchedPatterns, "、")}
}
