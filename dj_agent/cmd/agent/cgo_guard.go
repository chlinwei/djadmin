//go:build cgo

package main

// 构建守卫：只在 CGO_ENABLED=1 时参与编译并故意报错。
// dj-agent 必须是纯静态二进制，一旦链接 cgo 就会绑定构建机 glibc 版本，分发到旧内核/旧发行版会直接起不来。
type dj_agent_must_be_built_with_CGO_ENABLED_0 struct{}

var _ int = dj_agent_must_be_built_with_CGO_ENABLED_0{}
