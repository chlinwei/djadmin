package sysconfig

import (
	"database/sql"
	"testing"
	"time"

	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"
)

// normalize 是系统参数写入的唯一入口：int/bool/json 严格校验、secret 掩码透传与哈希、
// 未知类型按字符串存储。回归用例固化这些语义，防止误放宽校验或重复哈希掩码。
func TestNormalize(t *testing.T) {
	// int
	if got, err := normalize("int", "300"); err != nil || got != "300" {
		t.Fatalf("int normalize = %q, %v", got, err)
	}
	if _, err := normalize("int", "abc"); err != ErrValueNotInteger {
		t.Fatalf("int invalid = %v, want ErrValueNotInteger", err)
	}
	// bool：true/1/yes 与 false/0/no 大小写不敏感，其余拒绝
	for _, input := range []string{"true", "TRUE", "1", "Yes"} {
		if got, err := normalize("bool", input); err != nil || got != "true" {
			t.Fatalf("bool(%q) = %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"False", "0", "no"} {
		if got, err := normalize("bool", input); err != nil || got != "false" {
			t.Fatalf("bool(%q) = %q, %v", input, got, err)
		}
	}
	if _, err := normalize("bool", "maybe"); err != ErrValueNotBoolean {
		t.Fatalf("bool invalid = %v, want ErrValueNotBoolean", err)
	}
	// json：对象/数组透传，非法字符串拒绝
	if got, err := normalize("json", map[string]any{"a": 1}); err != nil || got != `{"a":1}` {
		t.Fatalf("json object = %q, %v", got, err)
	}
	if got, err := normalize("json", "[1,2]"); err != nil || got != "[1,2]" {
		t.Fatalf("json array = %q, %v", got, err)
	}
	if _, err := normalize("json", "{not-json"); err != ErrValueNotJSON {
		t.Fatalf("json invalid = %v, want ErrValueNotJSON", err)
	}
	// 未知类型按字符串
	if got, err := normalize("string", "hello"); err != nil || got != "hello" {
		t.Fatalf("string normalize = %q, %v", got, err)
	}
}

// secret：非掩码值必须被哈希（不可逆明文落库）；掩码值原样透传（编辑页未改密时防重复哈希）。
func TestNormalizeSecret(t *testing.T) {
	hashed, err := normalize("secret", "my-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hashed == "my-password" {
		t.Fatal("secret must not be stored as plaintext")
	}
	if !identity.VerifyPassword(hashed, "my-password") {
		t.Fatalf("hashed value must verify against original: %v", err)
	}
	if got, err := normalize("secret", "******"); err != nil || got != "******" {
		t.Fatalf("mask passthrough = %q, %v", got, err)
	}
}

func TestTyped(t *testing.T) {
	if got := typed("int", "300"); got != 300 {
		t.Fatalf("int typed = %#v, want 300", got)
	}
	if got := typed("bool", "yes"); got != true {
		t.Fatalf("bool typed = %#v, want true", got)
	}
	if got := typed("secret", "hashed-value"); got != "******" {
		t.Fatalf("secret typed must mask, got %#v", got)
	}
	if got := typed("json", `{"k":1}`); got.(map[string]any)["k"] != float64(1) {
		t.Fatalf("json typed should parse to map, got %#v", got)
	}
	if got := typed("string", "raw"); got != "raw" {
		t.Fatalf("string typed = %#v", got)
	}
}

func TestMapConfigMasksSecretAndFormatsTime(t *testing.T) {
	row := db.SysConfig{
		ID: 1, Key: "sys.assets.agent.grpc_advertise_addr", Value: "hashed-value", ValueType: "secret",
		Name: "对外地址", DefaultValue: sql.NullString{String: "default", Valid: true},
		CreateTime: time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC), UpdateTime: time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC),
	}
	item := mapConfig(row)
	if item.Value != "******" {
		t.Fatalf("secret value must be masked to client, got %#v", item.Value)
	}
	if item.CreateTime != "2026-09-03T08:00:00Z" {
		t.Fatalf("create_time format wrong: %q", item.CreateTime)
	}
	if item.DefaultValue != "******" { // defaultValue 同为 secret 类型，一样被掩码
		t.Fatalf("secret default_value must be masked, got %#v", item.DefaultValue)
	}
}
