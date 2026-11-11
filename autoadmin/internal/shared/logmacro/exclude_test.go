package logmacro

import (
	"regexp"
	"testing"
)

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
