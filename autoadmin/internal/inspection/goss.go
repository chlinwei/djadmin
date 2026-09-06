package inspection

import (
	"embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// goss 官方 JSON Schema（上游 docs/schema.yaml，随 goss v0.4.10 语义）。
// 用于保存巡检组时预校验 goss YAML，Agent 端无需重复校验。
//
//go:embed gossschema/goss-schema.yaml
var gossSchemaFS embed.FS

// validateGossSpec 把 goss YAML 转成 JSON 语义后按官方 schema 校验，返回人类可读的首个错误。
func validateGossSpec(spec string) error {
	var document any
	if err := yaml.Unmarshal([]byte(spec), &document); err != nil {
		return fmt.Errorf("goss YAML 解析失败: %v", err)
	}
	// 裸数字键（port: 61616:）会被 yaml 解析成非 string 键的 map，JSON schema 要求
	// 对象键必须是字符串；goss 引擎本身接受这种写法（键按 fmt.Sprint 归一），
	// 校验器保持同一行为，归一化后再校验。
	document = normalizeYAMLKeys(document)
	schemaBytes, err := gossSchemaFS.ReadFile("gossschema/goss-schema.yaml")
	if err != nil {
		return fmt.Errorf("读取 goss schema 失败: %v", err)
	}
	var schemaDocument any
	if err = yaml.Unmarshal(schemaBytes, &schemaDocument); err != nil {
		return fmt.Errorf("goss schema 解析失败: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	// schema 的 $id 是 https://goss.rocks/schema.yaml，按该 URL 注册后编译同 URL。
	schemaURL := "https://goss.rocks/schema.yaml"
	if err = compiler.AddResource(schemaURL, schemaDocument); err != nil {
		return fmt.Errorf("goss schema 注册失败: %v", err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return fmt.Errorf("goss schema 编译失败: %v", err)
	}
	if err = schema.Validate(document); err != nil {
		return fmt.Errorf("goss YAML 不符合官方 schema: %v", err)
	}
	return nil
}

// normalizeYAMLKeys 递归把 YAML 文档里非 string 的 map 键转成字符串
// （goss 引擎运行时的同款行为），使 JSON schema 校验与引擎语义一致。
func normalizeYAMLKeys(value any) any {
	switch typed := value.(type) {
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[fmt.Sprint(key)] = normalizeYAMLKeys(item)
		}
		return normalized
	case map[string]any:
		for key, item := range typed {
			typed[key] = normalizeYAMLKeys(item)
		}
		return typed
	case []any:
		for index, item := range typed {
			typed[index] = normalizeYAMLKeys(item)
		}
		return typed
	default:
		return value
	}
}
