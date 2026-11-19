package executor

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type rhel7PolicyDefinition struct {
	Key    string         `json:"key"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

func rhel7PolicyDefinitions() []rhel7PolicyDefinition {
	command := func(key, script, parse string) map[string]any {
		return map[string]any{"key": key, "exec": script, "parse": parse}
	}
	config := func(policy, remediation string, commands ...map[string]any) map[string]any {
		inputs := make([]any, len(commands))
		for i := range commands {
			inputs[i] = commands[i]
		}
		return map[string]any{
			"policy": policy, "remediation": remediation, "run_user": "root",
			"input_commands": inputs, "input_files": []any{},
		}
	}
	rules := []rhel7PolicyDefinition{{
		Key: "password-max-age", Name: "口令有效期不超过 90 天",
		Config: config(`package baseline
default defaults_ok := false
max_days := to_number(fields[1]) if {
    some line in input.login_defs.lines
    fields := regex.split("\\s+", trim_space(split(line, "#")[0]))
    count(fields) == 2
    fields[0] == "PASS_MAX_DAYS"
} else := -1
defaults_ok if { max_days >= 0; max_days <= 90 }
default account_ok(_) := false
account_ok(fields) if { fields[1] == "locked" }
account_ok(fields) if {
    fields[1] == "set"
    days := to_number(fields[2])
    days >= 0
    days <= 90
}
bad_accounts := [row |
    some line in input.account_ages.lines
    fields := split(line, ":")
    not account_ok(fields)
    days := to_number(fields[2])
    row := {"account": fields[0], "password_state": fields[1], "max_days": days}
]
assertions contains a if {
    a := {"name": "新建本地账号默认最长口令有效期 ≤ 90 天", "pass": defaults_ok,
          "expected": "0..90", "actual": max_days}
}
assertions contains a if {
    a := {"name": "存量本地可登录账号的可用口令最长有效期 ≤ 90 天",
          "pass": count(bad_accounts) == 0, "expected": "可用口令非空且最长有效期为 0..90 天",
          "actual": {"accounts_in_scope": count(input.account_ages.lines), "noncompliant_accounts": bad_accounts}}
}`,
			"设置 /etc/login.defs 的 PASS_MAX_DAYS 90；对结果中列出的本地可登录账号逐个评估后执行 chage -M 90 <账号>。锁定口令不要求过期；本项不证明 LDAP/AD 等远程账号策略合规。",
			command("login_defs", "cat /etc/login.defs", "lines"),
			command("account_ages", rhel7AccountAgeCollector, "lines")),
	}, {
		Key: "password-min-length", Name: "口令最小长度不低于 12 位",
		Config: config(`package baseline
default profile_ok(_) := false
profile_ok(f) if {
    to_number(f[1]) >= 0
    credits := [to_number(f[i]) | i := [2,3,4,5][_]]
    positive := sum([n | n := credits[_]; n > 0])
    required := sum([-n | n := credits[_]; n < 0])
    required_classes := count([n | n := credits[_]; n < 0])
    positive_classes := count([n | n := credits[_]; n > 0])
    minclass := to_number(f[9])
    uncredited_classes := max([0, minclass-required_classes-positive_classes])
    guaranteed := max([required+max([0,minclass-required_classes]),
                      ceil((to_number(f[1])+required+uncredited_classes)/2),
                      to_number(f[1])-positive])
    guaranteed >= 12
    f[6] == "1"
    f[7] == "1"
    f[8] == "1"
    f[10] == "1"
}
assertions contains a if {
    some line in input.pam_password_policy.lines
    fields := split(line, "|")
    a := {"name": concat("", [fields[0], " 的本地口令质量强制检查"]),
          "pass": profile_ok(fields),
          "expected": "强制 pam_pwquality、root 不豁免、后续 pam_unix 复用已校验口令，扣除积分后的长度下界 ≥ 12",
          "actual": {"profile": fields[0], "minlen": fields[1], "credits": array.slice(fields,2,6),
                     "enforce_for_root": fields[6], "mandatory": fields[7], "use_authtok": fields[8],
                     "minclass": fields[9], "enforcing": fields[10]}}
}`,
			"检查 passwd 和 sshd 实际引用的 PAM password 链：首个 password 模块应为 required/requisite pam_pwquality.so，带 enforce_for_root；后续 pam_unix.so 必须 use_authtok。设置生效的 minlen ≥ 12，建议 dcredit/ucredit/lcredit/ocredit 均为 0 或负数。PAM 参数覆盖 pwquality.conf；混合委托或复杂控制流需单独审计。",
			command("pam_password_policy", rhel7PAMCollector, "lines")),
	}}
	for _, spec := range []struct{ key, name, field, expected, remediation string }{
		{"ssh-root-login", "SSH 禁止 root 直接登录", "permitrootlogin", "no", "在 /etc/ssh/sshd_config 中设置 PermitRootLogin no；先用 sshd -t 校验，再在维护窗口重载 sshd。存在 Match/Include 时需按连接上下文审计，不能只判断全局配置。"},
		{"ssh-empty-password", "SSH 禁止空口令登录", "permitemptypasswords", "no", "设置 PermitEmptyPasswords no；先用 sshd -t 校验，再在维护窗口重载 sshd；同时处理空口令账号。"},
		{"ssh-max-auth", "SSH 最大认证尝试次数不超过 4 次", "maxauthtries", "0..4", "设置 MaxAuthTries 4；先用 sshd -t 校验，再在维护窗口重载 sshd。"},
	} {
		script := strings.ReplaceAll(rhel7SSHCollector, "@FIELD@", spec.field)
		condition := `value == "no"`
		if spec.field == "maxauthtries" {
			condition = `to_number(value) >= 0; to_number(value) <= 4`
		}
		policy := strings.NewReplacer("@NAME@", spec.name, "@EXPECTED@", spec.expected, "@CONDITION@", condition).Replace(`package baseline
value := trim_space(input.sshd_effective.raw)
default compliant := false
compliant if { @CONDITION@ }
assertions contains a if {
    a := {"name": "@NAME@", "pass": compliant, "expected": "@EXPECTED@", "actual": value}
}`)
		rules = append(rules, rhel7PolicyDefinition{Key: spec.key, Name: spec.name,
			Config: config(policy, spec.remediation, command("sshd_effective", script, "raw"))})
	}
	for _, spec := range []struct{ key, name, path, modes, expected string }{
		{"shadow-permissions", "/etc/shadow 权限受限", "/etc/shadow", `{"0","000","400","0400"}`, "000/0400"},
		{"passwd-permissions", "/etc/passwd 权限为 0644", "/etc/passwd", `{"644"}`, "0644"},
		{"sshd-permissions", "/etc/ssh/sshd_config 权限为 0600", "/etc/ssh/sshd_config", `{"600"}`, "0600"},
	} {
		policy := strings.NewReplacer("@NAME@", spec.name, "@MODES@", spec.modes, "@EXPECTED@", spec.expected).Replace(`package baseline
facts := input.file_permissions.data
assertions contains a if {
    a := {"name": "@NAME@", "pass": facts.mode in @MODES@, "expected": "@EXPECTED@", "actual": facts.mode}
}
assertions contains a if {
    a := {"name": "文件属主 UID 应为 0", "pass": facts.uid == 0, "expected": 0, "actual": facts.uid}
}
assertions contains a if {
    a := {"name": "文件属组 GID 应为 0", "pass": facts.gid == 0, "expected": 0, "actual": facts.gid}
}`)
		rules = append(rules, rhel7PolicyDefinition{Key: spec.key, Name: spec.name,
			Config: config(policy, "按结果核对文件目标的权限与 root:root 属主属组后整改，并重新扫描。/etc/shadow 本规则仅接受 000 或 0400，不接受 0600；不自动执行权限修改。",
				command("file_permissions", `stat -Lc '{"mode":"%a","uid":%u,"gid":%g}' `+spec.path, "json"))})
	}
	for _, spec := range []struct{ key, name, parameter, value string }{
		{"syn-cookies", "开启 SYN Flood 防护", "net.ipv4.tcp_syncookies", "1"},
		{"ip-forwarding", "禁用 IP 转发", "net.ipv4.ip_forward", "0"},
	} {
		policy := strings.NewReplacer("@NAME@", spec.name, "@VALUE@", spec.value).Replace(`package baseline
value := trim_space(input.kernel_value.raw)
assertions contains a if {
    a := {"name": "@NAME@", "pass": value == "@VALUE@", "expected": "@VALUE@", "actual": value}
}`)
		rules = append(rules, rhel7PolicyDefinition{Key: spec.key, Name: spec.name,
			Config: config(policy, "核对业务用途后设置 "+spec.parameter+" = "+spec.value+"，并持久化到系统使用的 sysctl 配置；本项观测当前内核值，不修改主机。",
				command("kernel_value", "sysctl -n "+spec.parameter, "raw"))})
	}
	for _, spec := range []struct{ family, label, attributes, jsonAttributes, values string }{
		{"ipv4", "IPv4", "accept_redirects send_redirects", `["accept_redirects","send_redirects"]`, "0|1"},
		{"ipv6", "IPv6", "accept_redirects", `["accept_redirects"]`, "0|1|2"},
	} {
		policy := strings.NewReplacer("@FAMILY@", spec.family, "@ATTRIBUTES@", spec.jsonAttributes).Replace(`package baseline
prefix := "net.@FAMILY@.conf."
attributes := @ATTRIBUTES@
default complete := false
complete if {
    keys := {split(line,"=")[0] | line := input.redirects.lines[_]}
    names := {trim_suffix(trim_prefix(key,prefix),suffix) |
        key := keys[_]
        attribute := attributes[_]
        suffix := concat("",[".",attribute])
        startswith(key,prefix)
        endswith(key,suffix)
    }
    "all" in names
    "default" in names
    count(names) > 2
    every name in names {
        every attribute in attributes {
            concat("",[prefix,name,".",attribute]) in keys
        }
    }
}
assertions contains a if {
    a := {"name": "@FAMILY@ 重定向观测完整性", "pass": complete,
          "expected": {"scope": "all/default/全部当前接口", "parameters": attributes},
          "actual": input.redirects.lines}
}
assertions contains a if {
    some line in input.redirects.lines
    fields := split(line,"=")
    a := {"name": concat("",[fields[0], " 应为 0"]),
          "pass": fields[1] == "0", "expected": "0", "actual": fields[1]}
}`)
		script := strings.NewReplacer("@FAMILY@", spec.family, "@ATTRIBUTES@", spec.attributes, "@VALUES@", spec.values).Replace(rhel7RedirectCollector)
		name := "禁用 " + spec.label + " ICMP 重定向"
		remediation := "将 net." + spec.family + ".conf 下 all、default 及全部现有接口的 " +
			spec.attributes + " 均设为 0，并写入 /etc/sysctl.d/ 配置以持久化。" +
			"写入配置文件不等于立即生效；维护窗口加载后核对现有接口。协议参数不可用时返回无法验证，不默认通过。"
		rules = append(rules, rhel7PolicyDefinition{
			Key: "icmp-redirects-" + spec.family, Name: name,
			Config: config(policy, remediation, command("redirects", script, "lines")),
		})
	}
	for _, unit := range []string{"auditd", "rsyslog", "chronyd"} {
		name := map[string]string{"auditd": "auditd 审计服务已启用", "rsyslog": "rsyslog 日志服务已启用", "chronyd": "chronyd 时间同步服务已启用"}[unit]
		policy := strings.ReplaceAll(`package baseline
facts := input.service_state.data
assertions contains a if {
    a := {"name": "@UNIT@ 服务单元已加载", "pass": facts.load_state == "loaded", "expected": "loaded", "actual": facts.load_state}
}
assertions contains a if {
    a := {"name": "@UNIT@ 服务正在运行", "pass": facts.active_state == "active", "expected": "active", "actual": facts.active_state}
}
assertions contains a if {
    a := {"name": "@UNIT@ 服务持久启用", "pass": facts.unit_file_state == "enabled", "expected": "enabled", "actual": facts.unit_file_state}
}`, "@UNIT@", unit)
		rules = append(rules, rhel7PolicyDefinition{Key: unit, Name: name,
			Config: config(policy, "排查采集错误后，在维护窗口评估执行 systemctl enable --now "+unit+"；本项检查加载、运行与持久启用状态，不证明审计规则、远程日志投递或 NTP 实际同步正常。",
				command("service_state", strings.ReplaceAll(rhel7ServiceCollector, "@UNIT@", unit), "json"))})
	}
	return rules
}

// 仅输出账号名、口令可用性和最长有效期；绝不把 shadow 哈希写入采集输出或报告。
const rhel7AccountAgeCollector = `set -eu
awk '
function problem(msg) { print msg > "/dev/stderr"; bad=1 }
FILENAME==ARGV[1] {
    n=split($0,f,":")
    if (n!=9 || f[1]=="") { problem("invalid shadow record"); next }
    if (f[1] in state) { problem("duplicate shadow account"); next }
    state[f[1]]=(f[2]~/^[!*]/ ? "locked" : (f[2]=="" ? "empty" : "set"))
    age[f[1]]=(f[5]=="" ? "-1" : f[5])
    if (age[f[1]]!~/^-?[0-9]+$/) problem("invalid password maximum age")
    next
}
FILENAME==ARGV[2] {
    n=split($0,f,":")
    if (n!=7 || f[1]=="" || f[3]!~/^[0-9]+$/) { problem("invalid passwd record"); next }
    if (f[1] in seen) { problem("duplicate passwd account"); next }
    seen[f[1]]=1
    if (f[7]~/(^|\/)(nologin|false)$/) next
    if (!(f[1] in state)) { problem("missing shadow data for login account"); next }
    print f[1] ":" state[f[1]] ":" age[f[1]]
}
END { if (length(seen)==0) problem("empty passwd database"); if (bad) exit 1 }
' /etc/shadow /etc/passwd`

// 只证明待检查字段不受 Match 影响，实际配置值仍由 sshd 解析。
const rhel7SSHCollector = `set -eu
export LC_ALL=C
ssh_dir=/etc/ssh/
config=${ssh_dir}sshd_config
crypto_policy=/etc/crypto-policies/back-ends/opensshserver.config
check_scope() {
    awk -v wanted='@FIELD@' -v scope="$2" -v ssh_dir="$ssh_dir" -v crypto_policy="$crypto_policy" '
function problem(msg) {
    printf "%s:%d: %s\n", FILENAME, FNR, msg > "/dev/stderr"
    bad=1
}
{
    line=$0; sub(/^[[:blank:]]+/,"",line)
    if (line=="" || line ~ /^#/) next
    if (line !~ /^[A-Za-z][A-Za-z0-9]*([[:blank:]=]|$)/ || line ~ /[^[:print:]\t]/) {
        problem("unsupported sshd directive syntax"); next
    }
    split(line,f,/[[:blank:]=]+/); key=tolower(f[1])
    if (key=="match") {
        conditional=1
        if (scope!="main") problem("Match inside an SSH included file requires context review")
    } else if (key=="include") {
        # Inspect the vendor crypto-policy leaf, never trust it based on its filename alone.
        path=line
        sub(/^[A-Za-z]+[[:blank:]]+/,"",path)
        sub(/[[:blank:]]+(#.*)?$/,"",path)
        if (!conditional && scope=="main" && (path=="sshd_config.d/*.conf" || path==ssh_dir "sshd_config.d/*.conf"))
            print "drop-ins"
        else if (!conditional && scope=="drop-in" && path==crypto_policy)
            print "crypto-policy"
        else problem("unsupported SSH Include scope or path")
    } else if (conditional && key==wanted) {
        problem("SSH Match overrides " wanted "; connection contexts must be verified")
    }
}
END { if (bad) exit 1 }
' "$1"
}
includes=$(check_scope "$config" main)
if [ -n "$includes" ]; then
    directory=${ssh_dir}sshd_config.d
    if [ -e "$directory" ] || [ -L "$directory" ]; then
        [ -d "$directory" ] && [ -r "$directory" ] && [ -x "$directory" ] ||
            { printf '%s: unreadable SSH drop-in directory\n' "$directory" >&2; exit 1; }
    fi
    for path in "$ssh_dir"sshd_config.d/*.conf; do
        [ -e "$path" ] || [ -L "$path" ] || continue
        [ -f "$path" ] && [ -r "$path" ] || { printf '%s: unreadable SSH drop-in\n' "$path" >&2; exit 1; }
        nested=$(check_scope "$path" drop-in)
        if [ -n "$nested" ]; then
            [ -f "$crypto_policy" ] && [ -r "$crypto_policy" ] ||
                { printf '%s: unreadable SSH crypto policy\n' "$crypto_policy" >&2; exit 1; }
            check_scope "$crypto_policy" crypto-policy
        fi
    done
fi
effective=$(/usr/sbin/sshd -T -f "$config")
printf '%s\n' "$effective" | awk -v wanted='@FIELD@' '
$1==wanted { if (NF!=2 || seen++) { bad=1; next }; value=$2 }
END {
    if (bad || seen!=1) { print "incomplete sshd effective configuration" > "/dev/stderr"; exit 1 }
    if (wanted=="maxauthtries" && value!~/^[0-9]+$/) {
        print "invalid MaxAuthTries" > "/dev/stderr"; exit 1
    }
    print value
}'`

const rhel7RedirectCollector = `set -eu
base=/proc/sys/net/@FAMILY@/conf
if [ ! -d "$base" ]; then
    printf '%s\n' '@FAMILY@ redirect parameters unavailable; cannot verify' >&2
    exit 1
fi
for required in all default; do
    for attribute in @ATTRIBUTES@; do
        if [ ! -r "$base/$required/$attribute" ]; then
            printf '%s\n' "missing @FAMILY@ $required.$attribute observation" >&2
            exit 1
        fi
    done
done
interfaces=0
for path in "$base"/*; do
    name=${path##*/}
    case "$name" in all|default) ;; *) interfaces=$((interfaces+1));; esac
    for attribute in @ATTRIBUTES@; do
        value=$(cat "$path/$attribute")
        case "$value" in @VALUES@) ;; *) printf '%s\n' "invalid @FAMILY@ $name.$attribute value" >&2; exit 1;; esac
        printf 'net.@FAMILY@.conf.%s.%s=%s\n' "$name" "$attribute" "$value"
    done
done
if [ "$interfaces" -eq 0 ]; then
    printf '%s\n' 'no @FAMILY@ interface redirect observations' >&2
    exit 1
fi`

const rhel7ServiceCollector = `set -eu
export LC_ALL=C
state=$(systemctl show @UNIT@.service --property=LoadState --property=ActiveState --property=UnitFileState --no-pager)
printf '%s\n' "$state" | awk -F= '
$1=="LoadState" || $1=="ActiveState" || $1=="UnitFileState" {
    if (seen[$1]++ || NF!=2 || $2!~/^[a-z-]*$/) bad=1
    value[$1]=$2
}
END {
    if (bad || !seen["LoadState"] || value["LoadState"]=="") {
        print "invalid systemd service state" > "/dev/stderr"; exit 1
    }
    if (value["LoadState"]=="not-found") {
        print "{\"load_state\":\"not-found\",\"active_state\":\"not-applicable\",\"unit_file_state\":\"not-applicable\"}"
        exit
    }
    if (!seen["ActiveState"] || !seen["UnitFileState"] || value["ActiveState"]=="") {
        print "incomplete systemd service state" > "/dev/stderr"; exit 1
    }
    printf "{\"load_state\":\"%s\",\"active_state\":\"%s\",\"unit_file_state\":\"%s\"}\n",
        value["LoadState"],value["ActiveState"],value["UnitFileState"]
}'`

// 只解析唯一的口令处理栈，不把任意混合模块和 substack 当成等价委托。
const rhel7PAMCollector = `set -eu
export LC_ALL=C
pam_dir=/etc/pam.d/
resolve_password_stack() {
    awk -v directory="$pam_dir" -v service="$1" '
function problem(path,msg) { print path ": " msg > "/dev/stderr"; exit 1 }
function resolve(name,depth,    path,line,n,f,rc,count,control,target,leaf,child,includes,keyring,postlogin) {
    path=directory name
    if (active[name]) problem(path,"PAM password delegation cycle")
    if (depth>16 || ++files>128) problem(path,"PAM dependency limit exceeded")
    active[name]=1
    while ((rc=(getline line < path))>0) {
        sub(/#.*/,"",line); gsub(/^[[:space:]]+|[[:space:]]+$/,"",line)
        if (line=="") continue
        n=split(line,f,/[[:space:]]+/)
        if (line ~ /\\$/) problem(path,"unsupported PAM line continuation")
        if (tolower(f[1])=="-password") {
            # The stock passwd wrapper puts only an optional token consumer after system-auth.
            if (name=="passwd" && count==1 && target=="system-auth" && control=="substack" &&
                n==4 && f[2]=="optional" && f[3]=="pam_gnome_keyring.so" && f[4]=="use_authtok") {
                keyring=1
                count++
                continue
            }
            problem(path,"unsupported optional-load PAM password directive")
        }
        if (f[1]=="@include") {
            if (n!=2 || f[2]!~/^[A-Za-z0-9_][A-Za-z0-9_.-]*$/) problem(path,"unsupported PAM @include target")
            includes++
            child=resolve(f[2],depth+1)
            if (child!="") {
                if (leaf!="") problem(path,"multiple included PAM password stacks")
                leaf=child
            }
            continue
        }
        if (tolower(f[1]) !~ /^(-?(auth|account|session)|password)$/)
            problem(path,"unsupported PAM directive")
        if (tolower(f[1])!="password") continue
        count++
        control=tolower(f[2])
        if (control=="include" || control=="substack") {
            if (n!=3 || f[3]!~/^[A-Za-z0-9_][A-Za-z0-9_.-]*$/) problem(path,"unsupported PAM delegation target")
            if (keyring && count==3 && control=="substack" && f[3]=="postlogin")
                postlogin=1
            else target=f[3]
        }
    }
    if (rc<0) problem(path,"cannot read PAM configuration")
    close(path)
    if (count && includes) problem(path,"mixed PAM @include and local password directives")
    if (target!="") {
        if (count!=1 && !(count==3 && keyring && postlogin))
            problem(path,"mixed PAM password delegation and local directives")
        if (postlogin && resolve("postlogin",depth+1)!="")
            problem(path,"postlogin contains an additional PAM password stack")
        leaf=resolve(target,depth+1)
        if (leaf=="") problem(path,"empty delegated PAM password stack")
    } else if (count) leaf=path
    delete active[name]
    return leaf
}
BEGIN {
    leaf=resolve(service,1)
    if (leaf=="") problem(directory service,"empty PAM password stack")
    print leaf
}'
}
passwd_stack=$(resolve_password_stack passwd)
sshd_stack=$(resolve_password_stack sshd)
for fragment in /etc/security/pwquality.conf.d/*.conf; do
    if [ -e "$fragment" ]; then
        printf '%s\n' 'pwquality.conf.d requires version-specific review; cannot establish effective RHEL7 policy' >&2
        exit 1
    fi
done
quality=/etc/security/pwquality.conf
if [ ! -e "$quality" ]; then quality=/dev/null; fi
awk '
function problem(msg) { print msg > "/dev/stderr"; bad=1 }
function numeric(k,v) {
    if (v!~/^-?[0-9]+$/) { problem("invalid password quality number"); return 0 }
    if ((k=="minlen" && v+0<0) || (k=="minclass" && (v+0<0 || v+0>4)) ||
        (k=="enforcing" && v+0!=0 && v+0!=1)) problem("out-of-range password quality setting")
    return v+0
}
BEGIN { q["minlen"]=-1; q["dcredit"]=0; q["ucredit"]=0; q["lcredit"]=0; q["ocredit"]=0; q["minclass"]=0; q["enforcing"]=1 }
FILENAME==ARGV[1] {
    line=$0; sub(/#.*/,"",line); gsub(/^[[:space:]]+|[[:space:]]+$/,"",line)
    n=split(line,f,/[[:space:]=]+/)
    if (f[1] in q) {
        if (n!=2) problem("malformed pwquality setting")
        else q[f[1]]=numeric(f[1],f[2])
    }
    next
}
FNR==1 { a=FILENAME==ARGV[2] && !leaf_started ? 2 : 3; leaf_started=1 }
{
    line=$0; sub(/#.*/,"",line); gsub(/^[[:space:]]+|[[:space:]]+$/,"",line)
    n=split(line,f,/[[:space:]]+/)
    if (tolower(f[1])!="password") next
    f[2]=tolower(f[2])
    file=a; profiles[file]=1; position[file]++
    # This exact Debian-style stack has no quality gate; do not generalize its jump semantics.
    normalized=line; gsub(/[[:space:]]+/," ",normalized)
    if (position[file]==1 && normalized ~ /^password \[success=1 default=ignore\] pam_unix[.]so obscure (yescrypt|sha512)$/)
        debian[file]=1
    if (debian[file]) {
        if (position[file]==2 && normalized=="password requisite pam_deny.so") debian[file]=2
        else if (position[file]==3 && debian[file]==2 && normalized=="password required pam_permit.so") debian[file]=3
        else if (!(position[file]==4 && debian[file]==3 && normalized=="password optional pam_gnome_keyring.so") &&
                 position[file]!=1) problem(FILENAME ": unsupported Debian PAM password stack")
        next
    }
    if (f[2]!~/^(required|requisite|sufficient|optional)$/) {
        problem("complex PAM password control flow requires explicit review"); next
    }
    module=f[3]; sub(/^.*\//,"",module)
    if (f[3]~/\// && f[3]!~/^\/(usr\/)?lib(64)?\/security\/pam_(pwquality|unix|deny)\.so$/)
        problem("non-system PAM module path requires explicit review")
    if (module=="pam_pwquality.so") {
        if (++quality_count[file]>1 || position[file]!=1) problem("ambiguous or bypassable pam_pwquality ordering")
        mandatory[file]=(f[2]=="required" || f[2]=="requisite") ? 1 : 0
        for (k in q) effective[file,k]=q[k]
        for (i=4;i<=n;i++) {
            if (f[i]=="enforce_for_root") root[file]=1
            split(f[i],option,"=")
            if (option[1]=="conf" || option[1]=="config") problem("alternate pwquality config requires explicit review")
            if (option[1] in q) effective[file,option[1]]=numeric(option[1],option[2])
        }
    } else if (module=="pam_unix.so") {
        if (++unix_count[file]>1) problem("multiple pam_unix password modules require explicit review")
        for (i=4;i<=n;i++) if (f[i]=="use_authtok") reuse[file]=1
    } else if (module!="pam_deny.so") problem("unsupported PAM password module requires explicit review")
}
END {
    for (a=2;a<=3;a++) {
        file=a
        if (!profiles[file]) { problem("empty PAM password chain"); continue }
        if (debian[file] && debian[file]!=3) { problem("incomplete Debian PAM password stack"); continue }
        name=(a==2 ? "passwd" : "sshd")
        if (!quality_count[file]) {
            printf "%s|-1|0|0|0|0|0|0|0|0|0\n",name
        } else {
            printf "%s|%d|%d|%d|%d|%d|%d|%d|%d|%d|%d\n",name,
                effective[file,"minlen"],effective[file,"dcredit"],effective[file,"ucredit"],
                effective[file,"lcredit"],effective[file,"ocredit"],root[file]+0,
                mandatory[file]+0,(unix_count[file] && reuse[file]) ? 1 : 0,effective[file,"minclass"],effective[file,"enforcing"]
        }
    }
    if (bad) exit 1
}' "$quality" "$passwd_stack" "$sshd_stack"`

func TestExportRHEL7BaselinePolicies(t *testing.T) {
	path := os.Getenv("RHEL7_BASELINE_EXPORT")
	if path == "" {
		return
	}
	encoded, err := json.MarshalIndent(rhel7PolicyDefinitions(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
