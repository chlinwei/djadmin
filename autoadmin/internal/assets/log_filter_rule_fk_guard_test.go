package assets

import (
	"strings"
	"testing"
)

// 守卫：服务级采集过滤的两列**不能**挂外键。
//
// 为什么值得守卫：这两列是**三态**——NULL 继承模板 / 0 显式关闭该方向 / >0 指定规则
// （见 db/schema 的列注释与 logcollect/log_collection_filter.go 的 resolveLogFilter）。
// "关闭"用 0 而不是 NULL，是因为 NULL 已经被"继承模板"占用；一旦这一列挂上外键，
// 0 就会被当成"指向 id=0 的规则"，保存直接外键失败并被 translate() 翻成
// **「关联资产不存在」**（2026-09-19 现场：日志中心把"继承模板 white-list"改成"不过滤"即触发，
// 连错误信息都指不到过滤上）。真库上 include 列曾有 Django 时代的外键，已由迁移 000040 摘掉；
// 折叠快照（db/schema）本来就没有它 —— 这里把"快照里也不许出现"钉住，防止有人"补全"外键。
//
// 引用完整性不靠外键：规则被删/停用/方向不符由渲染侧降级成"该方向不过滤 + 下发告警"，
// 这与平台"宁可多采不可不采"的取舍一致（见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §6.1）。
func TestServiceFilterColumnsHaveNoForeignKey(t *testing.T) {
	for _, path := range []string{
		"db/schema/mysql/002_assets.sql",
		"db/schema/postgres/002_assets.sql",
	} {
		content, err := readRepositoryFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		block := createTableBlock(t, path, content, "assets_application_service_log_setting")
		for _, column := range []string{"collection_filter_rule_id", "collection_exclude_filter_rule_id"} {
			if !strings.Contains(block, column) {
				t.Fatalf("%s 的 assets_application_service_log_setting 里找不到 %s 列：\n%s", path, column, block)
			}
		}
		// 逐行看：外键声明行里不许出现过滤列（列定义行可以自由出现这两个名字）。
		for _, line := range strings.Split(block, "\n") {
			if !strings.Contains(line, "FOREIGN KEY") {
				continue
			}
			if strings.Contains(line, "filter_rule_id") {
				t.Fatalf("%s 给采集过滤列加了外键，0（显式关闭）将无法落库：\n%s", path, line)
			}
		}
	}
}

// 守卫：迁移 000040 必须在两个方言里都摘掉这一列上的外键（真库有、折叠快照建的库没有，
// 所以两边都写"现查现删"）。删掉迁移文件或退回硬编码的 DROP FOREIGN KEY 都会在这里失败。
func TestLogFilterRuleForeignKeyDropMigrationExists(t *testing.T) {
	for _, path := range []string{
		"db/migrations/mysql/000040_log_filter_rule_explicit_off.up.sql",
		"db/migrations/postgres/000040_log_filter_rule_explicit_off.up.sql",
	} {
		content, err := readRepositoryFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(content, "collection_filter_rule_id") {
			t.Fatalf("%s 没有提到要摘外键的列", path)
		}
		if !strings.Contains(content, "assets_application_service_log_setting") {
			t.Fatalf("%s 没有提到要改的表", path)
		}
	}
}

// createTableBlock 截出某个 CREATE TABLE 的整段 DDL（到第一个行首 `);` 为止）。
func createTableBlock(t *testing.T, path, content, table string) string {
	t.Helper()
	start := strings.Index(content, "CREATE TABLE "+table)
	if start < 0 {
		// 反引号写法（MySQL 侧的折叠快照）。
		start = strings.Index(content, "CREATE TABLE `"+table+"`")
	}
	if start < 0 {
		t.Fatalf("%s 里找不到 %s 的建表语句", path, table)
	}
	rest := content[start:]
	end := strings.Index(rest, "\n);")
	if end < 0 {
		t.Fatalf("%s 的 %s 建表语句没有结尾 `);`", path, table)
	}
	return rest[:end]
}
