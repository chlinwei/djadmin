// 生成 embedansible 变体所需的嵌入数据：把 requirements.txt 里的 Python 包
// （目前只有 ansible-core）下载并解压到 ../embeddata/<goos>-<goarch>/。
//
// 放在 _generate（下划线前缀）是为了让 `go test ./...` / `go vet ./...` 不编译它
// ——它依赖 go-embed-python 的 pip 包，只在 `make ansible-embed` 时跑。
//
// 用法（由 go:generate 触发）：
//
//	make ansible-embed
//	# 等价于 cd autoadmin && go generate ./internal/automation/ansiblecmd/...
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kluctl/go-embed-python/pip"
)

func main() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("无法定位生成器源码位置")
	}
	dir := filepath.Dir(file)
	requirements := filepath.Join(dir, "..", "requirements.txt")
	targetDir := filepath.Join(dir, "..", "embeddata")

	// 只生成当前部署平台（Makefile 的 GOOS/GOARCH 是 linux/amd64）。
	// 需要别的平台时改这里并重跑；ansible-core 是纯 Python，唯一的二进制约是
	// PyYAML/cryptography 的 manylinux wheel。
	platforms := []string{"manylinux_2_17_x86_64", "manylinux_2_28_x86_64", "manylinux2014_x86_64"}
	if err := pip.CreateEmbeddedPipPackages(requirements, runtime.GOOS, runtime.GOARCH, platforms, targetDir); err != nil {
		panic(err)
	}

	// CreateEmbeddedPipPackages 会写一个 `package data` 的 embed_*.go；我们的 embed
	// 由 ansiblecmd_embedded.go 用 //go:embed all:embeddata 负责，不需要它，且留着会
	// 把 embeddata 变成一个 Go 包（被 ./... 扫描）。删掉。
	matches, _ := filepath.Glob(filepath.Join(targetDir, "embed_*.go"))
	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			panic(err)
		}
	}

	fmt.Printf("生成完成：%s（requirements: %s）\n", targetDir, requirements)
}
