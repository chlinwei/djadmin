//go:build cgo

package main

// This file intentionally fails compilation whenever CGO is enabled.
type autoadmin_must_be_built_with_CGO_ENABLED_0 struct{}

var _ int = autoadmin_must_be_built_with_CGO_ENABLED_0{}
