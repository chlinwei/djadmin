package logmacro

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestExcludeFilePatterns(t *testing.T) {
	const pattern = `-(星期[一二三四五六日天]|Mon|Tue|Wed|Thu|Fri|Sat|Sun)\.log$`
	raw, err := json.Marshal([]string{pattern})
	if err != nil {
		t.Fatal(err)
	}
	patterns, err := ResolveExcludeJSON([]byte(`["${LOG_DIR}/backup"]`), raw, map[string]string{"LOG_DIR": "/logs"})
	if err != nil || len(patterns) != 2 || patterns[1] != pattern {
		t.Fatalf("directory and file expressions must be combined unchanged: %v, %v", patterns, err)
	}
	re := regexp.MustCompile(patterns[1])
	for _, name := range []string{
		"cat.log", "cat2.log", "HAK10.log", "HAK1_random123.log", "HAK_whatever_name.log",
		"HAK_other-2026.log", "HAK_星期四.log", "HAK-random.log", "backup-Thu.log/HAK.log",
	} {
		if re.MatchString("/logs/" + name) {
			t.Errorf("non-weekday filename must remain: %s", name)
		}
	}
	for _, day := range []string{"星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日", "星期天", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		if !re.MatchString("/logs/HAK_arbitrary_" + day + "-" + day + ".log") {
			t.Errorf("weekday suffix must be excluded: %s", day)
		}
	}
	for _, invalid := range []string{"", " ", "*-Thu.log", "[", "(?=Thu)", "a\nb", "a\rb"} {
		if err := ValidateExcludeFilePatterns([]string{invalid}); err == nil {
			t.Errorf("invalid RE2 expression accepted: %q", invalid)
		}
	}
	if _, err := ResolveExcludeJSON(nil, []byte(`{"pattern":"Thu"}`)); err == nil {
		t.Fatal("object accepted instead of a string array")
	}
	if err := ValidateExcludeDirectories([]string{pattern}); err == nil {
		t.Fatal("filename regex must not be accepted as a directory")
	}
}

func TestExcludeDirectories(t *testing.T) {
	patterns, err := ResolveExcludeDirectories([]string{"${LOG_DIR}/backup", "${LOG_DIR}/test-*", "/logs/app[12]/archive", "/logs/node?/old", "/logs/备份", "/logs/node[^12]/archive"}, map[string]string{"LOG_DIR": "/logs"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		want bool
	}{
		{"/logs/backup/error.log", true}, {"/logs/backup/deep/error.log", true},
		{"/logs/backups/error.log", false}, {"/logs/test-one/error.log", true},
		{"/logs/nested/test-one/error.log", false}, {"/logs/app2/archive/log", true},
		{"/logs/app3/archive/log", false}, {"/logs/node1/old/log", true},
		{"/logs/node12/old/log", false}, {"/other/logs/backup/log", false},
		{"/logs/备份/error.log", true}, {"/logs/备份旧/error.log", false},
		{"/logs/node3/archive/error.log", true}, {"/logs/node1/archive/error.log", false},
		{"/logs/node/x/archive/error.log", false},
	} {
		matched := false
		for _, pattern := range patterns {
			matched = matched || regexp.MustCompile(pattern).MatchString(test.path)
		}
		if matched != test.want {
			t.Errorf("path %s matched=%v want=%v", test.path, matched, test.want)
		}
	}
}

func TestExcludeDirectoriesRejectInvalidPatterns(t *testing.T) {
	for _, directory := range []string{"", "/", "relative/path", "/logs/**/old", "/logs/[", "/logs/../old", "/logs//old", "/logs/a\nb", "/logs/[a/b]"} {
		if err := ValidateExcludeDirectories([]string{directory}); err == nil {
			t.Errorf("accepted %q", directory)
		}
	}
	if err := ValidateExcludeDirectories([]string{"${LOG_DIR}/backup", "/logs/${APP}/old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExcludeDirectories([]string{"${MISSING}/backup"}); err == nil {
		t.Fatal("accepted unresolved macro")
	}
}
