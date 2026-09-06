package inspection

import "testing"

func TestValidateGossSpec(t *testing.T) {
	valid := "file:\n  /etc/hosts:\n    exists: true\n    mode: \"0644\"\n"
	if err := validateGossSpec(valid); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	invalid := "file:\n  /etc/hosts:\n    exists: \"not-a-boolean\"\n"
	if err := validateGossSpec(invalid); err == nil {
		t.Fatal("invalid spec must be rejected")
	}
	broken := "file: [unclosed"
	if err := validateGossSpec(broken); err == nil {
		t.Fatal("broken yaml must be rejected")
	}
}

// 裸数字端口键是用户常见写法，goss 引擎接受（键按字符串归一）；校验器必须同语义。
func TestValidateGossSpecAcceptsNumericPortKeys(t *testing.T) {
	spec := `port:
  61616:
    listening: true
    ip: ["0.0.0.0"]
`
	if err := validateGossSpec(spec); err != nil {
		t.Fatalf("numeric port key should pass schema validation: %v", err)
	}
	quoted := `port:
  "tcp:61616":
    listening: true
`
	if err := validateGossSpec(quoted); err != nil {
		t.Fatalf("quoted protocol:port key should pass: %v", err)
	}
}
