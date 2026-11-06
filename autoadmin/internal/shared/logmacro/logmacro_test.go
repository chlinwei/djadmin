package logmacro

import "testing"

func TestResolveCheckedDependencies(t *testing.T) {
	values := map[string]string{
		"APP_HOME": "/home/${USER}/app", "USER": "nkg_imf",
		"FILE": "${NAME}.xml", "NAME": "server", "PORT": "${HTTP_PORT}", "HTTP_PORT": "8080",
	}
	for expression, want := range map[string]string{
		"${APP_HOME}/conf/${FILE}": "/home/nkg_imf/app/conf/server.xml",
		"${PORT}":                  "8080",
	} {
		got, err := ResolveChecked(expression, values)
		if err != nil || got != want {
			t.Fatalf("resolve %q: got %q err=%v, want %q", expression, got, err, want)
		}
	}
	for _, values := range []map[string]string{
		{"APP_HOME": "${MISSING}"},
		{"APP_HOME": "${OTHER}", "OTHER": "${APP_HOME}"},
	} {
		if got, err := ResolveChecked("${APP_HOME}", values); err == nil || got != "${APP_HOME}" {
			t.Fatalf("invalid dependency must remain unresolved: got %q err=%v", got, err)
		}
	}
}

func TestResolveNestedMacros(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		got := Resolve("${APP_HOME}/logs/errorLog.log",
			map[string]string{"IMF_USER": "default"},
			InstanceValues("", "/home/${IMF_USER}/apps/adapter1"),
			map[string]string{"IMF_USER": "nkg_imf"})
		if got != "/home/nkg_imf/apps/adapter1/logs/errorLog.log" {
			t.Fatalf("nested path not resolved: %s", got)
		}
	}
}

func TestResolveRejectsEmptyDependency(t *testing.T) {
	for _, value := range []string{"", "   "} {
		got, err := ResolveChecked("${APP_HOME}/logs/error.log", map[string]string{
			"APP_HOME": "/home/${IMF_USER}/apps/adapter1", "IMF_USER": value,
		})
		if got != "${APP_HOME}/logs/error.log" || err == nil || err.Error() != "宏 IMF_USER 值为空" {
			t.Fatalf("empty dependency must remain unresolved: got %q err=%v", got, err)
		}
	}
}

func TestResolveKeepsUndefinedAndCyclicMacrosBounded(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"undefined", map[string]string{"APP_HOME": "/home/${IMF_USER}"}, "${APP_HOME}"},
		{"self", map[string]string{"APP_HOME": "${APP_HOME}"}, "${APP_HOME}"},
		{"cycle", map[string]string{"APP_HOME": "${OTHER}", "OTHER": "${APP_HOME}"}, "${APP_HOME}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := Resolve("${APP_HOME}", test.values); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
