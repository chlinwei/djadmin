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
