package buildinfo

// 构建期注入的版本信息：默认 dev，正式构建由 Makefile 通过
// -ldflags "-X github.com/chlinwei/djadmin/dj_agent/internal/buildinfo.Version=vX.Y.Z" 注入。
var (
	Version = "dev"
	Commit  = "none"
)
