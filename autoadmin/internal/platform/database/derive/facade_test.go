package derive

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFacadeMatchesRepository 断言两个门面文件 == 现场生成的结果。
//
// 门面是生成的产物：改了 schema / 查询 → `make generate` 后会多出或少掉导出类型，
// 门面必须跟着重建。否则 PG 侧会缺别名（-tags postgres 构建失败）或留下已删类型的别名。
func TestFacadeMatchesRepository(t *testing.T) {
	generatedDir, mysqlGenDir, postgresGenDir := facadeDirs(t)
	files, err := FacadeFiles(mysqlGenDir, postgresGenDir)
	if err != nil {
		t.Fatalf("生成门面失败：%v", err)
	}
	if *update {
		if err := WriteFacade(generatedDir, mysqlGenDir, postgresGenDir); err != nil {
			t.Fatalf("写出门面失败：%v", err)
		}
		t.Logf("已写出 %d 个门面文件到 %s", len(files), generatedDir)
		return
	}
	for _, file := range files {
		onDisk, err := os.ReadFile(filepath.Join(generatedDir, file.Name))
		if err != nil {
			t.Errorf("%s 缺失或不可读（跑 make facade 生成）：%v", file.Name, err)
			continue
		}
		if string(onDisk) != file.Content {
			t.Errorf("%s 与生成结果不一致（门面是产物，请跑 make facade）：\n%s",
				file.Name, firstDifference(string(onDisk), file.Content))
		}
	}
}

// TestFacadeAdapterTypesDeclared 保证「排除集」与手写适配文件不脱节：
// 被排除出别名的每个类型，都必须在 dialect_postgres_adapters.go 里真的声明，
// 否则 PG 侧会报 undefined。
func TestFacadeAdapterTypesDeclared(t *testing.T) {
	generatedDir, _, _ := facadeDirs(t)
	raw, err := os.ReadFile(filepath.Join(generatedDir, "dialect_postgres_adapters.go"))
	if err != nil {
		t.Fatalf("读取适配文件失败：%v", err)
	}
	source := string(raw)
	// Queries / DBTX 由 dialect_postgres.go 自己声明，不在适配文件里
	skip := map[string]bool{"Queries": true, "DBTX": true}
	var missing []string
	for name := range facadeAdapterTypes {
		if skip[name] {
			continue
		}
		if !strings.Contains(source, "type "+name+" struct") {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("以下类型被排除出 PG 别名，但 dialect_postgres_adapters.go 里没有声明：%v", missing)
	}
}

func facadeDirs(t *testing.T) (string, string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件位置")
	}
	platformDir := filepath.Join(filepath.Dir(file), "..")
	generatedDir := filepath.Join(platformDir, "generated")
	return generatedDir, filepath.Join(generatedDir, "mysql"), filepath.Join(generatedDir, "postgres")
}
