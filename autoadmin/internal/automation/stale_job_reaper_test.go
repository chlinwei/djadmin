package automation

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// 失联判定：只有越过"自身超时 + 余量"才算失联，避免误伤跑得慢的作业。
func TestStaleJobExpired(t *testing.T) {
	now := time.Date(2026, 9, 18, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		elapsed time.Duration
		timeout uint32
		want    bool
	}{
		{"刚过超时但仍在余量内", 10*time.Minute + time.Minute, 600, false},
		{"超时+余量刚过", 10*time.Minute + staleJobGrace + time.Second, 600, true},
		{"短超时任务已越过", 70*time.Second + staleJobGrace + time.Second, 70, true},
		{"短超时任务仍在跑", 40 * time.Second, 70, false},
		{"超时为 0 回落默认 600s", 11 * time.Minute, 0, false},
		{"超时为 0 且已越默认+余量", 12*time.Minute + time.Second, 0, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			start := now.Add(-testCase.elapsed)
			if got := staleJobExpired(start, testCase.timeout, now); got != testCase.want {
				t.Errorf("elapsed=%v timeout=%ds → %v, want %v", testCase.elapsed, testCase.timeout, got, testCase.want)
			}
		})
	}
}

// 进程组隔离的**行为**验证：超时被杀后，整个进程组（含 ansible fork 出来的子进程）都必须消失。
//
// 这是回归保护：默认的 exec.CommandContext 只杀 controller，`sh -c 'sleep &'` 这类被 fork 出来的
// 子进程会活下来（作业 #831 就留下 8 个这样的孤儿）。判定方式是"进程组是否还存在"——
// 组长被杀后只要还有任一成员存活，`kill(-pgid, 0)` 就不会返回 ESRCH。
//
// 注意必须带重试：SIGKILL 的送达与回收是异步的，信号发出后立刻检查会偶发"组还在"的假失败。
func TestIsolateProcessGroupKillsChildren(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	// 子进程留在同一个进程组里（非交互 sh 不做作业控制），模拟 ansible 的 --forks 工作进程。
	command := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & wait")
	isolateProcessGroup(command)
	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output

	if err := runInProcessGroup(command); err == nil {
		t.Fatal("超时后应当返回错误")
	}
	if command.Process == nil {
		t.Fatal("进程未启动")
	}
	pid := command.Process.Pid
	// 兜底清理：即使断言失败（说明整组没被杀干净），也别把 sleep 留在这儿。
	defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()

	assertProcessGroupGone(t, pid)
}

// 命令**正常结束但留下了子进程**时也必须回收整组——这条覆盖 Cancel 覆盖不到的路径：
// 命令自己退出了、context 没取消，因此不会触发 Cancel；而 ansible 结束后仍挂着的 ssh/工作进程
// 正是这种形态（作业 #831 留下的孤儿就属于"没人回收"这一类）。
func TestRunInProcessGroupCleansUpAfterNormalExit(t *testing.T) {
	// 注意用不带超时的 context：命令会立即 exit 0，超时与 Cancel 都不会参与。
	command := exec.CommandContext(context.Background(), "/bin/sh", "-c", "sleep 30 & exit 0")
	isolateProcessGroup(command)
	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output

	if err := runInProcessGroup(command); err != nil {
		t.Fatalf("正常退出不应报错：%v", err)
	}
	if command.Process == nil {
		t.Fatal("进程未启动")
	}
	pid := command.Process.Pid
	defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()
	assertProcessGroupGone(t, pid)
}

// assertProcessGroupGone 等待进程组消失（SIGKILL 的送达与回收是异步的，必须重试）。
func assertProcessGroupGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var state error
	for time.Now().Before(deadline) {
		state = syscall.Kill(-pid, 0)
		if state == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("进程组 %d 仍有存活进程（孤儿 fork 未被回收）：kill(-pgid,0) = %v", pid, state)
}

// 进程组隔离顺带保证 Run 一定会返回：子进程持有 stdout 管道时，若只等 I/O 会一直挂住。
func TestIsolateProcessGroupReturnsPromptlyOnTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30 & wait")
	isolateProcessGroup(command)
	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output

	start := time.Now()
	_ = runInProcessGroup(command)
	elapsed := time.Since(start)
	defer func() {
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
	}()
	// 500ms 超时 + WaitDelay 10s 的上限内必须返回；这里给足余量只排除"挂死"。
	if elapsed > 15*time.Second {
		t.Errorf("Run 挂住过久：%v", elapsed)
	}
}

// 收尾写入用的 context 必须**不可取消**：进程关闭/请求取消都不能阻止作业落终态，
// 否则作业会永久停在 running（2026-09-18 作业 #831 的成因）。
func TestPersistenceContextSurvivesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 模拟请求中断 / 服务关闭
	if ctx.Err() == nil {
		t.Fatal("前置条件不成立：原 context 应当已取消")
	}
	persist := persistenceContext(ctx)
	if persist.Err() != nil {
		t.Fatalf("收尾 context 不应被取消，实际 err=%v", persist.Err())
	}
	select {
	case <-persist.Done():
		t.Fatal("收尾 context 不应随原 context 一起结束")
	default:
	}
}

// 终态判定：只有终态才停止按实时输出推送（其余状态要去读实时块）。
func TestIsTerminalJobStatus(t *testing.T) {
	for _, status := range []string{"success", "failed", "cancelled", " FAILED "} {
		if !isTerminalJobStatus(status) {
			t.Errorf("%q 应判为终态", status)
		}
	}
	for _, status := range []string{"pending", "running", "", "unknown"} {
		if isTerminalJobStatus(status) {
			t.Errorf("%q 不应判为终态", status)
		}
	}
}

// 实时输出按"攒够字节"或"显式 Flush"落库，且不会丢尾块。
func TestLiveLogStreamerChunking(t *testing.T) {
	var chunks []string
	streamer := newLiveLogStreamer(func(chunk string) { chunks = append(chunks, chunk) })
	if _, err := streamer.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("未达阈值不应刷出：%v", chunks)
	}
	// 超过字节阈值 → 立刻刷一次
	big := strings.Repeat("x", liveLogChunkBytes)
	if _, err := streamer.Write([]byte(big)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("达阈值应刷出一块，实际 %d 块", len(chunks))
	}
	if chunks[0] != "hello"+big {
		t.Fatalf("首块内容不对：%d 字节", len(chunks[0]))
	}
	// 结尾的零散输出靠 Flush 兜住
	if _, err := streamer.Write([]byte("tail")); err != nil {
		t.Fatalf("write: %v", err)
	}
	streamer.Flush()
	if len(chunks) != 2 || chunks[1] != "tail" {
		t.Fatalf("Flush 未把尾块写出：%v", chunks)
	}
	// sink 为 nil 时不得 panic（安装类作业走这条路径）
	nilSink := newLiveLogStreamer(nil)
	if _, err := nilSink.Write([]byte("noop")); err != nil {
		t.Fatalf("nil sink write: %v", err)
	}
	nilSink.Flush()
}

// inventory 别名要能一眼看出是哪台机器，且必须唯一（同名同 IP 会被 ansible 并成一台）。
func TestInventoryHostLabel(t *testing.T) {
	used := map[string]bool{}
	first := inventoryHostLabel(hostSnapshot{HostID: 222, HostName: "pvg-esb4-207", HostIP: "10.25.66.207"}, used)
	if first != "pvg-esb4-207(10.25.66.207)" {
		t.Errorf("别名 = %q", first)
	}
	// 同名同 IP 的另一条记录不能拿到同一个别名
	dup := inventoryHostLabel(hostSnapshot{HostID: 999, HostName: "pvg-esb4-207", HostIP: "10.25.66.207"}, used)
	if dup == first || dup != "pvg-esb4-207(10.25.66.207)#999" {
		t.Errorf("撞名别名 = %q，应当带 #id 后缀区分", dup)
	}
	// 名字为空时回落 host-<id>，仍带 IP
	empty := inventoryHostLabel(hostSnapshot{HostID: 7, HostIP: "10.0.0.7"}, map[string]bool{})
	if empty != "host-7(10.0.0.7)" {
		t.Errorf("空名别名 = %q", empty)
	}
	// 纯中文名规整后没有可读内容 → 回落 host-<id>，不出现 "__-01(...)"
	chinese := inventoryHostLabel(hostSnapshot{HostID: 8, HostName: "主机-01", HostIP: "10.0.0.8"}, map[string]bool{})
	if chinese != "host-8(10.0.0.8)" {
		t.Errorf("纯中文名别名 = %q", chinese)
	}
}

// 别名里的脏字符必须被规整：空格会把别名拆成 inventory 变量，中文/斜杠等在语法里有歧义。
func TestSanitizeInventoryLabel(t *testing.T) {
	cases := map[string]string{
		"pvg-esb4-207":      "pvg-esb4-207",
		" my host ":         "my_host",
		"tib_nginx-208":     "tib_nginx-208",
		"主机-01":             "__-01", // 中文被规整掉；别名层会因"无可读内容"回落 host-<id>
		"a/b:c, d":          "a_b_c__d",
		"node.example.com.": "node.example.com.",
	}
	for input, want := range cases {
		if got := sanitizeInventoryLabel(input); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", input, got, want)
		}
	}
}
