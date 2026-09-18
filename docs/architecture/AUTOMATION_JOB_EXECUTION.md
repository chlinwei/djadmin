# 自动化作业执行（autoadmin）

描述作业（`automation_execution_job`）从派发到终态的最终逻辑：**在哪跑、怎么超时、失败/失联如何收敛、
输出什么时候可见**。后端唯一实现为 `internal/automation`（执行入口 `executeLocalAnsible`，
收尾 `finishJob`，对账 `stale_job_reaper.go`）。Inventory 与 Playbook 的 CRUD 语义见
[AUTOMATION_INVENTORY.md](AUTOMATION_INVENTORY.md)。

## 状态机

```
pending ──(worker 消费/内联派发)──> running ──┬──> success
                                             ├──> failed      （ansible 退出码非 0 / 超时 / 对账判定失联）
                                             └──> cancelled    （用户取消：仅改状态，不杀进程）
```

- `start_time` 进入 running 时写入；`end_time`/`duration_seconds` 由应用层算（`TIMESTAMPDIFF` 是方言函数）。
- 收尾语句带 `status <> 'cancelled'` 守卫：取消后到达的收尾不再覆盖取消结论。

## 派发方式：手动"立即执行"是后台异步的

`POST /sys/automation/tasks/:id/run_now/`（`RunTaskNow`）创建作业后**立刻返回**，
作业由独立 goroutine 在后台执行（`dispatchJobAsync`，外层 6 小时兜底超时），
前端拿作业 id 去运行记录中心看状态与日志。

**为什么不在请求里同步跑**（2026-09-18 作业 #831 的成因）：原先 `run_now` 用
`context.Request.Context()` 同步执行整趟 ansible。多主机作业要跑几分钟，请求一旦中断
（浏览器关闭、网关超时、服务重启），请求的 context 就被取消，连锁反应是：

1. `commandCtx` 被取消 → ansible 的 controller 被 SIGKILL（它的 `--forks` 子进程变成孤儿，
   并卡在"输出管道无人读取"上）；
2. 临时目录**被 defer 清掉了**（所以现场看起来"跑过"）；
3. 但收尾那几条 UPDATE 用的是**同一个已取消的 context** → 写入失败 → **作业永久停在 `running`**。

因此定了两条规则：

- **派发脱离请求 context**：作业一旦派发，请求结束/取消都不再影响它。
- **收尾写入用不可取消的 context**（`persistenceContext` = `context.WithoutCancel`）：
  cancel 只用来中断执行本身，不用来中断结果落库。这两条缺一不可——只做第一条，
  服务重启仍会把正在跑的作业卡在 `running`。

监控域的安装派发（Export/Filebeat）本来就是"脱离请求 + goroutine"，此处与之统一。

## 在哪跑：本地 ansible（不是 agent 侧）

作业由控制端**本地**执行：嵌入式 CPython + ansible-core（`-tags embedansible`，见
[ansiblecmd](DJ_AGENT_ARCHITECTURE.md)）或宿主 PATH 上的 `ansible-playbook`。
一次派发对应**一次** `ansible-playbook` 调用，多主机靠 `--forks min(10, 主机数)` 并行。

- 临时目录 `/tmp/autoadmin-ansible-<随机>/`：`inventory.ini`、`playbook.yml`、`controller_key`、`known_hosts`。
- inventory 用 `ansible_user=root` + 控制端私钥 SSH 直连；`StrictHostKeyChecking=accept-new`。
- **主机别名是 `<主机名>(<IP>)`**（`inventoryHostLabel`）：ansible 的 task 输出与 PLAY RECAP 都用别名
  标识主机，用它才能一眼看出是哪台机器（早先的 `host_<id>` 在日志里无法对应到服务器）。
  别名只保留 INI 安全的字符（空格会把它拆成 inventory 变量），非 ASCII 名（中文）回落 `host-<id>`，
  同名同 IP 的重复记录加 `#<id>` 后缀——**别名必须唯一**，否则 ansible 会把它们并成一台、任务只跑一次。
  别名只由后端生成、不被任何代码解析，因此改它只影响展示。
- 子进程按**独立进程组**运行（`Setpgid`），并在返回前按组回收，见下文"孤儿进程"。

## 超时语义

| 项 | 值 |
|---|---|
| 来源 | 任务上的 `execution_timeout_seconds`（作业行只存 `task_id`，读取时 `COALESCE(...,600)`） |
| 取值域 | 1–14400 秒，写入任务时校验 |
| 缺省 | 600 秒（任务未配置或作业无关联任务） |
| 强制方式 | **执行进程内的 `context.WithTimeout`**，超时后返回退出码 **124**，摘要追加 "Playbook execution timed out." |
| 落库 | 超时 → 作业 `failed`（`finishJob` 按 `failed>0 || code!=0` 判定） |
| 后台派发外层上限 | 6 小时（`jobDispatchTimeout`），仅防 goroutine 意外永久挂住 |

⚠️ **这是进程内超时**：执行进程一旦消失（被 kill、崩溃、worker 重启），它永远不会触发——
所以必须有下面对账兜底。

## 失联作业对账（stale job reaper）

**问题**：执行进程消失后作业会永久停在 `running`，界面上「查看日志」一直显示"等待新输出"，
没有任何东西把它收敛（2026-09-18 作业 #831 即此情形：执行进程消失，留下 8 个孤儿 ansible 进程）。

**判定口径**：`running` 且 `start_time` 超过"作业自身超时 + **2 分钟余量**"。
余量刻意给足——正常路径下进程内超时会先把作业置成 failed，对账只在执行进程确实不在时命中，
不会误伤跑得慢的作业。

- 扫描周期 1 分钟，由 **worker 模式**启动（`StartStaleJobReaper`，单实例部署、进程内唯一 goroutine，
  随进程退出终止；与监控域失联告警对账同一范式）。
- 置失败走 `FailStaleAutomationJob`，带 `status='running'` 守卫——正常收尾可能同时在写，
  没有守卫会把刚成功的作业改写成失败。
- 摘要写明"执行进程已失联、实际输出可能已丢失"，并有 `execution_mode: stale_reaper` 标记，
  便于与正常失败区分。

## 输出与作业日志

作业的可见文本分两段，日志 WebSocket（`GET /ws/automation/jobs/:id/logs/`）按作业状态自动选源：

| 阶段 | 数据来源 | 说明 |
|---|---|---|
| 运行中（非终态） | `automation_execution_job_log` 的**实时块** | 边跑边看；块为空时前端显示"等待新输出" |
| 终态（success/failed/cancelled） | `automation_execution_host_log` 的**按主机结果行** | 每台一行，带退出码与 stdout/stderr |

- **实时块**：`executeLocalAnsible` 用管道读 ansible 的 stdout/stderr，边写内存缓冲（供结束时的按主机行）
  边按块落库——攒够 4KB 或间隔 1.5s 刷一次（`liveLogStreamer`）。写失败只丢弃该块：
  实时输出是"尽力而为"的展示，不影响作业本身。
- **块的清理**：作业收尾时由执行方删除（`DeleteAutomationJobLogChunks`）。终态后日志视图改读按主机
  结果行，留着只会让同一份 ansible 输出重复展示。执行进程没来得及清（失联作业）时由对账补清。
  因此这张表只在"运行中"有数据，没有留存/清理负担。
- **多主机作业的输出仍会重复**：收尾写按主机行时，每台都把同一份 ansible stdout 存了一份，
  日志视图最终会把整份输出按主机重复展示。这是既有格式（便于按主机看退出码与错误），
  未做重构。

## 孤儿进程：为什么必须按进程组管理

只杀 `ansible-playbook` 的 controller 是不够的：`--forks N` 会 fork 出多个工作进程，它们还会拉起 ssh。
只杀 controller 会让这些子进程变成孤儿（PPID=1），并**卡在"输出管道已无人读取"上永不退出**——占着
到生产主机的 SSH 连接。据此定了三条规则（`isolateProcessGroup` + `runInProcessGroup`）：

1. `SysProcAttr.Setpgid` 让 ansible 及其 fork 处在**独立进程组**（组长即 controller，pgid == pid）。
2. `Cancel` 改为按**整组**杀（`kill(-pgid)`），让超时/取消能迅速清场。
3. **返回前无条件再按组补一刀**：覆盖"命令正常结束但留下子进程"这条 Cancel 覆盖不到的路径
   （典型是 ansible 跑完仍有挂住的 ssh）。正常结束、超时、被杀三条路径都收口。

另外设了 `WaitDelay = 10s`：进程被杀后若仍有子进程持有 stdout/stderr 管道，`Wait` 会一直等 I/O
结束而挂住；有兜底期限才能保证 `Run` 一定返回。**但 WaitDelay 到期返回的是 `ErrWaitDelay`**，
若此时进程本身已退出且退出码为 0（ansible 其实成功了），不能因为一个残留子进程把作业判失败——
`runInProcessGroup` 因此按真实退出码判定，把"成功 + 残留子进程"归一为成功。

## 取消语义

`CancelJob` **只更新数据库**（`status='cancelled'`、写 `end_time`/时长、摘要 "Cancelled by user"），
不会去杀已派发的进程。对已经失联的作业，取消是最快的止血手段：DB 状态一变，
日志视图立刻从"等待新输出"变成"已结束（已取消）"。
