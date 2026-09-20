package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// `DROP CHECK` 后面直接跟标识符字符 = 把约束名写死在语句里（动态拼法是 `DROP CHECK `', @var, '“，
// 紧跟的是单引号）。变量名如 `@retention_chk` 不算 —— 那种 `_chk` 是拼出来的，不是查出来的。
var hardcodedConstraintName = regexp.MustCompile("DROP CHECK `[A-Za-z0-9_]")

// 守卫：迁移 000045 必须在改名之前**按列现查现删**掉 Django 生成的 CHECK 约束。
//
// 为什么值得守卫（2026-09-20 现场）：`monitor_log_retention_tier` 是 Django 时代建的表，
// Django 4.1 会给每个 PositiveIntegerField 自动加一条 CHECK —— 真库上是
// `CONSTRAINT monitor_log_retention_tier_chk_1 CHECK ((retention_days >= 0))`。
// **MySQL 不允许重命名被 CHECK 引用的列**，于是"把 retention_days 改名成 retention_value"
// 这一句直接报 `Error 3959: Check constraint '...' uses column 'retention_days', hence column
// cannot be dropped or renamed`，整个迁移卡在版本 45 上。
//
// 两条要求缺一不可，丢了任何一条都会退回失败现场：
//   - 删约束这件事**必须在改名之前**发生（顺序）；
//   - 删的是**按列现查**出来的约束（`<表>_chk_<n>` 是 Django 生成名），不能写死名字：
//     按折叠快照建的库上根本没有这个约束，写死名字会报 "check that column/key exists"。
//     折叠快照（db/schema/*/003）里没有它，所以"没有 → 跳过"这一路必须存在（`SELECT 1` 空操作）。
//
// PG 侧不需要这一步（PG 允许重命名被 CHECK 引用的列），但按 README 的约定，
// 语句级差异要在 PG 文件顶部写成 `-- PG 侧差异：` 注释 —— 这里一并钉住，
// 免得有人"为了对齐两侧"把 MySQL 的动态 SQL 也搬过去。
func TestLogRetentionUnitMigrationDropsLegacyCheckConstraint(t *testing.T) {
	const (
		mysqlUp = "db/migrations/mysql/000045_log_retention_unit.up.sql"
		pgUp    = "db/migrations/postgres/000045_log_retention_unit.up.sql"
	)

	content := readRepoFile(t, mysqlUp)
	// 只看代码：注释里本来就会写约束名和报错原文，不能当成"写死了名字"。
	code := sqlCodeLines(content)

	for _, want := range []string{
		"information_schema.CHECK_CONSTRAINTS",
		"tc.CONSTRAINT_TYPE = 'CHECK'",
		"CHECK_CLAUSE LIKE '%retention_days%'",
		"CONCAT('ALTER TABLE `monitor_log_retention_tier` DROP CHECK",
		"PREPARE",
		"EXECUTE",
		"DEALLOCATE PREPARE",
		"'SELECT 1'", // 没有这个约束的库（折叠快照建的）走空操作
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("%s 缺少动态摘约束所需的一环：%q\n折叠快照库上没有该约束，必须现查现删且允许查不到。", mysqlUp, want)
		}
	}
	if hardcodedConstraintName.MatchString(code) {
		t.Fatalf("%s 把 Django 生成的约束名写死进了 DROP CHECK：按折叠快照建的库上没有这个约束，"+
			"写死名字会让迁移在那类库上失败。要按 information_schema 现查、用变量拼语句。%s",
			mysqlUp, hardcodedConstraintName.FindString(code))
	}

	// 顺序：摘 CHECK → 改名 → 加单位列。
	drop := strings.Index(code, "DROP CHECK")
	rename := strings.Index(code, "CHANGE COLUMN `retention_days` `retention_value`")
	addUnit := strings.Index(code, "ADD COLUMN `retention_unit`")
	if drop < 0 || rename < 0 || addUnit < 0 {
		t.Fatalf("%s 缺少三步之一：drop=%d rename=%d addUnit=%d", mysqlUp, drop, rename, addUnit)
	}
	if drop > rename {
		t.Fatalf("%s 的摘 CHECK 排在改名之后：MySQL 会以 Error 3959 拒绝改名（2026-09-20 现场）。", mysqlUp)
	}
	if rename > addUnit {
		t.Fatalf("%s 的加单位列排在改名之前，逻辑顺序不对。", mysqlUp)
	}

	pg := readRepoFile(t, pgUp)
	if !strings.Contains(pg, "RENAME COLUMN retention_days TO retention_value") {
		t.Fatalf("%s 没有把值列改名", pgUp)
	}
	if !strings.Contains(pg, "-- PG 侧差异：") {
		t.Fatalf("%s 与 MySQL 侧有语句级差异（不需要摘 CHECK），按 README 约定要写 `-- PG 侧差异：` 注释", pgUp)
	}
	if strings.Contains(sqlCodeLines(pg), "DROP CHECK") {
		t.Fatalf("%s 抄了 MySQL 侧的摘约束动作：PG 允许重命名被 CHECK 引用的列，那一列在 PG 上还是有符号整数，"+
			"`>= 0` 是真校验，不该摘。", pgUp)
	}
}

// 守卫：折叠快照里的保留档位表**不许出现** CHECK 约束。
//
// 为什么：真库上那条 `CHECK (retention_days >= 0)` 是 Django 自动生成的，而这一列是
// `int unsigned` —— `>= 0` 恒真、没有任何校验价值；UP 摘掉它正是为了让真库与折叠快照收敛
// （同 assets 包对采集过滤外键的做法）。这里把"快照里也不许出现"钉住，防止有人照着真库
// "补全"这个约束，把两侧重新拉开。
func TestRetentionTierSnapshotHasNoLegacyCheckConstraint(t *testing.T) {
	for _, path := range []string{
		"db/schema/mysql/003_monitor_inspection.sql",
		"db/schema/postgres/003_monitor_inspection.sql",
	} {
		block := createTableBlock(t, path, readRepoFile(t, path), "monitor_log_retention_tier")
		if !strings.Contains(block, "retention_value") || !strings.Contains(block, "retention_unit") {
			t.Fatalf("%s 的 monitor_log_retention_tier 不是改名后的形态（应有 retention_value + retention_unit）：\n%s", path, block)
		}
		if strings.Contains(block, "CHECK") {
			t.Fatalf("%s 的 monitor_log_retention_tier 里出现了 CHECK 约束：真库上那条是 Django 生成、"+
				"在 unsigned 列上恒真的历史产物，迁移 000045 已摘掉，快照不应再出现：\n%s", path, block)
		}
	}
}

// sqlCodeLines 丢掉 `--` 注释行，只留可执行语句。
func sqlCodeLines(content string) string {
	var kept []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(raw)
}

// repoRoot 从测试文件位置回溯到 autoadmin（go.mod 所在目录）——迁移文件都在它下面。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("找不到 go.mod（仓库根）")
	return ""
}

// createTableBlock 截出某个 CREATE TABLE 的整段 DDL（到第一个行首 `);` 为止）。
func createTableBlock(t *testing.T, path, content, table string) string {
	t.Helper()
	start := strings.Index(content, "CREATE TABLE "+table)
	if start < 0 {
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
