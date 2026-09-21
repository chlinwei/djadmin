//go:build embedansible

package ansiblecmd

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kluctl/go-embed-python/embed_util"
	"github.com/kluctl/go-embed-python/python"
)

//go:embed all:embeddata
var embeddataFS embed.FS

var (
	prepareOnce sync.Once
	prepareErr  error

	pythonExe  string
	ansibleBin string
	// cfgPath 指向一个只含 [defaults] 的空 ansible.cfg，用于屏蔽部署机上的
	// /etc/ansible/ansible.cfg，保证行为自包含；用户显式设置 ANSIBLE_CONFIG 时不覆盖。
	cfgPath string
)

// prepare 只在首次调用时解压嵌入的 Python 与 ansible-core，解压结果按内容哈希缓存，
// 后续进程启动直接复用（见 go-embed-python 的 EmbeddedFiles）。进程生命周期内不清理。
func prepare() error {
	prepareOnce.Do(func() {
		ep, err := python.NewEmbeddedPython("autoadmin")
		if err != nil {
			prepareErr = fmt.Errorf("extract embedded python: %w", err)
			return
		}
		// 平台数据由 ansible-embed 生成在 embeddata/<goos>-<goarch>/ 下；
		// 当前构建只有 linux-amd64（Makefile 的 GOOS/GOARCH）。
		sub, err := fs.Sub(embeddataFS, "embeddata/linux-amd64")
		if err != nil {
			prepareErr = fmt.Errorf("open embedded ansible data: %w", err)
			return
		}
		libFs, err := embed_util.NewEmbeddedFiles(sub, "autoadmin-ansible")
		if err != nil {
			prepareErr = fmt.Errorf("extract embedded ansible: %w", err)
			return
		}
		pythonExe, err = ep.GetExePath()
		if err != nil {
			prepareErr = fmt.Errorf("locate embedded python: %w", err)
			return
		}
		// 把嵌入 ansible 的目录加到嵌入 Python 的纯库目录（site-packages）下的 .pth，
		// 这样无需设置 PYTHONPATH（避免被 Ansible local 连接继承、污染目标端 Python）。
		sitePackages, err := embeddedSitePackages(pythonExe)
		if err != nil {
			prepareErr = err
			return
		}
		if err := os.MkdirAll(sitePackages, 0o755); err != nil {
			prepareErr = fmt.Errorf("create embedded site-packages: %w", err)
			return
		}
		pth := filepath.Join(sitePackages, "zz-autoadmin-ansible.pth")
		if err := os.WriteFile(pth, []byte(libFs.GetExtractedPath()+"\n"), 0o644); err != nil {
			prepareErr = fmt.Errorf("write .pth: %w", err)
			return
		}
		ansibleBin = filepath.Join(libFs.GetExtractedPath(), "bin", "ansible-playbook")

		if strings.TrimSpace(os.Getenv("ANSIBLE_CONFIG")) == "" {
			cfgPath = filepath.Join(os.TempDir(), "autoadmin-ansible-embed.cfg")
			if err := os.WriteFile(cfgPath, []byte("[defaults]\n"), 0o644); err != nil {
				cfgPath = ""
			}
		}
	})
	return prepareErr
}

// embeddedSitePackages 用嵌入 Python 自报纯库目录，避免把 Python 版本写死。
func embeddedSitePackages(exe string) (string, error) {
	out, err := exec.Command(exe, "-c", "import sysconfig; print(sysconfig.get_paths()['purelib'])").Output()
	if err != nil {
		return "", fmt.Errorf("resolve embedded site-packages: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("resolve embedded site-packages: empty path")
	}
	return path, nil
}

// CommandContext 返回"用嵌入 CPython + ansible-core 执行"的命令（-tags embedansible）。
// 首次调用会解压运行时（约 1s，之后走缓存）。
//
// 环境见 ansibleEnv（默认 minimal stdout 回调，让 shell 任务的 stdout 可见）；
// 另在用户未显式设置 ANSIBLE_CONFIG 时用自包含的空配置屏蔽部署机的 /etc/ansible/ansible.cfg。
func CommandContext(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if err := prepare(); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, pythonExe, append([]string{ansibleBin}, args...)...)
	env := ansibleEnv()
	if cfgPath != "" {
		env = append(env, "ANSIBLE_CONFIG="+cfgPath)
	}
	command.Env = env
	return command, nil
}
