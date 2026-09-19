package logcollect

import (
	"fmt"
	"regexp"
	"strings"
)

// 正则方言：处理规则里的首行/续行正则由 **Filebeat** 执行，Filebeat 与平台服务端用的都是
// Go 的 regexp（RE2），不是浏览器/Java 那套正则方言。所以规则的"合法"判定必须是 RE2 的判定，
// 且要在保存时就做——曾经只在认证时才编译，结果是：
//
//	规则里写 [A-Za-z\u4e00-\u9fa5]（Java/JS 写中文范围的常见写法）能存能调试（前端用 JS 正则，
//	`\u4e00` 合法），但主机上的 Filebeat 用 RE2 编译直接失败 → 多行合并不生效、采集配置起不来；
//	认证时报 "error parsing regexp: invalid escape sequence: `\u`"，光看这句看不出该怎么改。
//
// 这个文件只做两件事：
//  1. 保存/认证前用 RE2 编译一次（validateRulePattern）；
//  2. 把 RE2 的编译错误翻译成"照着改就行"的说明（regexHint）——两处共用同一份，避免说法漂移。
//
// 有意不做"自动改写方言"（把 \uXXXX 悄悄换成 \x{XXXX}）：这类改写只覆盖得了这一种写法，
// 覆盖不了后向断言、反向引用等 RE2 根本不支持的能力，用户会以为平台什么 Java 正则都吃。
// 该报错就报错、并给出等价写法，与本仓库既有的"报错而不是猜"一致。

// validateRulePattern 用 RE2 编译处理规则里的正则，通过返回 ""，失败返回可直接展示给用户的说明。
// field 是给用户看的字段名（首行正则 / 续行正则）。
func validateRulePattern(field, pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Sprintf("%s不合法：%s（Go RE2，与 Filebeat 同一个引擎）%s", field, err.Error(), regexHint(pattern))
	}
	return ""
}

// regexHint 按 pattern 里**实际出现的写法**给出"该改成什么"。
//
// 不看 Go 的错误文案来决定提示内容：RE2 的错误只报"哪个转义无效"，不说是哪一家方言
// （例如后向断言 (?<=a) 的报错是 "invalid named capture"，照着这句根本想不到是断言）。
func regexHint(pattern string) string {
	hints := make([]string, 0, 2)
	seen := make(map[string]bool, 2)
	add := func(hint string) {
		if !seen[hint] {
			seen[hint] = true
			hints = append(hints, hint)
		}
	}
	forEachPatternToken(pattern, func(token patternToken) bool {
		switch token.kind {
		case tokenUnicodeEscape:
			add("RE2 不支持 \\uXXXX（Java/JS 的码点写法），请写成 \\x{XXXX}，例如 \\u4e00 → \\x{4e00}")
		case tokenLookaround:
			// 断言是"看着能像正则但其实跑不了"的重灾区：RE2 四种断言一个都不支持。
			// 续行正则尤其容易踩：`^(?!首行正则)` 是最自然的续行写法，而这里恰好**不需要**它。
			add("RE2 不支持断言（(?= / (?! / (?<= / (?<! 一个都不支持）：续行正则不需要断言——" +
				"Filebeat 用 negate/match=after 表达“不以首行正则开头的行并入上一行”，续行正则留空即可；" +
				"其他场景请把判断挪进 ingest pipeline 处理器")
		case tokenBackReference:
			add("RE2 不支持 \\1 这类写法（反向引用与八进制转义都不支持），请改用命名捕获 + pipeline 处理器，或拆成多条规则")
		case tokenAtomicOrPossessive:
			add("RE2 不支持原子组 (?>) 与占有量词（如 a*+），请去掉它们")
		}
		return true
	})
	if len(hints) == 0 {
		return ""
	}
	return "。" + strings.Join(hints, "；")
}

type patternTokenKind int

const (
	tokenUnicodeEscape patternTokenKind = iota
	tokenLookaround
	tokenBackReference
	tokenAtomicOrPossessive
)

type patternToken struct {
	kind  patternTokenKind
	start int
}

// forEachPatternToken 扫描 pattern，对每个"RE2 不支持的写法"回调一次；回调返回 false 可提前结束。
//
// 按转义与字符类逐字符走，而不是拿几个正则去搜：`\u` 必须排除 `\\u`（用户想要字面反斜杠+u），
// `\1` 在字符类里是八进制转义而非反向引用，盲搜会把正常规则判成非法。
func forEachPatternToken(pattern string, visit func(patternToken) bool) {
	inClass := false
	for index := 0; index < len(pattern); index++ {
		char := pattern[index]
		switch {
		case char == '\\':
			// 转义序列整体跳过（不把被转义字符当语法看）。
			if index+1 >= len(pattern) {
				return
			}
			next := pattern[index+1]
			// `\u` 与 `\<数字>` 在字符类内外都判：现场那条规则恰好是把 `\u4e00` 写在
			// `[A-Za-z\u4e00-\u9fa5]` 里的（Java/JS 写中文范围的惯用写法），按"类内不判"就会漏掉。
			switch {
			case next == 'u':
				if !visit(patternToken{kind: tokenUnicodeEscape, start: index}) {
					return
				}
			case next >= '1' && next <= '9':
				if !visit(patternToken{kind: tokenBackReference, start: index}) {
					return
				}
			}
			index++
		case char == '[':
			// `[[:alpha:]]` 这类 POSIX 类里的 `[` 不改变状态；只按最简单的情形翻转即可，
			// 判错的代价只是"少给一条提示"，仍有 RE2 编译错误兜底。
			if !inClass || index == 0 || pattern[index-1] != '[' {
				inClass = !inClass
			}
		case char == ']':
			inClass = false
		case char == '(' && !inClass:
			// 四种断言都要认出来：RE2 全都不支持，而 Go 的报错文案各不相同
			// （`(?=`/`(?!` 报 invalid or unsupported Perl syntax，`(?<=`/`(?<!` 报 invalid named
			// capture——后者的文案会把人引向"命名组写错了"，完全想不到是断言）。
			switch {
			case strings.HasPrefix(pattern[index:], "(?="), strings.HasPrefix(pattern[index:], "(?!"),
				strings.HasPrefix(pattern[index:], "(?<="), strings.HasPrefix(pattern[index:], "(?<!"):
				if !visit(patternToken{kind: tokenLookaround, start: index}) {
					return
				}
			case strings.HasPrefix(pattern[index:], "(?>"):
				if !visit(patternToken{kind: tokenAtomicOrPossessive, start: index}) {
					return
				}
			}
		case (char == '*' || char == '+' || char == '?') && !inClass:
			// 占有量词：`X*+` / `X++` / `X?+`（RE2 报 invalid nested repetition operator）。
			if index+1 < len(pattern) && pattern[index+1] == '+' {
				prev := previousPatternByte(pattern, index)
				// 排除 `(?i)+` 这种把量词当字面量的极端情况：前一个字符是分组收尾或普通字符才算量词。
				if prev != 0 && prev != '?' && prev != '(' && prev != '|' {
					if !visit(patternToken{kind: tokenAtomicOrPossessive, start: index}) {
						return
					}
				}
			}
		}
	}
}

// previousPatternByte 返回 index 之前的有效字符（跳过转义），没有则返回 0。
func previousPatternByte(pattern string, index int) byte {
	if index == 0 {
		return 0
	}
	return pattern[index-1]
}
