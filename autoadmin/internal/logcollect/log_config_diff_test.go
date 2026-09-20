package logcollect

import (
	"testing"
)

// 采集配置差异的判定（架构文档 §8.8 的补充能力）：期望片段 vs 主机上已下发的片段。
//
// 这是整个功能里唯一"会算错"的地方——把"要删"算成"要加"，用户就会以为下发只会新增文件，
// 而 agent 侧的实际语义是**全量替换**（没交付的 .yml 会被删掉）。所以四种状态逐一钉住。
//
// 读不准的情况（读失败 / 内容被截断）不参与"一致/不一致"判定，另计 unread：
// 既不说"一致"（可能其实改了），也不说"不一致"（可能其实没改）。

func fragment(path, content string) logConfigFragment {
	return logConfigFragment{Path: path, Content: content}
}

func deployed(path, content string) deployedLogConfigFragment {
	return deployedLogConfigFragment{Path: path, Content: content}
}

func diffByPath(files []logConfigDiffFile) map[string]logConfigDiffFile {
	result := make(map[string]logConfigDiffFile, len(files))
	for _, file := range files {
		result[file.Path] = file
	}
	return result
}

func TestDiffLogConfigFragmentsClassifiesEveryChange(t *testing.T) {
	expected := []logConfigFragment{
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__app.log.yml", "same"),
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml", "new-content"),
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__access.log.yml", "will-be-added"),
	}
	deployed := []deployedLogConfigFragment{
		deployed("/etc/filebeat/inputs.d/tomcat__order__15__app.log.yml", "same"),
		deployed("/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml", "old-content"),
		// 期望侧没有它 → 下发时会被删掉（agent 侧是全量替换语义）。
		deployed("/etc/filebeat/inputs.d/tomcat__order__15__gc.log.yml", "stale"),
	}

	files, summary := diffLogConfigFragments(expected, deployed)
	byPath := diffByPath(files)

	if byPath["/etc/filebeat/inputs.d/tomcat__order__15__app.log.yml"].Status != logConfigDiffUnchanged {
		t.Fatalf("内容一致应判未变：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order__15__app.log.yml"])
	}
	if byPath["/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml"].Status != logConfigDiffChanged {
		t.Fatalf("内容不同应判修改：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml"])
	}
	if byPath["/etc/filebeat/inputs.d/tomcat__order__15__access.log.yml"].Status != logConfigDiffAdded {
		t.Fatalf("期望有、主机没有应判新增：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order__15__access.log.yml"])
	}
	removed := byPath["/etc/filebeat/inputs.d/tomcat__order__15__gc.log.yml"]
	if removed.Status != logConfigDiffRemoved || removed.Expected != nil || removed.Applied == nil {
		t.Fatalf("主机上有、期望没有应判删除（且期望侧内容为 null）：%+v", removed)
	}
	if summary.Added != 1 || summary.Removed != 1 || summary.Changed != 1 || summary.Unchanged != 1 || summary.Unread != 0 {
		t.Fatalf("摘要不对：%+v", summary)
	}
	// 两侧内容都要原样给出：界面自己做行级 diff，后端不该只回状态。
	if byPath["/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml"].Applied == nil ||
		*byPath["/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml"].Applied != "old-content" {
		t.Fatalf("已下发内容应原样返回：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order__15__error.log.yml"])
	}
}

// 每个片段带出归属服务（从文件名解析）：界面默认"只看本服务"，并把同主机其他服务的片段折叠。
func TestDiffLogConfigFragmentsCarriesOwningService(t *testing.T) {
	expected := []logConfigFragment{
		fragment("/etc/filebeat/inputs.d/tomcat__order-api__15__app.log.yml", "a"),
		fragment("/etc/filebeat/inputs.d/tomcat__pay-api__16__app.log.yml", "b"),
	}
	deployed := []deployedLogConfigFragment{
		deployed("/etc/filebeat/inputs.d/legacy__name-without-id.yml", "c"),
	}

	files, _ := diffLogConfigFragments(expected, deployed)
	byPath := diffByPath(files)

	if byPath["/etc/filebeat/inputs.d/tomcat__order-api__15__app.log.yml"].ServiceID != 15 {
		t.Fatalf("应从文件名解析出服务 id：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order-api__15__app.log.yml"])
	}
	if byPath["/etc/filebeat/inputs.d/tomcat__order-api__15__app.log.yml"].ServiceCode != "order-api" {
		t.Fatalf("应从文件名解析出服务编码：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order-api__15__app.log.yml"])
	}
	// 解析不出来的（历史命名/手工放进去的）归 0，界面标"无法归属"，不能瞎猜成某个服务。
	if byPath["/etc/filebeat/inputs.d/legacy__name-without-id.yml"].ServiceID != 0 {
		t.Fatalf("解析不出服务 id 时应为 0：%+v", byPath["/etc/filebeat/inputs.d/legacy__name-without-id.yml"])
	}
}

func TestDiffLogConfigFragmentsTreatsUnreadableAsUnknown(t *testing.T) {
	expected := []logConfigFragment{
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__a.yml", "expected"),
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__b.yml", "expected"),
	}
	deployed := []deployedLogConfigFragment{
		{Path: "/etc/filebeat/inputs.d/tomcat__order__15__a.yml", ReadError: "读取失败：permission denied"},
		{Path: "/etc/filebeat/inputs.d/tomcat__order__15__b.yml", Content: "truncated", Truncated: true},
	}

	files, summary := diffLogConfigFragments(expected, deployed)
	byPath := diffByPath(files)

	for _, path := range []string{
		"/etc/filebeat/inputs.d/tomcat__order__15__a.yml",
		"/etc/filebeat/inputs.d/tomcat__order__15__b.yml",
	} {
		if byPath[path].Status == logConfigDiffUnchanged {
			t.Fatalf("读不到/被截断时不能判成「一致」：%+v", byPath[path])
		}
	}
	if byPath["/etc/filebeat/inputs.d/tomcat__order__15__a.yml"].ReadError == "" {
		t.Fatalf("读失败的原因要带上：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order__15__a.yml"])
	}
	if !byPath["/etc/filebeat/inputs.d/tomcat__order__15__b.yml"].AppliedTruncated {
		t.Fatalf("截断要标注：%+v", byPath["/etc/filebeat/inputs.d/tomcat__order__15__b.yml"])
	}
	if summary.Unread != 2 || summary.Unchanged != 0 || summary.Changed != 0 {
		t.Fatalf("读不准的两个文件应只计 unread：%+v", summary)
	}
}

// 主机上一片空白（刚纳管、还没下发过）：期望侧全部判"新增"，这是 never 状态下的正常差异。
func TestDiffLogConfigFragmentsOnEmptyHost(t *testing.T) {
	expected := []logConfigFragment{
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__a.yml", "expected"),
		fragment("/etc/filebeat/inputs.d/tomcat__order__15__b.yml", "expected"),
	}

	files, summary := diffLogConfigFragments(expected, nil)

	if len(files) != 2 || summary.Added != 2 || summary.Removed != 0 {
		t.Fatalf("空主机应全部判新增：%+v / %+v", files, summary)
	}
	for _, file := range files {
		if file.Applied != nil {
			t.Fatalf("主机上没有的文件，已下发侧应为 null：%+v", file)
		}
	}
}

// 片段基名白名单：读回的主机文件名要挡住路径分隔符与上跳段（读回来的名字是不可信输入）。
func TestSafeConfigBaseName(t *testing.T) {
	valid := []string{"tomcat__order__15__app.log", "a__b__1__x", "with-dash__s__2__l.1"}
	for _, name := range valid {
		if !safeConfigBaseName(name) {
			t.Fatalf("%q 应被接受", name)
		}
	}
	invalid := []string{"", "..", "../../etc/passwd", "a/b", `a\b`, "a__b__1__x.yml.bak\n", "含中文"}
	for _, name := range invalid {
		if safeConfigBaseName(name) {
			t.Fatalf("%q 不该被接受", name)
		}
	}
}
