# 日志采集配置与数据生命周期（变更 / 状态 / 下发 / 清理）

> 状态：设计已定（剩余待确认项见 §9）。**§8 Phase 0 前置修复、Phase 1 漂移可见均已完成（2026-09-18）**，
> 可进入 Phase 2（下发闭环）——其中"批量动作异步化"是 §9 第 8/9 条的前提。
> 定稿后最终语义并入 [LOG_COLLECTION_ARCHITECTURE.md](../architecture/LOG_COLLECTION_ARCHITECTURE.md)。
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
| 3 | 配置变更本身不触发下发：只有「安装成功后自动下发一次」与「批量下发」两个入口，改完配置要人工点 | `internal/logcollect/log_target_actions.go:327-331`、`BatchApplyLogTargets` |
| 4 | Filebeat 停止就点不了「下发配置」 | `fronted/src/views/monitor/log-collectors/index.vue`（`canApplyConfig` 要求 `runtime_status==='running'`；2026-09-18 前该逻辑在 `monitor/index.vue`） |
| 5 | 数据流是服务级、档位在流名里 → 改档位必产生新流；旧流无管理 | `internal/shared/logstream/name.go:17`、`internal/logcollect/datastream_status.go:146` |
| 6 | 主机片段由 agent 全量托管、下发时清残留；ES 数据不随之清理 | `dj_agent/internal/executor/builtin_actions.go:183` |
| 7 | 清理只有按服务，不支持按日志文件；孤儿流只以 `Recognized=false` 隐式呈现，无显式状态与清理入口 | `internal/logcollect/log_datastream_cleanup.go` |
| 8 | 模板保存是全量替换日志定义，且不带稳定 id → 改名与删除无法区分 | `internal/assets/template.go:322-326` |
| 9 | ✅**已修复**（Phase 0）原为：**两条下发路径语义不一致**：单条 `POST /log-targets/:id/apply/` 只调 `configure_filebeat_output`（只写 `filebeat.yml`），不下发 inputs.d 片段、不写 `config_fingerprint`，却把 `runtime_status` 置 running、清空 `last_error`；批量/安装后自动走的是 `applyLogTargetConfigRow` 全流程 | `internal/logcollect/log_target_actions.go:442-487` vs `:633-708`；agent 侧 `dj_agent/internal/executor/builtin_actions.go:143` |
| 10 | ✅**已修复**（Phase 0）原为：**指纹只覆盖 inputs.d 片段，不含 output**（ES 地址/账号/TLS）：只改默认集群地址时指纹不变 → 命中"指纹一致则跳过"，`filebeat.yml` 保持旧值（与架构文档 §8.3"下发一定会写主配置"的承诺冲突） | `internal/logcollect/log_config_render.go:201`、`log_target_actions.go:669` |
| 11 | **批量动作在 1000 台规模下不成立**：批量下发在单个 HTTP 请求内**串行**遍历目标、每台 2 次 agent gRPC（超时 60s+120s），无进度、无断点；批量安装/重试对每台 `go func()` 内联跑 ansible，**无并发上限**且绕过平台 worker 队列 | `internal/logcollect/log_target_actions.go:556-582`、`:278-281`、`internal/automation/runtime.go:1207` |

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
| 已关闭 | 有效开关链=false（服务/定义/覆盖） | 需要时开启 |
| 配置不完整 | 无处理规则 / 路径宏展不开 / 无实例 | 补配置 |
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
`Recognized=false`（`internal/logcollect/datastream_status.go`），存储水位页已经在用这个信号，
只需把它接成显式状态与清理入口。

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

| 变更 | 主机片段 | 数据流 | 必须下发 | 数据清理 |
|---|---|---|---|---|
| 新增日志文件 | +片段 | 复用服务流 | 是 | 无 |
| 删除日志文件 | -片段 | 数据仍在服务流 | 是 | 默认不删；可选按 `log_name` 清 |
| 改路径/宏 | 改片段 | 不变 | 是 | 无 |
| 关闭采集（服务/定义/覆盖） | -片段 | 停写，ILM 到期 | 是 | 不删 |
| 改保留档位 | index 改 | 新流；旧流停写 | 是 | 旧流按原档位到期，不迁移 |
| 卸载/停 Filebeat | 不可用 | 停写 | 先恢复安装/启动 | 保留 |
| 删除逻辑服务/模板 | -片段 | 变孤儿流 | 是 | 显式清孤儿流 |

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
- **限流设施已有，只是没走**：平台自带 `worker` 模式 + RabbitMQ `WorkerPrefetch`
  （`internal/app/app.go:259-272`），但日志采集的安装/下发路径目前内联在 API 进程执行
  （`RunJobByID`，`internal/automation/runtime.go:1207`），应改为走队列复用这套限流。

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

**Phase 2 · 下发闭环**

- 放宽 `canApplyFilebeatConfig`：仅需 agent 在线 + Filebeat 已装；`runtime_status` 只作提示；
  apply 顺带启动服务。（单条按钮此时已等价于全流程，放宽门槛才安全；否则只是把更多主机推到残缺路径。）
- 配置变更后提示"N 台主机待下发"，一键应用（是否自动下发见 §9）。
- **批量动作异步化（规模前提，见上文规模基线）**：批量下发 / 一键应用改为「入队 + 有界并发 + 进度可查 +
  可续跑」，走 worker 队列与 prefetch 限流；安装/重试不再在 API 进程内联无上限起 goroutine。
  前端按作业进度刷新，不再等一个可能跑几十分钟的同步响应。
- 改档位提示："写入新流 `<新>`，旧流 `<旧>` 保留至原档位到期，不迁移"。

**Phase 3 · 清理闭环**

- **第一步**：给模板日志定义行加稳定 id。这是"区分改名与真删除"的唯一前提，
  没有它后面的"疑似删除/改名"提示无从判断（见 §9 第 7 条）。
- 扩展清理接口支持 `log_names` + 新增 `log-names` 聚合接口。
- 前端清理弹窗：服务 + 日志文件多选 + 时间窗 + 影响预览。
- 删除日志定义：先给"疑似删除/改名"提示，显式确认后清理。
- 存储水位：流清单（活跃/历史/孤儿，孤儿复用 `Recognized=false`）+ 孤儿整流删除入口。

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
8. 配置变更后**是否自动下发**，还是提示后人工一键应用？（建议：提示 + 人工一键，不自动。
   500–1000 台规模下自动下发会把一次误配置瞬间放大到全网，且重启 Filebeat 的噪声无法收敛）
9. 批量动作的**有界并发批大小**（如每批 50 台）？批量安装与批量下发是否分开限流？
   （建议：分开配置，安装走 worker 队列的 prefetch，下发用独立的较小并发，避免 1000 台同时重启 Filebeat）

## 10. 相关代码 / 文档索引

- 采集与存储全部在 `internal/logcollect/`：`log_config_render.go`（渲染与指纹，纯函数）、
  `log_target_actions.go`（安装/启停/下发/批量）、`log_health.go`（体检）、
  `log_management.go`（索引模板 / ILM / `standardLogFields` / bootstrap）
- 日志配置资源：`internal/logcollect/config_resources.go`、`config_resource_writers.go`
- ES 客户端与检索：`internal/logcollect/elasticsearch.go`、`elasticsearch_config.go`、`elasticsearch_pipeline.go`
- 清理：`internal/logcollect/log_datastream_cleanup.go`
- 存储水位：`internal/logcollect/datastream_status.go`、`internal/shared/logstream/name.go`
- 模板/日志定义：`internal/assets/template.go`、`db/schema/*/002_assets.sql`
- 内置 Playbook/unit：`internal/shared/filebeat`
- 主机列表与概览（含 filebeat 列）：`internal/monitor/read_resources.go`、`lists.go`
- 架构：`docs/architecture/LOG_COLLECTION_ARCHITECTURE.md`
