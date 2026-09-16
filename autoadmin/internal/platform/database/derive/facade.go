package derive

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// 本文件生成**方言门面**（internal/platform/database/generated/dialect_*.go）。
//
// 门面的作用：应用代码只 import internal/platform/database/generated 一个包，
// 由 build tag 决定背后是 MySQL 还是 PostgreSQL 的 sqlc 产物——
// 默认（不带 tag）是 MySQL，`-tags postgres` 是 PostgreSQL。
//
// 门面里只有类型别名与 New 转发（MySQL 侧）和「把 PG 实现包成 Queries」（PG 侧）。
// 两侧签名不一致的少数查询（实测见 SQL_DESIGN §4.6.1）由**手写**的
// dialect_postgres_adapters.go 适配，因此那些类型不参与别名生成——
// facadeAdapterTypes 与那个文件必须保持一致，facade_test.go 会检查。

// facadeAdapterTypes 是 PostgreSQL 门面里手工适配的类型名。
var facadeAdapterTypes = map[string]bool{
	"Queries":                   true,                                   // PG 侧是包装结构体（嵌入 + 覆盖分歧方法）
	"DBTX":                      true,                                   // 由门面自己声明
	"CountAlertHistoriesParams": true, "ListAlertHistoriesParams": true, // LabelKey/LabelValue 字段类型不同
	"CountDeploymentTemplatesParams": true, "ListDeploymentTemplatesParams": true, // Column3 ↔ Column1
	"ListProjectsRow": true, // 聚合列（string_agg）类型不同
	// 以下 10 个在 PG 产物里没有结构体（单 pattern 入参被展开），由适配层补回
	"CountApplicationsParams": true, "CountBaselinesParams": true,
	"CountBusinessEnvironmentsParams": true, "CountBusinessSystemsParams": true,
	"CountCredentialsParams": true, "CountHostGroupsParams": true,
	"CountInspectionGroupsParams": true, "CountInspectionTasksParams": true,
	"CountInventoriesParams": true, "CountProjectsParams": true,
}

var facadeTypePattern = regexp.MustCompile(`(?m)^type (\w+) (?:struct|interface)`)

// FacadeFiles 生成两个门面文件（dialect_mysql.go / dialect_postgres.go）。
func FacadeFiles(mysqlGenDir, postgresGenDir string) ([]File, error) {
	mysqlTypes, err := exportedTypes(mysqlGenDir)
	if err != nil {
		return nil, err
	}
	postgresTypes, err := exportedTypes(postgresGenDir)
	if err != nil {
		return nil, err
	}
	var postgresAliases []string
	for _, name := range postgresTypes {
		if !facadeAdapterTypes[name] {
			postgresAliases = append(postgresAliases, name)
		}
	}
	mysqlContent := facadeHeader(false) + mysqlPreamble + aliasBlock("mysql", mysqlTypes)
	postgresContent := facadeHeader(true) + postgresPreamble + aliasBlock("postgres", postgresAliases)
	return []File{
		{Name: "dialect_mysql.go", Content: mysqlContent},
		{Name: "dialect_postgres.go", Content: postgresContent},
	}, nil
}

// WriteFacade 把门面写进 generated 目录。
func WriteFacade(generatedDir, mysqlGenDir, postgresGenDir string) error {
	files, err := FacadeFiles(mysqlGenDir, postgresGenDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(generatedDir, file.Name), []byte(file.Content), 0o664); err != nil {
			return fmt.Errorf("write facade %s: %w", file.Name, err)
		}
	}
	return nil
}

func exportedTypes(dir string) ([]string, error) {
	names, err := goFileNames(dir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var types []string
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		for _, match := range facadeTypePattern.FindAllStringSubmatch(string(raw), -1) {
			if !seen[match[1]] {
				seen[match[1]] = true
				types = append(types, match[1])
			}
		}
	}
	sort.Strings(types)
	return types, nil
}

func goFileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func facadeHeader(postgres bool) string {
	tag, description, packageName := "!postgres", "MySQL", "mysql"
	if postgres {
		tag, description, packageName = "postgres", "PostgreSQL", "postgres"
	}
	return fmt.Sprintf(`//go:build %s

// Package db 是数据访问层的**方言门面**：应用代码统一 import 本包，具体实现由构建标签选择——
// 默认（不带 tag）是 MySQL 产物，`+"`-tags postgres`"+` 是 PostgreSQL 产物。
//
// 本文件由 make facade 生成（实现见 internal/platform/database/derive/facade.go），勿手改。
// 门面只做类型别名与构造转发，不承载业务逻辑；两侧签名不一致的少数查询由
// dialect_postgres_adapters.go 手工适配。见 docs/architecture/SQL_DESIGN.md §4.6。
//
// 本文件对应方言：%s。
package db

import (
	%s "autoadmin/internal/platform/database/generated/%s"
)

`, tag, description, packageName, packageName)
}

const mysqlPreamble = `// New 转发到 MySQL 实现；MySQL 侧签名与产物完全一致，不需要包装结构体。
var New = mysql.New

`

const postgresPreamble = `// DBTX 与 New 对齐 MySQL 侧：应用代码拿到的始终是 *db.Queries。
type DBTX = postgres.DBTX

// Queries 包装 PG 实现：未分歧的方法由嵌入字段提升，分歧的查询由 adapters 文件覆盖。
type Queries struct{ *postgres.Queries }

func New(dbtx DBTX) *Queries { return &Queries{Queries: postgres.New(dbtx)} }

`

func aliasBlock(packageName string, types []string) string {
	var builder strings.Builder
	builder.WriteString("// 以下为当前方言产物的全部导出类型别名（PG 侧已剔除需要手工适配的类型）。\n")
	for _, name := range types {
		fmt.Fprintf(&builder, "type %s = %s.%s\n", name, packageName, name)
	}
	return builder.String()
}
