// WebSSH 终端在 gRPC 文件传输长连接上的复用实现：一个 request_id 对应一个 PTY 进程，
// backend 通过 terminal_open/data/resize/close 请求驱动，agent 把 PTY 输出持续回传，
// 进程退出时回传一次 terminal_exit_response。之所以复用同一条 AgentChannel.Session
// 连接而不是新开一条 gRPC 服务，是因为 agent 是拨出方、backend 无法主动连接 agent，
// 复用已建立的长连接可以避免再实现一套 Hello/鉴权/重连逻辑。
package grpcfile

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"

	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile/pb"
)

type terminalSession struct {
	cmd     *exec.Cmd
	ptyFile *os.File
}

func (s *session) handleTerminalOpen(req *pb.TerminalOpenRequest) {
	resp := &pb.TerminalOpenResponse{RequestId: req.RequestId}

	shell := strings.TrimSpace(os.Getenv("DJ_AGENT_TERM_SHELL"))
	if shell == "" {
		shell = "/bin/bash"
	}
	openCtx, err := resolveTerminalOpenContext(strings.TrimSpace(req.GetTargetUser()))
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_TerminalOpenResponse{TerminalOpenResponse: resp}})
		return
	}
	homeDir := openCtx.homeDir

	// 强制走 login+interactive shell，保证 /etc/profile 与交互式 rc 都能被加载。
	execCmd := exec.Command(shell, "-il")
	// 构造与 SSH 登录一致的干净环境：设置 HOME/USER/LOGNAME/SHELL/TERM，并复刻
	// PAM(pam_env) 加载 /etc/environment 的行为；同时不继承 agent 进程自身环境，
	// 避免把 DJ_AGENT_RABBITMQ_URL 等敏感配置泄漏进用户终端。
	execCmd.Env = buildTerminalEnv(shell, homeDir, openCtx.effectiveUser)
	execCmd.Dir = homeDir
	if openCtx.credential != nil {
		execCmd.SysProcAttr = &syscall.SysProcAttr{Credential: openCtx.credential}
	}

	cols := toValidTermDim(int(req.Cols), 120)
	rows := toValidTermDim(int(req.Rows), 32)

	ptyFile, err := pty.StartWithSize(execCmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		resp.Error = err.Error()
		_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_TerminalOpenResponse{TerminalOpenResponse: resp}})
		return
	}
	resp.EffectiveUser = openCtx.effectiveUser

	s.terminalsMu.Lock()
	s.terminals[req.RequestId] = &terminalSession{cmd: execCmd, ptyFile: ptyFile}
	s.terminalsMu.Unlock()

	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_TerminalOpenResponse{TerminalOpenResponse: resp}})

	go s.terminalReadLoop(req.RequestId, ptyFile)
	go s.terminalWaitProcess(req.RequestId, execCmd)
}

func (s *session) handleTerminalData(req *pb.TerminalDataRequest) {
	term := s.getTerminal(req.RequestId)
	if term == nil || len(req.Data) == 0 {
		return
	}
	if _, err := term.ptyFile.Write(req.Data); err != nil {
		s.closeTerminal(req.RequestId, -1, err.Error())
	}
}

func (s *session) handleTerminalResize(req *pb.TerminalResizeRequest) {
	term := s.getTerminal(req.RequestId)
	if term == nil {
		return
	}
	ws := &pty.Winsize{
		Cols: uint16(toValidTermDim(int(req.Cols), 120)),
		Rows: uint16(toValidTermDim(int(req.Rows), 32)),
	}
	_ = pty.Setsize(term.ptyFile, ws)
}

func (s *session) handleTerminalClose(req *pb.TerminalCloseRequest) {
	s.closeTerminal(req.RequestId, 0, "")
}

func (s *session) terminalReadLoop(requestID string, ptyFile *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptyFile.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if sendErr := s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_TerminalDataResponse{
				TerminalDataResponse: &pb.TerminalDataResponse{RequestId: requestID, Data: data},
			}}); sendErr != nil {
				return
			}
		}
		if err != nil {
			if !isExpectedPtyCloseError(err) {
				slog.Warn("grpc terminal read error", "request_id", requestID, "err", err)
			}
			return
		}
	}
}

func (s *session) terminalWaitProcess(requestID string, cmd *exec.Cmd) {
	err := cmd.Wait()
	exitCode := 0
	errMessage := ""
	if err != nil {
		errMessage = err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	s.closeTerminal(requestID, int32(exitCode), errMessage)
}

// closeTerminal 统一负责关闭 PTY/杀掉子进程并回传一次 exit 事件；重复调用是安全的
// （第二次调用时 map 里已找不到对应 session，直接返回）。
func (s *session) closeTerminal(requestID string, exitCode int32, errMessage string) {
	s.terminalsMu.Lock()
	term, ok := s.terminals[requestID]
	if ok {
		delete(s.terminals, requestID)
	}
	s.terminalsMu.Unlock()
	if !ok || term == nil {
		return
	}

	if term.ptyFile != nil {
		_ = term.ptyFile.Close()
	}
	if term.cmd != nil && term.cmd.Process != nil {
		_ = term.cmd.Process.Kill()
	}

	_ = s.send(&pb.AgentFrame{Payload: &pb.AgentFrame_TerminalExitResponse{
		TerminalExitResponse: &pb.TerminalExitResponse{RequestId: requestID, ExitCode: exitCode, Error: errMessage},
	}})
}

func (s *session) getTerminal(requestID string) *terminalSession {
	s.terminalsMu.Lock()
	defer s.terminalsMu.Unlock()
	return s.terminals[requestID]
}

// closeAllTerminals 在整条 gRPC 会话断开时调用，避免子进程随连接断开泄漏残留。
func (s *session) closeAllTerminals() {
	s.terminalsMu.Lock()
	ids := make([]string, 0, len(s.terminals))
	for id := range s.terminals {
		ids = append(ids, id)
	}
	s.terminalsMu.Unlock()
	for _, id := range ids {
		s.closeTerminal(id, -1, "agent 连接已断开")
	}
}

func isExpectedPtyCloseError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EIO {
			return true
		}
	}
	return false
}

func toValidTermDim(v int, fallback int) int {
	if v <= 0 {
		return fallback
	}
	if v > 500 {
		return 500
	}
	return v
}

func resolveTerminalHomeDir() string {
	customHome := strings.TrimSpace(os.Getenv("DJ_AGENT_TERM_HOME"))
	if customHome != "" {
		return customHome
	}
	homeDir, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(homeDir) != "" {
		return homeDir
	}
	return "/tmp"
}

type terminalOpenContext struct {
	homeDir       string
	effectiveUser string
	credential    *syscall.Credential
}

func resolveTerminalOpenContext(targetUser string) (*terminalOpenContext, error) {
	current := currentUsername()
	ctx := &terminalOpenContext{
		homeDir:       resolveTerminalHomeDir(),
		effectiveUser: current,
	}

	if strings.TrimSpace(targetUser) == "" {
		return ctx, nil
	}
	if !isTargetUserAllowed(targetUser) {
		return nil, errors.New("目标用户不在允许列表中")
	}
	usr, err := user.Lookup(targetUser)
	if err != nil {
		return nil, err
	}
	effectiveUser := strings.TrimSpace(usr.Username)
	if effectiveUser == "" {
		effectiveUser = strings.TrimSpace(targetUser)
	}
	if effectiveUser == "" {
		return nil, errors.New("目标用户不能为空")
	}

	homeDir := strings.TrimSpace(usr.HomeDir)
	if homeDir == "" {
		homeDir = "/tmp"
	}
	ctx.homeDir = homeDir
	ctx.effectiveUser = effectiveUser

	if os.Geteuid() != 0 {
		if effectiveUser != current {
			return nil, errors.New("agent 非 root 运行，无法切换到其他系统用户")
		}
		return ctx, nil
	}

	if effectiveUser == current {
		return ctx, nil
	}

	uid, err := strconv.ParseUint(strings.TrimSpace(usr.Uid), 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(strings.TrimSpace(usr.Gid), 10, 32)
	if err != nil {
		return nil, err
	}
	ctx.credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	return ctx, nil
}

func isTargetUserAllowed(targetUser string) bool {
	raw := strings.TrimSpace(os.Getenv("DJ_AGENT_TERM_ALLOWED_USERS"))
	if raw == "" {
		return true
	}
	for _, item := range strings.Split(raw, ",") {
		if strings.TrimSpace(item) == targetUser {
			return true
		}
	}
	return false
}

// buildTerminalEnv 构造一个贴近 SSH 交互登录的终端环境。
// 直接 spawn shell 会跳过 sshd/PAM，因此这里显式补齐 sshd 会话变量，并手动加载
// /etc/environment（等价于 pam_env 的行为），其余变量交给 login shell 的 profile/rc 脚本加载。
func buildTerminalEnv(shell, homeDir, effectiveUser string) []string {
	env := loadEtcEnvironment()

	// 与 sshd 建立会话时设置的核心登录变量对齐。
	env["HOME"] = homeDir
	env["SHELL"] = shell
	if username := strings.TrimSpace(effectiveUser); username != "" {
		env["USER"] = username
		env["LOGNAME"] = username
	}

	if strings.TrimSpace(env["TERM"]) == "" {
		if t := strings.TrimSpace(os.Getenv("TERM")); t != "" {
			env["TERM"] = t
		} else {
			env["TERM"] = "xterm-256color"
		}
	}
	if strings.TrimSpace(env["PATH"]) == "" {
		if p := strings.TrimSpace(os.Getenv("PATH")); p != "" {
			env["PATH"] = p
		} else {
			env["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
		}
	}
	// locale 兜底，避免中文/UTF-8 乱码。
	for _, key := range []string{"LANG", "LANGUAGE", "LC_ALL", "LC_CTYPE"} {
		if strings.TrimSpace(env[key]) == "" {
			if v := strings.TrimSpace(os.Getenv(key)); v != "" {
				env[key] = v
			}
		}
	}

	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func currentUsername() string {
	if u, err := user.Current(); err == nil {
		if name := strings.TrimSpace(u.Username); name != "" {
			return name
		}
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

// loadEtcEnvironment 解析 /etc/environment（KEY=VALUE，支持可选 export 前缀与成对引号），
// 复刻 SSH 登录时 PAM 注入这些变量的效果；文件不存在时返回空 map。
func loadEtcEnvironment() map[string]string {
	result := map[string]string{}
	data, err := os.ReadFile("/etc/environment")
	if err != nil {
		return result
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" {
			result[key] = val
		}
	}
	return result
}
