package monitor

import (
	"regexp"
	"strings"
	"testing"
)

// 收件人解析回归：与 Django test action 一致，接受数组或逗号/分号分隔字符串。
func TestParseAlertMediaTestRecipients(t *testing.T) {
	cases := []struct {
		raw  any
		want []string
	}{
		{"a@x.com, b@x.com;c@x.com", []string{"a@x.com", "b@x.com", "c@x.com"}},
		{[]any{"a@x.com", " b@x.com ", ""}, []string{"a@x.com", "b@x.com"}},
		{[]any{"a@x.com", "b@x.com;c@x.com"}, []string{"a@x.com", "b@x.com", "c@x.com"}},
		{nil, nil},
		{123, nil},
		{"   ", nil},
	}
	for index, item := range cases {
		got := parseAlertMediaTestRecipients(item.raw)
		if len(got) != len(item.want) {
			t.Fatalf("case %d: got %v, want %v", index, got, item.want)
		}
		for position := range got {
			if got[position] != item.want[position] {
				t.Fatalf("case %d: got %v, want %v", index, got, item.want)
			}
		}
	}
}

func TestBuildSMTPMessageText(t *testing.T) {
	message := string(buildSMTPMessage("alert@djadmin.local", []string{"ops@x.com"}, "test alert", "内容正文", "text"))
	for _, want := range []string{
		"From: alert@djadmin.local\r\n",
		"To: ops@x.com\r\n",
		"Subject: test alert\r\n",
		"Content-Type: text/plain; charset=UTF-8",
		"内容正文",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "multipart/alternative") {
		t.Fatalf("text message must not contain html part:\n%s", message)
	}
}

func TestBuildSMTPMessageHTML(t *testing.T) {
	message := string(buildSMTPMessage("alert@djadmin.local", []string{"a@x.com", "b@x.com"}, "server down", "<b>down</b>", "html"))
	if !strings.Contains(message, `Content-Type: multipart/alternative; boundary="`) {
		t.Fatalf("missing multipart header:\n%s", message)
	}
	if strings.Count(message, "Content-Type: text/plain; charset=UTF-8") != 1 ||
		strings.Count(message, "Content-Type: text/html; charset=UTF-8") != 1 {
		t.Fatalf("alternative parts missing:\n%s", message)
	}
	if !strings.Contains(message, "<b>down</b>") {
		t.Fatalf("html body missing:\n%s", message)
	}
	if !strings.Contains(message, "To: a@x.com, b@x.com\r\n") {
		t.Fatalf("recipients missing:\n%s", message)
	}
}

// 主题含非 ASCII 时必须做 RFC 2047 编码，否则部分 SMTP 服务端拒收或乱码。
func TestBuildSMTPMessageEncodedSubject(t *testing.T) {
	message := string(buildSMTPMessage("a@x.local", []string{"b@x.local"}, "告警通知", "正文", "text"))
	subjectLine := ""
	for _, line := range regexp.MustCompile("\r\n").Split(message, -1) {
		if strings.HasPrefix(line, "Subject: ") {
			subjectLine = line
		}
	}
	if !strings.HasPrefix(subjectLine, "Subject: =?UTF-8?q?") {
		t.Fatalf("subject not RFC 2047 encoded: %q", subjectLine)
	}
}
