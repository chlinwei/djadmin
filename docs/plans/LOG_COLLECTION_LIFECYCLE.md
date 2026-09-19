# 日志采集配置与数据生命周期（变更 / 状态 / 下发 / 清理）

> 状态：设计已定（剩余待确认项见 §9）。**§8 Phase 0 前置修复、Phase 1 漂移可见、Phase 2 下发闭环均已完成
> （2026-09-18）**；Phase 2 的最终语义已并入
> [LOG_COLLECTION_ARCHITECTURE.md](../architecture/LOG_COLLECTION_ARCHITECTURE.md) §8.8。
> 剩余为 Phase 3 清理闭环（前置是"模板日志定义行加稳定 id"）。
>
> 相关：采集器迁移见 [LOG_COLLECTOR_MIGRATION.md](LOG_COLLECTOR_MIGRATION.md)。

## 0. 背景与原则

改日志路径 / 改档位 / 增删日志文件 / 关停采集后，缺少确定答案：要不要重新下发？
旧 datastream 怎么办？要不要清数据？谁触发？本文把这些问题收敛成一套可预期、可审计的模型。

两条不可动摇的原则：

1. **配置生命周期 ≠ 数据生命周期**：删配置默认不动数据。
2. **停止采集 ≠ 删除数据**：关闭只停写，数据由保留策略（ILM）到期。

## 1. 现状与根因

| # | 现象 | 位置 |
|---|---|---|
| 1 | 期望态与实际态未比对：体检只看 `config_fingerprint` 是否非空，不比对内容 | `internal/logcollect/log_health.go:318-340`（注释仍写"后端没有期望指纹生成器"） |
| 2 | 期望指纹能力其实已存在，只用于"下发时跳过" | `internal/logcollect/log_config_render.go`（`loadHostLogRenderInput` + `renderHostLogConfig`） |
| 3 | 配置变更本身不触发下发：只有「安装成功后自动下发一次」与「批量下发」两个入口，改完配置要人工点。✅**已收敛**（Phase 2）：不做自动下发（§9 第 8 条），但待下发的**全量台数**在页面上可见（`pending-summary`），并有一键应用入口 | `internal/logcollect/log_batch_api.go`、`fronted/src/views/monitor/log-collectors/index.vue` |
| 4 | ✅**已修复**（Phase 2）原为：Filebeat 停止就点不了「下发配置」 | `fronted/src/views/monitor/log-collectors/index.vue`（`canApplyConfig` 原来还要求 `runtime_status==='running'`；2026-09-18 前该逻辑在 `monitor/index.vue`） |
| 5 | 数据流是服务级、档位在流名里 → 改档位必产生新流；旧流无管理 | `internal/shared/logstream/name.go:17`、`internal/logcollect/datastream_status.go:146` |
| 6 | 主机片段由 agent 全量托管、下发时清残留；ES 数据不随之清理 | `dj_agent/internal/executor/builtin_actions.go:183` |
| 7 | 清理只有按服务，不支持按日志文件；孤儿流只以 `Recognized=false` 隐式呈现，无显式状态与清理入口 | `internal/logcollect/log_datastream_cleanup.go` |
| 8 | ✅**已修（配置侧，2026-09-19）**：原为"模板保存是全量替换日志定义、不带稳定 id"→ 改名与删除无法区分、只改路径也会让服务级覆盖失效、且删旧行会被覆盖行的外键挡住（`assets_application_service_log_setting.log_definition_id` 无 `ON DELETE CASCADE`；`translate` 把 1451 转成「资产仍被其他记录引用，无法删除」）。现改为**按 id 增量写**：带 id 原地更新（id 稳定 → 覆盖值保留）、无 id 新增、未提交的删除（删前级联清覆盖行），顺序先删后改再插；另加"提交 id 必须属于本模板""日志名不能为空"两条校验，新建/复制模板时提交的 id 一律按新增处理。**数据侧仍未解决**：改名的旧 `log_name` 与删除在数据层仍无法区分（清理提示需要它），留给 Phase 3 的清理闭环 | `internal/assets/template.go`（`applyTemplateLogWrites` / `planTemplateLogWrites`）、`db/queries/*/assets.sql` |
| 9 | ✅**已修复**（Phase 0）原为：**两条下发路径语义不一致**：单条 `POST /log-targets/:id/apply/` 只调 `configure_filebeat_output`（只写 `filebeat.yml`），不下发 inputs.d 片段、不写 `config_fingerprint`，却把 `runtime_status` 置 running、清空 `last_error`；批量/安装后自动走的是 `applyLogTargetConfigRow` 全流程 | `internal/logcollect/log_target_actions.go:442-487` vs `:633-708`；agent 侧 `dj_agent/internal/executor/builtin_actions.go:143` |
| 10 | ✅**已修复**（Phase 0）原为：**指纹只覆盖 inputs.d 片段，不含 output**（ES 地址/账号/TLS）：只改默认集群地址时指纹不变 → 命中"指纹一致则跳过"，`filebeat.yml` 保持旧值（与架构文档 §8.3"下发一定会写主配置"的承诺冲突） | `internal/logcollect/log_config_render.go:201`、`log_target_actions.go:669` |
| 11 | ✅**已修复**（Phase 2）原为：**批量动作在 1000 台规模下不成立**：批量下发在单个 HTTP 请求内**串行**遍历目标、每台 2 次 agent gRPC（超时 60s+120s），无进度、无断点；批量安装/重试对每台 `go func()` 内联跑 ansible，**无并发上限**且绕过平台 worker 队列 | 现为 `internal/logcollect/log_batch_job.go`（执行器）、`log_batch_api.go`（入口）、`db/migrations/*/000033_log_batch_job.*.sql` |

## 2. 状态模型（核心）

状态拆成三层正交维度，避免用一个字段硬塞：

- **配置态（Desired vs Applied）**：`never` / `drift` / `synced`
- **运行态（agent + filebeat）**：`agent_offline` / `not_installed` / `task_pending` / `install_failed` / `stopped` / `error` / `running`
- **数据态（ES 是否有写入）**：`flowing` / `no_data` / `unknown`（按需查 ES）

再对每类对象给出**单一结论状态**（按优先级取第一个命中）。单一结论状态**只由配置态 + 运行态决定**；
数据态是并排的第二个徽标，不进结论状态、也不进列表默认渲染（理由见 §2.4）。

### 2.1 主机采集目标（`monitor_log_collection_target`）

| 优先级 | 状态 | 判定 | 颜色 | 建议动作 |
|---|---|---|---|---|
| 1 | Agent 离线 | `gateway.IsOnline=false` | 灰 | 检查 dj-agent |
| 2 | 未安装 Filebeat | `agent_installed=false` 且无进行中任务 | 灰 | 离线安装 |
| 3 | 任务进行中 | `install_status='pending'` | 蓝 | 等待/取消 |
| 4 | 安装失败 | `install_status=failed` | 红 | 看历史/重试 |
| 5 | 待下发（从未） | `config_fingerprint=''` | 橙 | 下发配置 |
| 6 | 待下发（配置已变更） | 期望指纹 ≠ `config_fingerprint` | 橙 | 下发配置 |
| 7 | 运行异常 | `runtime_status=error` | 红 | 看 `last_error` |
| 8 | 未运行 | `runtime_status ∈ {stopped,unknown}` | 橙 | 启动服务 |
| 9 | 正常采集 | 以上均不命中 | 绿 | — |

只显示"最该处理的那一个"（如 agent 离线时不再纠缠配置漂移），其余作为原因下钻。

`install_status` 的合法取值是 `pending / success / failed / uninstalled / unknown`，没有 `running`
（`db/queries/mysql/monitor.sql:894`、`:904`、`:592`、`:977`），"进行中"只认 `pending`。

**数据态不参与单一结论状态**：上表 9 档全部由"配置态 + 运行态"决定，不含任何 ES 查询。
"已下发但无数据"是独立的第二个徽标（见 §2.4），因此列表里不出现、也不需要 N 次 ES 请求。

### 2.2 逻辑服务 × 日志定义：采集状态

| 状态 | 判定 | 建议动作 |
|---|---|---|
| 已关闭 | 有效开关链=false（服务总开关 / 服务级每日志开关，见架构 §6） | 需要时开启 |
| 配置不完整 | 无处理规则（**去模板日志定义上补**，服务侧不可覆盖，2026-09-19）/ 路径宏展不开 / 无实例 | 补配置 |
| 待下发 | 任一覆盖主机为 `never/drift` | 下发 |
| 已下发未运行 | 覆盖主机全 `synced`，但存在非 `running`（细因下钻主机态） | 启动/排错 |
| 采集中 | 覆盖主机全 `synced`+`running` | — |

同上：判定只到"配置态 + 运行态"，不含 ES 查询。**"有配置无写入"不是一种采集状态，而是数据态徽标**
（`有写入 / 无写入 / 未知`），只在详情页或体检时按需查询。

"历史遗留"（定义已删但流仍有数据）不属于本表：定义已删时这一行对象已不存在，
它是数据流层的孤儿状态，见 §2.3。

### 2.3 数据流：存储状态

`active`（当前档位、有写入）/ `idle`（有配置无写入）/ `historical`（旧档位，等 ILM）/
`orphan`（维度已删）/ `expiring`（即将到期）。

`orphan` 无需新造识别逻辑：`streamNameMatcher.resolveStreamName` 对已删除/改名的维度本就返回
`Recognized=false`（`internal/logcollect/datastream_status.go`），日志中心的水位 tab 已经在用这个信号，
只需把它接成显式状态与清理入口。

**`orphan` 只认"行已删/改名"，不认"已停用"**（2026-09-18 落地时修正）：识别候选集必须覆盖
**全部**服务与档位，不能按 `enabled` 过滤。原实现给候选查询加了 `WHERE s.enabled = TRUE`，
于是停用一个服务（或档位）会让它既有的流被判成"未识别"——既是误报，更危险的是 Phase 3 的
"孤儿整流删除入口"正是复用这个信号，等于把暂停采集的存量数据标成待清理对象，与 §0 的
「停止采集 ≠ 删除数据」直接冲突。落地改动：`ListServiceStreamDims` / `ListServiceStreamRows` /
`ListRetentionTierCodes` 三条识别路径的查询去掉 enabled 过滤（下发路径 `ListHostLogRenderEntries`
保持不变），守卫用例 `internal/logcollect/stream_recognition_guard_test.go`。

### 2.4 计算与展示原则

- 状态**实算不落库**（除 `config_fingerprint` / `last_applied_time` 这类不可推导的），避免存的状态过期。
- 后端返回 `{status, reasons[], actions[]}`；前端一个徽标 + tooltip 说明原因 + 明确按钮。
- **配置态**实时算：复用 `loadHostLogRenderInput + renderHostLogConfig` 生成期望指纹，与 `config_fingerprint` 比对。
  两个必须写死的口径：
  1. **期望指纹 = 片段指纹 + output 指纹**。现有 `renderHostLogConfig` 的 digest 只求和 fragments
     （`internal/logcollect/log_config_render.go:201`），必须把 `filebeatOutputParams`（ES 地址/账号/verify_tls）的结果一并纳入，
     否则只改集群地址时判不出漂移，下发还会被"指纹一致则跳过"挡住（§1 第 10 条）。
  2. **前缀取自默认启用的 Elasticsearch 集群**（`index_prefix`），与下发路径一致
     （`internal/logcollect/log_target_actions.go:644-658`）；集群或其前缀变更会让全部主机同时进入 `drift`，这是期望行为，但要在文案里说明原因。
- **列表的算力约束（500–1000 台基线）**：主机列表是**服务端分页、`page_size` 上限 30**
  （`internal/monitor/lists.go:122-130`，`HostOverview`），所以展示侧每页最多算 30 台，
  不存在"上千次查询"。真正的规模风险不在列表，而在批量"下发/安装"动作上，见 §8 规模基线。
- **"按配置状态筛选"与分页天然冲突（必须先定形态）**：状态是实时算出来的，SQL 无法按它筛选；
  要筛出"全部待下发"就必须把 **1000 台一次算完**。这个量级在「一次批量查询 + 内存纯函数渲染」
  的形态下可接受（渲染是 CPU-only，无 N 次 IO），但三种场景必须分开实现，都不能逐主机查库：
  - 展示：每页 ≤30 台，按页批量渲染；
  - 筛选/统计（"待下发 N 台"）：全量批量渲染一次——「批量下发待变更」本来也需要这一次全量计算；
  - 退路：若实测仍偏慢，就把 `expected_fingerprint + config_state` 落库、由配置变更事件或定时任务维护。
    但这会引入"存的状态会过期"，与本节第一条原则冲突，只在实测证明确有必要时采用。
- **数据态**涉及 ES 查询，是**独立于上述单一结论状态**的第二个徽标（`有写入 / 无写入 / 未知`）：
  列表默认不渲染（显示"未校验"），详情页或体检时按需查询，避免 N 次 ES 请求。
  因此"采集中"的准确文案是"已下发 · 运行中（数据未校验）"，而不是断言"采集中"。

## 3. 变更 → 影响矩阵（目标语义）

**变更 → 配置态 → 下发时主机动作 → 数据流后果 → 是否值得自动下发**的完整落地版见
[LOG_COLLECTION_ARCHITECTURE.md §8.9](../architecture/LOG_COLLECTION_ARCHITECTURE.md)（2026-09-19 定稿，
含每个动作的自动下发建议与判据）；本处不再维护第二份。

本文只保留**清理意图**这一列（Phase 3 尚未实现的部分）：

| 变更 | 数据清理（目标语义） |
|---|---|
| 新增/改路径/关采集 | 不清理 |
| 删除日志文件/日志定义 | 默认不删；可选按 `log_name` 清 |
| 改保留档位 | 旧流按原档位到期，不迁移，不提前删 |
| 删除逻辑服务/模板 | 流变孤儿，走显式孤儿流清理入口（不自动删） |

## 4. 数据流粒度决策：服务级 vs 日志级

现状流名 `<prefix>-<项目>-<业务>-<环境>-<服务>-<档位>`（服务级；档位在尾部以便 ILM 后缀匹配）。

- **按日志的保留档位现已能实现**：档位在流名里，`Tier` 按 `(服务×日志定义)` 的 log_setting 解析
  （`internal/logcollect/log_config_render.go:264` 的 `COALESCE(log_setting.retention_tier_id, service.log_retention_tier_id)`），同服务不同档位的日志本就分属不同流 → 日志级流对"保留"无增益，
  只对"清理速度/隔离"有增益。
- **规模估算**：全系统约几百个逻辑服务，90% 服务 2 条日志、少数 ~10 条 → 全面日志级流使流数 ×~2.8，
  达**千级**；每条流默认 1 分片，小分片是 ES 反模式（集群状态/堆/ILM 扫描变重）。
- **结论（待最终确认）**：
  - **方案 A（默认）**：服务级流 + `log_name` 精确删。零迁移、零解析重写、零小分片放大。
  - **方案 B（不推荐）**：全面日志级流。清理最简，但流数膨胀 + 流名解析（`streamNameMatcher`）重写 + 迁移面大。
  - **方案 C（按需）**：混合，仅对"单条超大且需频繁清理"的日志白名单单独建流；代价是双命名并存的解析兼容。

一句话：**"清理容易"主要靠"显式清理入口 + `log_name` 精确删 + ILM 兜底"达成，不需要动流粒度。**

## 5. 数据流生命周期与 ES 删除手段

- **活跃流**：当前档位、仍在写入。
- **历史档位流**：换档后停写，按各自档位 ILM 到期。
- **孤儿流**：服务/项目/模板已删或改名残留，无人引用。

| 手段 | 空间回收 | 粒度 | 适用 |
|---|---|---|---|
| ILM 到期（整索引删） | 立即 | 档位级 | **默认保留**，零操作 |
| `DELETE /_data_stream/<s>`（整流） | 立即 | 整服务 | 孤儿流/整服务废弃，强确认 |
| `_delete_by_query`(+`terms log_name`) | 延迟（等 merge） | 文档级 | 按日志文件精确清，异步、重 |

现有清理已用异步、保留流的
`_delete_by_query?wait_for_completion=false&conflicts=proceed&refresh=false`（`internal/logcollect/log_datastream_cleanup.go:102`）。

## 6. 清理粒度：一个服务多个日志文件

文档携带 `log_name`（filebeat 输入注入，= 日志定义 name，`internal/logcollect/log_config_render.go:154`），因此：

- 流模式：`<prefix>-<项目>-<业务>-<环境>-<服务>-*`（`*` 覆盖历史档位）。
- 过滤：`{"terms": {"log_name": [...]}}` + 可选时间窗。
- 粒度三档：整服务（不传 `log_name`）/ 按日志文件（`log_names`）/ 时间窗（`mode=hours|days`）。
  三档可组合；**默认范围按 §9 第 3 条**——必须显式选择，不默认全清。
- 配套接口：
  - `GET /monitor/log-datastreams/log-names/?service_id=`：对 `log_name` 聚合，返回
    `[{log_name, doc_count, 最早/最新时间}]`，列出多选框（含改名遗留的旧名）。
  - 扩展 `POST /monitor/log-datastreams/cleanup/`：body 增加可选 `log_names?: string[]`
    （仍只接受 service_id + 名字，不接受索引名，安全）。

## 7. 删除合理性

**不该把"删配置"与"删数据"绑定、默认自动删。**

- 配置可逆、数据不可逆；一次模板保存（全量替换、改名会被误判为删除）就触发不可逆删除，风险高。
- 日志是 append-only 的留档/排障数据，惯例是配置与数据解耦（删 dashboard 不删 metrics）。
- 破坏性操作应由显式动作发起，需具备：独立权限、影响预览、二次确认、审计、异步可观测。
- ES 按文档删除本身重且空间延迟回收，不适合挂在保存路径。

合理删除的要素：显式 / 可预期（预览）/ 可授权（权限分离）/ 边界清晰（误删配置时数据仍在）/
可异步观测 / 有 ILM 兜底。

**权限现状与目标（必须整体拆，不能只加一个 cleanup）**：现在整个 `/monitor` 路由组只挂
`middleware.RequirePermission("monitor:view")`（`internal/api/router/router.go:366`），也就是说
**只读权限的账号现在就能卸载 Filebeat、批量删除采集目标、清理数据流**。要做"可授权"这一要素，
就得把所有破坏性动作一起拆出来（至少：数据清理、目标删除、服务停止、配置下发），
只给清理加一个 `monitor:logs:cleanup` 解决不了 §7 自己要的"权限分离"。

## 8. 分期计划

**规模基线（硬约束）**：平台按 **500 台起、1000 台上限**的虚拟机规模设计。以下结论都以此为前提：

- **列表/详情侧不是瓶颈**：主机列表服务端分页、`page_size` 上限 30，配置态实时算每页 ≤30 台（§2.4）。
- **流数不随 VM 数增长**：data stream 数 = `逻辑服务 × 档位`（§4），1000 台影响的是"每主机的 inputs
  片段条数"（每实例一个 input），不是流数。因此 §4 的流粒度结论、ILM 与清理侧都不需要按 VM 数改造。
- **批量动作必须异步化**：当前批量下发串行阻塞在单个 HTTP 请求内、批量安装/重试无并发上限（§1 第 11 条）。
  1000 台下的目标形态是「入队 + 有界并发 + 进度可查 + 可续跑」。这也是 Phase 2「一键应用 N 台待下发」的
  前提——否则 N=1000 时该功能只会超时并留下部分下发的中间态。
- **限流设施已有，落地时按"执行者需要什么"分了两条队列**：原计划的"走 worker 模式复用
  prefetch"在实现时发现一个硬约束——日志采集的下发/安装要通过 **agent gRPC 会话**在主机上执行，
  而 agent 会话只存在于 api 进程（`Gateway` 是进程内的会话表），放到 worker 角色上
  `IsOnline` 恒为 false、作业会全部失败。因此新增 `rabbitmq.LogCollectRoute`
  （`autoadmin.logcollect.execute`）**由 api 角色消费**，计划任务那条队列仍归 worker 角色。
  `prefetch` 语义同时被修正：原 `Consume` 在 `for range deliveries` 里同步调 `Handle`，
  prefetch 只让消息被取到本地、处理仍严格串行，现已改为 prefetch 个 goroutine 并发处理。

**Phase 0 · 前置修复（不做则后续状态全是错的）—— ✅ 已完成（2026-09-18）**

实际落点：`internal/logcollect/log_target_actions.go`（下发入口统一）、
`log_config_render.go`（指纹纳入 outputIdentity）、`log_target_actions.go` 的
`buildFilebeatOutputParams` / `filebeatOutputIdentity`；随旁路删除的两条语句
`GetLogTargetDefaultCluster`、`MarkLogTargetApplied` 已从 `db/queries` 移除并重新生成
（derive/generate/facade）；回归用例 `TestRenderFingerprintCoversOutputIdentity`。
部署提示：指纹语义变更后，存量主机的 `config_fingerprint` 与期望值不一致，
**下一次下发会全量重推一遍**（一次性，之后恢复稳定）。

原定范围（两项均已落地）：

- **统一两条下发路径**：单条 `POST /log-targets/:id/apply/` 改为直接复用 `applyLogTargetConfigRow`
  （下发片段 + 写指纹），删掉只发 output 的重复实现；去掉它对 `runtime_status='running'` /
  `last_error=''` 的无条件改写——这两个字段只能由真实启停/探测结果维护（见 §1 第 9 条）。
- **期望指纹纳入 output**：把 `filebeatOutputParams` 的结果（ES 地址/账号/verify_tls）并入指纹，
  使默认集群变更也能判出漂移，并让"指纹一致则跳过"不再挡住主配置更新（见 §1 第 10 条）。
- 前置理由：Phase 1 的 `never/drift/synced` 完全建立在 `config_fingerprint` 之上。
  只要存在一个不写指纹的下发入口、或指纹漏掉 output，这套状态一上线就是错的。

**Phase 1 · 漂移可见 —— ✅ 已完成（2026-09-18）**

实际落点：`internal/logcollect/log_target_state.go`（`EvaluateLogConfigStates` 批量评估）、
`log_health.go`（"主机配置"层改为内容比对）、`db/queries` 的
`ListHostLogRenderInstances` / `ListHostLogRenderEntries`（批量取渲染输入，**取代了最后两条内联 SQL**）、
`internal/monitor/read_resources.go`（列表暴露配置态 + `config_state` 筛选）、
`fronted/src/views/monitor/log-collectors/index.vue`（配置状态列 + 筛选 + 「下发本页待变更」）；
主机表外壳与状态逻辑分别下沉到 `fronted/src/views/monitor/components/HostTargetPanel.vue` 与
`fronted/src/util/hostTargetTable.js`，与 Exporter 目标页共用（2026-09-18 从 `monitor/index.vue` 拆出）。
回归用例：`log_config_render_loader_test.go`（N+1 保护）、`log_target_state_test.go`、
`read_resources_state_filter_test.go`；内联 SQL 守卫改写为 `assets/inline_sql_guard_test.go`。

与原计划的差异（有意为之）：
- 原计划的「批量下发待变更」只落地到**当前页**（前端按钮 N = 本页 drift+never 台数）。
  跨全量的一键下发必须后端异步化，否则 N=1000 时只会超时——按 §8 规模基线挪到 Phase 2 与异步化一起做。
- 列表状态词汇增加了 `unknown`（期望配置算不出来，如没有启用的默认集群），
  并在响应里回传原因，不谎报"已同步"。

原定范围：

- 后端 `evaluateLogTargetConfigState`：实时渲染出期望指纹（片段 + output），与 `config_fingerprint`
  比对 → `synced/drift/never`（+ 简短 diff）。
- **批量实现**：按 §2.4 的三种场景分开实现——展示按页（≤30 台）批量渲染、筛选/统计全量批量渲染一次；
  禁止按主机循环查库。
- 采集目标列表加「配置状态」列/筛选；体检 `checkLogHostConfigs` 改为内容比对，去掉过时注释。
  注意体检扫的是**全部**纳管目标（`ListManagedLogTargetConfigs` 无分页），1000 台下同样要按主机批量渲染，
  且不要把它放进会自动刷新的 summary 路径。
- 「批量下发待变更」。

**Phase 2 · 下发闭环 —— ✅ 已完成（2026-09-18）**

实际落点：`internal/logcollect/log_batch_job.go`（执行器：入队/分片/有界并发/心跳/失联对账）、
`log_batch_api.go`（批量作业入口、进度查询、全量待下发汇总）、
`internal/messaging/rabbitmq/client.go`（`LogCollectRoute` 拓扑 + prefetch 真并发消费）、
`internal/api/server.go` 与 `internal/app/app.go`（api 角色消费采集队列）、
`db/migrations/*/000033_log_batch_job.*.sql`（两张表 + 查询 + 派生 + 门面）、
`fronted/src/views/monitor/log-collectors/index.vue`（全量待下发计数、一键下发、作业进度轮询弹窗，
进度逻辑落在 `components/HostTargetPanel.vue` 的宿主页里）、
`fronted/src/views/assets/application/components/ApplicationServiceDialog.vue`（改档位提示）。
回归用例：`log_batch_job_test.go`（终态 success/partial/failed、分片续跑、已结束作业的重投不做事、
认领抢不到时不推进、失联对账重投、外来 kind 拒绝、投递失败冒泡），
以及 `smoke_log_batch_test.go`（`MONITOR_SMOKE_DSN` 触发的真库冒烟：整套语句 + 对账链路，
自己造的行按 id 精确删除）。

原定范围（四项均落地，两处与计划的差异记在下面）：

- 放宽 `canApplyConfig`：`fronted` 侧已改为只要求 agent 在线 + Filebeat 已装，`runtime_status`
  只在提示里说明（"当前未运行，下发后会一并启动"）；下发流程本身会写主配置 + 片段并重启服务。
- 配置变更后提示"N 台主机待下发"，一键应用：`GET /log-targets/pending-summary/` 给全量口径，
  「一键下发全部待变更（N）」按钮用 `POST /log-targets/batch-jobs/`（省略 ids → 服务端实时算全量）。
- **批量动作异步化**：见上文"限流设施"一条与架构文档 §8.8。
- 改档位提示：在服务/日志定义的保留档位处提示"写入新流，旧流停写并按原档位保留到期、不迁移"，
  并提示需重新下发才生效。

与原计划的差异（有意为之）：

- 队列**由 api 角色消费**而不是 worker 角色（agent 会话只在 api 进程，见上文规模基线）。
- **批量启停与批量删除仍是同步接口**：单台只是一次 30 秒超时的 `systemctl` 调用或一条 DELETE，
  逐台串行的代价可接受；分钟级的「下发 / 安装」才作业化。这是刻意收窄的范围。
- 单台重试仍旧是"后台 goroutine + 看安装历史"（一次只派一台，不存在无上限扇出），
  只有批量安装才进作业；`纳管并立即安装` 的安装部分已改为建批量作业。

### 关联与状态的归属（2026-09-18 定案，Phase 2 之后）

- **实例 ↔ 逻辑服务的绑定只由服务侧维护**（服务编辑弹窗的成员列表 → `member_configs`，
  整组删后重建）。**不做实例侧"反向绑定"**：一个实例可同时属于多个应用下的服务，
  实例侧保存去改关联会和"服务侧整组重建"互相踩。实例弹窗里的 `application_service` 字段
  已从前端移除（后端本就不接收，此前是静默忽略的假绑定），详见 ASSET_CATALOG 的
  「逻辑服务 ↔ 部署实例关联」一节。
- **日志状态一律按逻辑服务归属**：水位视图的"已停用 / 未开启采集"标注取自逻辑服务行
  （`service_enabled` / `service_collection_enabled`），随每条流返回；
  主机级状态（agent 在线、Filebeat 运行、配置是否已下发）仍按主机归属——那本质是每台主机的事实
  （见 §2.1 与 §2.2 的分层）。

**Phase 3 · 清理闭环**

- ✅ **第一步已完成（2026-09-19）**：模板日志定义按 id 增量写（改/删/增），id 从此稳定——
  覆盖行不再因改模板失效，删定义不再撞外键，见 §1 第 8 条。**注意这只解决了配置侧**：
  "某条日志定义是改名了还是被删了"在**数据侧**仍无从判断（旧数据里的 `log_name` 只留了旧名字），
  所以下面的"疑似删除/改名"提示仍需要额外依据（例如按 `log_name` 聚合出"最近还在写但已不在
  期望配置里"的名字）。
- 扩展清理接口支持 `log_names` + 新增 `log-names` 聚合接口。
- 前端清理弹窗：服务 + 日志文件多选 + 时间窗 + 影响预览。
- 删除日志定义：先给"疑似删除/改名"提示，显式确认后清理。
- 水位视图（日志中心的水位 tab）：流清单（活跃/历史/孤儿，孤儿复用 `Recognized=false`）+ 孤儿整流删除入口。

## 9. 待确认（含建议默认）

1. 接受**方案 A 为默认**？（建议：是）
2. 删日志定义默认**不自动删数据**、只提示？（建议：是）
3. 清理的**默认范围**？（建议：**必须显式选择范围，不提供"默认全清"**；默认落在时间窗如"最近 30 天"，
   选"全部历史"要额外二次确认。默认全清与 §7 的"破坏性操作最小化"自相矛盾）
4. 破坏性动作的权限是否**整体**拆分？（建议：是。范围至少含 数据清理 / 目标删除 / 停止服务 / 配置下发；
   现状是 `/monitor` 组统一 `monitor:view`（`router.go:366`），只加 `monitor:logs:cleanup` 达不成权限分离）
5. 界面标注"关闭采集 = 数据保留至档位 X 天过期"？（建议：是）
6. 状态文案：中文短词（正常/待下发/离线…）还是带数值（"待下发 3 台"）？
   （建议：主机级短词 + 服务级带数值；数据态徽标列表默认显示"未校验"，点开才查）
7. 日志定义改名是否清旧数据？（建议：**不改名清数据**。为区分"改名"与"真删除"，模板日志行**必须加稳定 id**，
   这一步是 Phase 3 的前置而非可选项，见 §8）
8. 配置变更后**是否自动下发**，还是提示后人工一键应用？**已按建议落地：提示 + 人工一键，不自动。**
   500–1000 台规模下自动下发会把一次误配置瞬间放大到全网，且重启 Filebeat 的噪声无法收敛。
   **补充（2026-09-19 建议，未实现）**：该理由只对**跨服务/全网**的变更成立，不应对"单服务范围内
   可逆变更"一刀切。建议按动作分级自动化的白名单（判据与逐条结论见架构文档 §8.9）：
   - 第一批自动：**新增部署实例**（现状 = 每次扩容留一次人工下发，是漏采主要来源）、关闭采集、
     改路径/宏、改多行参数；
   - 第二批（需先有预检护栏）：开启采集、新增日志定义；
   - 保持人工：换规则标识/换集群、改档位、删除日志定义（前置：模板日志定义加稳定 id）、
     停用删除服务与模板、集群输出段、口令、安装卸载。
   - 落地护栏：改动在资产写路径提交成功后触发、复用批量作业队列 + 有界并发 + **时间窗合并**
     （避免反复重启 Filebeat）、渲染预检不通过则不自动（落回"待下发 + 原因"）、失败不回滚配置。
9. 批量动作的**有界并发批大小**？批量安装与批量下发是否分开限流？
   **已按建议落地：分开配置**——`LOG_BATCH_INSTALL_CONCURRENCY`（默认 20）与
   `LOG_BATCH_APPLY_CONCURRENCY`（默认 5，下发会重启 Filebeat 所以要小）；
   另加 `LOG_BATCH_PREFETCH`（同时在跑的作业数，默认 2）与 `LOG_BATCH_BUDGET`（单条消息的时间预算，
   默认 15 分钟）。"批大小"最终不按台数而按**时间预算**分片：台数分片会让单条消息的时长随
   单台耗时浮动，撞上 RabbitMQ 的 `consumer_timeout`(默认 30 分钟) 就会被服务端强断并重投。

## 10. 相关代码 / 文档索引

- 采集与存储全部在 `internal/logcollect/`：`log_config_render.go`（渲染与指纹，纯函数）、
  `log_target_actions.go`（安装/启停/下发/批量启停删除）、`log_batch_job.go`（批量作业执行器）、
  `log_batch_api.go`（批量作业入口与全量待下发汇总）、`log_target_state.go`（配置态评估）、
  `log_health.go`（体检）、`log_management.go`（索引模板 / ILM / `standardLogFields` / bootstrap）
- 队列：`internal/messaging/rabbitmq/client.go`（`JobRoute`=worker 角色 / `LogCollectRoute`=api 角色、
  prefetch 并发消费）、`internal/logcollect`+`internal/api/server.go`（消费者装配）
- 日志配置资源：`internal/logcollect/config_resources.go`、`config_resource_writers.go`
- ES 客户端与检索：`internal/logcollect/elasticsearch.go`、`elasticsearch_config.go`、`elasticsearch_pipeline.go`
- 清理：`internal/logcollect/log_datastream_cleanup.go`
- 存储水位：`internal/logcollect/datastream_status.go`、`internal/shared/logstream/name.go`
- 模板/日志定义：`internal/assets/template.go`、`db/schema/*/002_assets.sql`
- 内置 Playbook/unit：`internal/shared/filebeat`
- 主机列表与概览（含 filebeat 列）：`internal/monitor/read_resources.go`、`lists.go`
- 架构：`docs/architecture/LOG_COLLECTION_ARCHITECTURE.md`
