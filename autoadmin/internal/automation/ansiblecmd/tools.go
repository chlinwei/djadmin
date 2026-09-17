//go:build tools

// tools.go：把"仅生成器需要"的依赖钉在 go.mod 里。
//
// 生成器放在 _generate（下划线前缀，`go test/vet ./...` 不编译、`go mod tidy` 也不扫描），
// 若不在主模块显式引用，tidy 会把 go-embed-python/pip 的依赖删掉，导致 `make ansible-embed`
// 报 "updates to go.mod needed"。tools 标签让本文件只在 tidy 时参与，正常构建不带。
package ansiblecmd

import _ "github.com/kluctl/go-embed-python/pip"
