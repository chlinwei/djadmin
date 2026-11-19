package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func rhel7Definition(t *testing.T, key string) rhel7PolicyDefinition {
	t.Helper()
	for _, rule := range rhel7PolicyDefinitions() {
		if rule.Key == key {
			return rule
		}
	}
	t.Fatalf("unknown policy %s", key)
	return rhel7PolicyDefinition{}
}

func assertRHEL7Policy(t *testing.T, key string, input map[string]any, want string) {
	t.Helper()
	rule := rhel7Definition(t, key)
	entries, defined, err := evalOpaQuery(context.Background(), opaAssertionsQuery, rule.Config["policy"].(string), input)
	if err != nil {
		if want == "error" {
			return
		}
		t.Fatal(err)
	}
	if !defined || len(entries) == 0 {
		t.Fatalf("%s silently lost its assertions", key)
	}
	status := "pass"
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("non-object assertion: %#v", raw)
		}
		pass, ok := entry["pass"].(bool)
		if !ok {
			t.Fatalf("non-boolean pass: %#v", raw)
		}
		if !pass {
			status = "fail"
		}
	}
	if status != want {
		t.Fatalf("%s status=%s want=%s assertions=%+v", key, status, want, entries)
	}
}

func TestRHEL7PasswordAgeAssertions(t *testing.T) {
	for _, test := range []struct{ name, content, accounts, want string }{
		{"90", "PASS_MAX_DAYS 90", "alice:set:90", "pass"},
		{"zero", "PASS_MAX_DAYS 0", "alice:set:0", "pass"},
		{"99999", "PASS_MAX_DAYS 99999", "alice:set:90", "fail"},
		{"91", "PASS_MAX_DAYS 91", "alice:set:90", "fail"},
		{"negative", "PASS_MAX_DAYS -1", "alice:set:90", "fail"},
		{"tabs", "PASS_MAX_DAYS\t90", "alice:set:90", "pass"},
		{"comment", " PASS_MAX_DAYS   90 # expiry limit", "alice:set:90", "pass"},
		{"comment-does-not-override", "PASS_MAX_DAYS 99999 # change to 90", "alice:set:90", "fail"},
		{"similar-key", "PASS_MAX_DAYS_BACKUP 90", "alice:set:90", "fail"},
		{"missing-setting", "# PASS_MAX_DAYS 90", "alice:set:90", "fail"},
		{"invalid-number", "PASS_MAX_DAYS invalid", "alice:set:90", "fail"},
		{"existing-account", "PASS_MAX_DAYS 90", "alice:set:99999", "fail"},
		{"existing-unlimited", "PASS_MAX_DAYS 90", "alice:set:-1", "fail"},
		{"empty-password", "PASS_MAX_DAYS 90", "alice:empty:90", "fail"},
		{"locked-password", "PASS_MAX_DAYS 90", "alice:locked:-1", "pass"},
		{"no-login-accounts", "PASS_MAX_DAYS 90", "", "pass"},
		{"conflicting-defaults", "PASS_MAX_DAYS 90\nPASS_MAX_DAYS 99999", "alice:set:90", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertRHEL7Policy(t, "password-max-age", map[string]any{
				"login_defs":   opaParsedOutput("command", test.content, 0, "lines"),
				"account_ages": opaParsedOutput("command", test.accounts, 0, "lines"),
			}, test.want)
		})
	}
}

type rhel7CollectorFixture struct {
	root string
}

func newRHEL7Fixture(t *testing.T) *rhel7CollectorFixture {
	t.Helper()
	fixture := &rhel7CollectorFixture{root: t.TempDir()}
	for path, content := range map[string]string{
		"/etc/login.defs":              "PASS_MAX_DAYS\t90\n",
		"/etc/passwd":                  "root:x:0:0:root:/root:/bin/bash\nalice:x:1000:1000:Alice:/home/alice:/bin/bash\n",
		"/etc/shadow":                  "root:!:1:0:99999:7:::\nalice:synthetic:1:0:90:7:::\n",
		"/etc/security/pwquality.conf": "minlen = 12\ndcredit = 0\nucredit = 0\nlcredit = 0\nocredit = 0\n",
		"/etc/pam.d/system-auth":       "password requisite pam_pwquality.so retry=3 enforce_for_root\npassword sufficient pam_unix.so sha512 shadow use_authtok\npassword required pam_deny.so\n",
		"/etc/pam.d/password-auth":     "password requisite pam_pwquality.so retry=3 enforce_for_root\npassword sufficient pam_unix.so sha512 shadow use_authtok\npassword required pam_deny.so\n",
		"/etc/pam.d/passwd":            "password substack system-auth\n",
		"/etc/pam.d/sshd":              "password include password-auth\n",
		"/etc/ssh/sshd_config":         "PermitRootLogin no\nPermitEmptyPasswords no\nMaxAuthTries 4\n",
	} {
		fixture.write(t, path, content)
	}
	for _, family := range []string{"ipv4", "ipv6"} {
		attributes := []string{"accept_redirects"}
		if family == "ipv4" {
			attributes = append(attributes, "send_redirects")
		}
		for _, target := range []string{"all", "default", "lo", "eth0"} {
			for _, attribute := range attributes {
				fixture.write(t, "/proc/sys/net/"+family+"/conf/"+target+"/"+attribute, "0\n")
			}
		}
	}
	fixture.binary(t, "systemctl", "printf '%s\\n' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=enabled'")
	fixture.binary(t, "sshd", `[ "$1" = "-T" ] && [ "$2" = "-f" ] || exit 91
printf '%s\n' 'permitrootlogin no' 'permitemptypasswords no' 'maxauthtries 4'`)
	return fixture
}

func (fixture *rhel7CollectorFixture) write(t *testing.T, path, content string) {
	t.Helper()
	resolved := filepath.Join(fixture.root, strings.TrimPrefix(path, "/"))
	if err := os.MkdirAll(filepath.Dir(resolved), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resolved, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (fixture *rhel7CollectorFixture) binary(t *testing.T, name, script string) {
	t.Helper()
	path := filepath.Join(fixture.root, "bin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func (fixture *rhel7CollectorFixture) run(t *testing.T, key, want string) applicationCheckResult {
	t.Helper()
	rule := rhel7Definition(t, key)
	rewrite := strings.NewReplacer(
		"/usr/sbin/sshd", filepath.Join(fixture.root, "bin", "sshd"),
		"/etc/", fixture.root+"/etc/",
		"/proc/sys/", fixture.root+"/proc/sys/",
	)
	for _, raw := range rule.Config["input_commands"].([]any) {
		entry := raw.(map[string]any)
		entry["exec"] = "PATH='" + fixture.root + "/bin:/usr/bin:/bin'; export PATH\n" + rewrite.Replace(entry["exec"].(string))
	}
	result := New(0).checkOpa(context.Background(), map[string]any{
		"key": key, "type": "opa", "name": rule.Name, "config": rule.Config, "run_user": "root",
	})
	if result.Status != want {
		t.Fatalf("%s status=%s want=%s message=%s actual=%+v", key, result.Status, want, result.Message, result.Actual)
	}
	if want != "error" {
		actual := result.Actual.(map[string]any)
		if len(actual["details"].([]map[string]any)) == 0 {
			t.Fatal("collector produced no decision assertions")
		}
		encoded, err := json.Marshal(actual)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "synthetic") {
			t.Fatal("password material leaked into stored evidence")
		}
	}
	return result
}

func TestRHEL7AccountAgeCollector(t *testing.T) {
	for _, test := range []struct{ name, passwd, shadow, want string }{
		{"normal", "alice:x:1000:1000:A:/home/a:/bin/bash\n", "alice:synthetic:1:0:90:7:::\n", "pass"},
		{"root-unlimited", "root:x:0:0:root:/root:/bin/bash\n", "root:synthetic:1:0:99999:7:::\n", "fail"},
		{"low-uid-login", "legacy:x:500:500:A:/home/a:/bin/bash\n", "legacy:synthetic:1:0:99999:7:::\n", "fail"},
		{"locked", "alice:x:1000:1000:A:/home/a:/bin/bash\n", "alice:!!:1:0:99999:7:::\n", "pass"},
		{"non-login", "svc:x:1001:1001:A:/srv:/sbin/nologin\n", "svc:synthetic:1:0:99999:7:::\n", "pass"},
		{"empty", "alice:x:1000:1000:A:/home/a:/bin/bash\n", "alice::1:0:90:7:::\n", "fail"},
		{"missing-shadow", "alice:x:1000:1000:A:/home/a:/bin/bash\n", "", "error"},
		{"duplicate-shadow", "alice:x:1000:1000:A:/home/a:/bin/bash\n", "alice:!:1:0:90:7:::\nalice:!:1:0:90:7:::\n", "error"},
		{"malformed-passwd", "alice:x:not-a-uid:1000:A:/home/a:/bin/bash\n", "alice:!:1:0:90:7:::\n", "error"},
		{"empty-passwd", "", "", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			fixture.write(t, "/etc/passwd", test.passwd)
			fixture.write(t, "/etc/shadow", test.shadow)
			fixture.run(t, "password-max-age", test.want)
		})
	}
}

func TestRHEL7PAMCollector(t *testing.T) {
	for _, test := range []struct{ name, path, content, want string }{
		{"normal", "", "", "pass"},
		{"tabs-comments", "/etc/security/pwquality.conf", " minlen\t=\t12 # length\n", "pass"},
		{"crlf-config", "/etc/security/pwquality.conf", "minlen=12\r\n", "pass"},
		{"too-short", "/etc/security/pwquality.conf", "minlen = 11\n", "fail"},
		{"credits-reduce-length", "/etc/security/pwquality.conf", "minlen=12\ndcredit=1\nucredit=1\nlcredit=1\nocredit=1\n", "fail"},
		{"credits-compensated", "/etc/security/pwquality.conf", "minlen=16\ndcredit=1\nucredit=1\nlcredit=1\nocredit=1\n", "pass"},
		{"large-credits-bound", "/etc/security/pwquality.conf", "minlen=24\ndcredit=10\nucredit=10\nlcredit=10\nocredit=10\n", "pass"},
		{"module-override-weaker", "/etc/pam.d/system-auth", "password requisite pam_pwquality.so enforce_for_root minlen=8\npassword sufficient pam_unix.so use_authtok\n", "fail"},
		{"root-exempt", "/etc/pam.d/system-auth", "password requisite pam_pwquality.so\npassword sufficient pam_unix.so use_authtok\n", "fail"},
		{"quality-warning-only", "/etc/security/pwquality.conf", "minlen=12\nenforcing=0\n", "fail"},
		{"module-warning-only", "/etc/pam.d/system-auth", "password requisite pam_pwquality.so enforce_for_root enforcing=0\npassword sufficient pam_unix.so use_authtok\n", "fail"},
		{"token-reprompt", "/etc/pam.d/system-auth", "password requisite pam_pwquality.so enforce_for_root\npassword sufficient pam_unix.so\n", "fail"},
		{"missing-quality-module", "/etc/pam.d/system-auth", "password sufficient pam_unix.so use_authtok\npassword required pam_deny.so\n", "fail"},
		{"optional-quality-module", "/etc/pam.d/system-auth", "password optional pam_pwquality.so enforce_for_root\npassword sufficient pam_unix.so use_authtok\n", "fail"},
		{"bypass-before-quality", "/etc/pam.d/system-auth", "password sufficient pam_unix.so use_authtok\npassword requisite pam_pwquality.so enforce_for_root\n", "error"},
		{"dash-type-bypass", "/etc/pam.d/system-auth", "-password sufficient pam_unix.so use_authtok\npassword requisite pam_pwquality.so enforce_for_root\n", "error"},
		{"non-system-module", "/etc/pam.d/system-auth", "password requisite /tmp/pam_pwquality.so enforce_for_root\npassword sufficient pam_unix.so use_authtok\n", "error"},
		{"alternate-quality-config", "/etc/pam.d/system-auth", "password requisite pam_pwquality.so enforce_for_root conf=/etc/security/custom.conf\npassword sufficient pam_unix.so use_authtok\n", "error"},
		{"include-unknown", "/etc/pam.d/system-auth", "password include custom-password\n", "error"},
		{"extended-control", "/etc/pam.d/system-auth", "password [success=1 default=ignore] pam_pwquality.so\n", "error"},
		{"quality-fragment", "/etc/security/pwquality.conf.d/custom.conf", "minlen=8\n", "error"},
		{"direct-stack-without-quality", "/etc/pam.d/passwd", "password sufficient pam_unix.so\n", "fail"},
		{"alternate-entry-profile", "/etc/pam.d/passwd", "password include password-auth\n", "pass"},
		{"nested-delegation", "/etc/pam.d/system-auth", "password substack password-auth\n", "pass"},
		{"delegation-cycle", "/etc/pam.d/system-auth", "password include passwd\n", "error"},
		{"sshd-entry-bypass", "/etc/pam.d/sshd", "password sufficient pam_unix.so\npassword include password-auth\n", "error"},
		{"invalid-number", "/etc/security/pwquality.conf", "minlen=not-number\n", "error"},
		{"empty-chain", "/etc/pam.d/system-auth", "# empty\n", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			if test.path != "" {
				fixture.write(t, test.path, test.content)
			}
			fixture.run(t, "password-min-length", test.want)
		})
	}
}

func TestPAMDebianAtInclude(t *testing.T) {
	const stack = "password [success=1 default=ignore] pam_unix.so obscure yescrypt\n" +
		"password requisite pam_deny.so\npassword required pam_permit.so\n"
	for _, test := range []struct{ name, commonAuth, commonPassword, want string }{
		{"missing-quality-gate", "auth required pam_unix.so\n", stack, "fail"},
		{"optional-keyring", "auth required pam_unix.so\n", stack+"password optional pam_gnome_keyring.so\n", "fail"},
		{"unknown-trailing-module", "auth required pam_unix.so\n", stack+"password optional pam_unix.so\n", "error"},
		{"unknown-jump", "auth required pam_unix.so\n", strings.Replace(stack, "success=1", "success=2", 1), "error"},
		{"multiple-password-stacks", "password required pam_unix.so\n", stack, "error"},
		{"unreadable-include-chain", "@include missing-profile\n", stack, "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			fixture.write(t, "/etc/pam.d/passwd", "@include common-password\n")
			fixture.write(t, "/etc/pam.d/sshd", "@include common-auth\n@include common-account\n@include common-session\n@include common-password\n")
			fixture.write(t, "/etc/pam.d/common-auth", test.commonAuth)
			fixture.write(t, "/etc/pam.d/common-account", "account required pam_unix.so\n")
			fixture.write(t, "/etc/pam.d/common-session", "session required pam_unix.so\n")
			fixture.write(t, "/etc/pam.d/common-password", test.commonPassword)
			fixture.run(t, "password-min-length", test.want)
		})
	}
}

func TestRHEL7PasswdWrapper(t *testing.T) {
	const entry = "auth include system-auth\naccount include system-auth\npassword substack system-auth\n" +
		"-password optional pam_gnome_keyring.so use_authtok\npassword substack postlogin\n"
	const stack = "password requisite pam_pwquality.so try_first_pass local_users_only retry=3 authtok_type=\n" +
		"password sufficient pam_unix.so sha512 shadow nullok try_first_pass use_authtok\npassword required pam_deny.so\n"
	for _, test := range []struct{ name, entry, stack, postlogin, want string }{
		{"reported-stack-root-exempt", entry, stack, "session optional pam_lastlog.so\n", "fail"},
		{"root-also-enforced", entry, strings.Replace(stack, "retry=3", "retry=3 enforce_for_root", 1), "session optional pam_lastlog.so\n", "pass"},
		{"postlogin-also-changes-passwords", entry, stack, "password optional pam_unix.so\n", "error"},
		{"unrecognized-wrapper-module", strings.Replace(entry, "pam_gnome_keyring.so", "pam_unix.so", 1), stack, "", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			fixture.write(t, "/etc/pam.d/passwd", test.entry)
			fixture.write(t, "/etc/pam.d/system-auth", test.stack)
			fixture.write(t, "/etc/pam.d/postlogin", test.postlogin)
			fixture.run(t, "password-min-length", test.want)
		})
	}
}

func TestRHEL7SSHCollectors(t *testing.T) {
	for _, key := range []string{"ssh-root-login", "ssh-empty-password", "ssh-max-auth"} {
		for _, test := range []struct{ name, config, native, want string }{
			{"canonical", "PermitRootLogin no\n", "", "pass"},
			{"comments-tabs-case", " permitrootlogin\tno # yes\n# Match User root\n", "", "pass"},
			{"first-obtained", "PermitRootLogin no\nPermitRootLogin yes\n", "", "pass"},
			{"match-unrelated", "Match User auditor\n X11Forwarding no\n", "", "pass"},
			{"match-override", "Match User auditor\n PermitRootLogin no\n PermitEmptyPasswords no\n MaxAuthTries 4\n", "", "error"},
			{"match-cr-separator", "Match\rUser auditor\n PermitRootLogin no\n", "", "error"},
			{"include", "Include /etc/ssh/sshd_config.d/*.conf\n", "", "pass"},
			{"include-under-match", "Match User auditor\n Include sshd_config.d/*.conf\n", "", "error"},
			{"native-error", "MaxAuthTries -1\n", "printf '%s\\n' 'invalid configuration' >&2; exit 1", "error"},
			{"missing-property", "", "printf '%s\\n' 'unrelated yes'", "error"},
			{"insecure-native-values", "", "printf '%s\\n' 'permitrootlogin yes' 'permitemptypasswords yes' 'maxauthtries 9'", "fail"},
		} {
			t.Run(key+"/"+test.name, func(t *testing.T) {
				fixture := newRHEL7Fixture(t)
				fixture.write(t, "/etc/ssh/sshd_config", strings.ReplaceAll(test.config, "/etc/ssh/", fixture.root+"/etc/ssh/"))
				if test.native != "" {
					fixture.binary(t, "sshd", test.native)
				}
				fixture.run(t, key, test.want)
			})
		}
	}
	t.Run("included-match", func(t *testing.T) {
		fixture := newRHEL7Fixture(t)
		fixture.write(t, "/etc/ssh/sshd_config", "Include sshd_config.d/*.conf\n")
		fixture.write(t, "/etc/ssh/sshd_config.d/01-test.conf", "Match User auditor\n PermitRootLogin yes\n")
		fixture.run(t, "ssh-root-login", "error")
	})
}

func TestRHEL7SSHCryptoPolicyInclude(t *testing.T) {
	const policy = "/etc/crypto-policies/back-ends/opensshserver.config"
	const include = "Include " + policy + "\n"
	for _, test := range []struct{ name, dropIn, cryptoPolicy, native, want string }{
		{"global-crypto-policy", include, "Ciphers aes128-ctr\n", "", "pass"},
		{"native-noncompliance", include, "Ciphers aes128-ctr\n", "printf '%s\\n' 'permitrootlogin yes'", "fail"},
		{"missing-crypto-policy", include, "", "", "error"},
		{"match-in-crypto-policy", include, "Match User auditor\n PermitRootLogin yes\n", "", "error"},
		{"further-include", include, "Include /etc/another.conf\n", "", "error"},
		{"unknown-nested-include", "Include /etc/another.conf\n", "", "", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			fixture.write(t, "/etc/ssh/sshd_config", "Include sshd_config.d/*.conf\n")
			fixture.write(t, "/etc/ssh/sshd_config.d/50-redhat.conf",
				strings.ReplaceAll(test.dropIn, policy, fixture.root+policy))
			if test.cryptoPolicy != "" {
				fixture.write(t, policy, test.cryptoPolicy)
			}
			if test.native != "" {
				fixture.binary(t, "sshd", test.native)
			}
			fixture.run(t, "ssh-root-login", test.want)
		})
	}
}

func TestRHEL7SSHNativeParser(t *testing.T) {
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		t.Skip("native sshd not installed; isolated collector tests still run")
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not installed for temporary native-parser host key")
	}
	key := filepath.Join(t.TempDir(), "hostkey")
	if output, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("generate temporary test host key: %v: %s", err, output)
	}
	for _, test := range []struct{ name, rule, config, want string }{
		{"root-case-tabs-comment", "ssh-root-login", "permitrootlogin\tno # yes\n", "pass"},
		{"root-comment-not-setting", "ssh-root-login", "PermitRootLogin yes # change to no\n", "fail"},
		{"root-first-no", "ssh-root-login", "PermitRootLogin no\nPermitRootLogin yes\n", "pass"},
		{"root-first-yes", "ssh-root-login", "PermitRootLogin yes\nPermitRootLogin no\n", "fail"},
		{"root-unrelated-match", "ssh-root-login", "PermitRootLogin no\nMatch User auditor\n X11Forwarding no\n", "pass"},
		{"root-from-crypto-policy", "ssh-root-login", "Include @fixture@/etc/ssh/sshd_config.d/*.conf\nPermitRootLogin no\n", "fail"},
		{"empty-keyword-case", "ssh-empty-password", "permitemptypasswords yes\n", "fail"},
		{"empty-comment-not-setting", "ssh-empty-password", "PermitEmptyPasswords yes # no\n", "fail"},
		{"auth-tabs", "ssh-max-auth", "maxauthtries\t4\n", "pass"},
		{"auth-comment-not-setting", "ssh-max-auth", "MaxAuthTries 9 # change to 4\n", "fail"},
		{"auth-first-four", "ssh-max-auth", "MaxAuthTries 4\nMaxAuthTries 6\n", "pass"},
		{"auth-negative", "ssh-max-auth", "MaxAuthTries -1\n", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			fixture.binary(t, "sshd", `exec "`+sshd+`" "$@"`)
			if strings.Contains(test.config, "@fixture@") {
				const policy = "/etc/crypto-policies/back-ends/opensshserver.config"
				fixture.write(t, "/etc/ssh/sshd_config.d/50-redhat.conf", "Include "+fixture.root+policy+"\n")
				fixture.write(t, policy, "PermitRootLogin yes\n")
			}
			fixture.write(t, "/etc/ssh/sshd_config", "HostKey "+key+"\n"+strings.ReplaceAll(test.config, "@fixture@", fixture.root))
			fixture.run(t, test.rule, test.want)
		})
	}
}

func TestRHEL7SystemdCollectors(t *testing.T) {
	for _, key := range []string{"auditd", "rsyslog", "chronyd"} {
		for _, test := range []struct{ name, script, want string }{
			{"enabled-active", "printf '%s\\n' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=enabled'", "pass"},
			{"inactive", "printf '%s\\n' 'LoadState=loaded' 'ActiveState=inactive' 'UnitFileState=enabled'", "fail"},
			{"disabled", "printf '%s\\n' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=disabled'", "fail"},
			{"runtime-only", "printf '%s\\n' 'LoadState=loaded' 'ActiveState=active' 'UnitFileState=enabled-runtime'", "fail"},
			{"not-found", "printf '%s\\n' 'LoadState=not-found'", "fail"},
			{"bus-error", "printf '%s\\n' 'cannot connect to bus' >&2; exit 1", "error"},
			{"incomplete", "printf '%s\\n' 'LoadState=loaded' 'ActiveState=active'", "error"},
			{"duplicate", "printf '%s\\n' 'LoadState=loaded' 'LoadState=error' 'ActiveState=active' 'UnitFileState=enabled'", "error"},
		} {
			t.Run(key+"/"+test.name, func(t *testing.T) {
				fixture := newRHEL7Fixture(t)
				fixture.binary(t, "systemctl", `[ "$1" = "show" ] || exit 91
`+test.script)
				fixture.run(t, key, test.want)
			})
		}
	}
}

func TestRHEL7RedirectCollector(t *testing.T) {
	for _, family := range []string{"ipv4", "ipv6"} {
		other := "ipv4"
		attributes := []string{"accept_redirects"}
		if family == "ipv4" {
			other = "ipv6"
			attributes = append(attributes, "send_redirects")
		}
		key := "icmp-redirects-" + family
		t.Run(family+"/disabled", func(t *testing.T) {
			newRHEL7Fixture(t).run(t, key, "pass")
		})
		for _, target := range []string{"all", "default", "eth0", "lo"} {
			for _, attribute := range attributes {
				t.Run(family+"/"+target+"/"+attribute, func(t *testing.T) {
					fixture := newRHEL7Fixture(t)
					fixture.write(t, "/proc/sys/net/"+family+"/conf/"+target+"/"+attribute, "1\n")
					fixture.run(t, key, "fail")
					fixture.run(t, "icmp-redirects-"+other, "pass")
				})
				t.Run(family+"/"+target+"/"+attribute+"/missing", func(t *testing.T) {
					fixture := newRHEL7Fixture(t)
					if err := os.Remove(filepath.Join(fixture.root, "proc/sys/net", family, "conf", target, attribute)); err != nil {
						t.Fatal(err)
					}
					fixture.run(t, key, "error")
				})
			}
		}
		t.Run(family+"/unavailable", func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			if err := os.Rename(filepath.Join(fixture.root, "proc/sys/net", family), filepath.Join(fixture.root, "unavailable")); err != nil {
				t.Fatal(err)
			}
			fixture.run(t, key, "error")
			fixture.run(t, "icmp-redirects-"+other, "pass")
		})
		t.Run(family+"/invalid", func(t *testing.T) {
			fixture := newRHEL7Fixture(t)
			fixture.write(t, "/proc/sys/net/"+family+"/conf/eth0/accept_redirects", "invalid\n")
			fixture.run(t, key, "error")
		})
	}
	t.Run("ipv6/accept-even-when-forwarding", func(t *testing.T) {
		fixture := newRHEL7Fixture(t)
		fixture.write(t, "/proc/sys/net/ipv6/conf/eth0/accept_redirects", "2\n")
		fixture.run(t, "icmp-redirects-ipv6", "fail")
	})
}

func TestRHEL7ModesAndKernelAssertions(t *testing.T) {
	for _, test := range []struct {
		key, mode, want string
		uid, gid        int
	}{
		{"shadow-permissions", "0", "pass", 0, 0}, {"shadow-permissions", "000", "pass", 0, 0},
		{"shadow-permissions", "400", "pass", 0, 0}, {"shadow-permissions", "0400", "pass", 0, 0},
		{"shadow-permissions", "600", "fail", 0, 0}, {"shadow-permissions", "644", "fail", 0, 0},
		{"shadow-permissions", "400", "fail", 1000, 0}, {"shadow-permissions", "400", "fail", 0, 1000},
		{"passwd-permissions", "644", "pass", 0, 0}, {"passwd-permissions", "666", "fail", 0, 0},
		{"passwd-permissions", "644", "fail", 1000, 0}, {"sshd-permissions", "600", "pass", 0, 0},
		{"sshd-permissions", "644", "fail", 0, 0}, {"sshd-permissions", "600", "fail", 0, 1000},
	} {
		t.Run(fmt.Sprintf("%s/%s/%d/%d", test.key, test.mode, test.uid, test.gid), func(t *testing.T) {
			assertRHEL7Policy(t, test.key, map[string]any{"file_permissions": map[string]any{
				"data": map[string]any{"mode": test.mode, "uid": test.uid, "gid": test.gid},
			}}, test.want)
		})
	}
	for _, test := range []struct{ key, value, want string }{
		{"syn-cookies", "1\n", "pass"}, {"syn-cookies", "0", "fail"}, {"syn-cookies", "2", "fail"},
		{"ip-forwarding", "0\n", "pass"}, {"ip-forwarding", "1", "fail"},
	} {
		t.Run(test.key+"/"+test.value, func(t *testing.T) {
			assertRHEL7Policy(t, test.key, map[string]any{"kernel_value": opaParsedOutput("command", test.value, 0, "raw")}, test.want)
		})
	}
}
