# 日志采集配置与数据生命周期（变更 / 状态 / 下发 / 清理）

> 状态：设计梳理中（核心决策已定，剩余项见 §9）。定稿后最终语义并入
> [LOG_COLLECTION_ARCHITECTURE.md](../architecture/LOG_COLLECTION_ARCHITECTURE.md)。
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
| 1 | 期望态与实际态未比对：体检只看 `config_fingerprint` 是否非空，不比对内容 | `internal/monitor/log_health.go:318-340`（注释仍写"后端没有期望指纹生成器"） |
| 2 | 期望指纹能力其实已存在，只用于"下发时跳过" | `internal/monitor/log_config_render.go`（`loadHostLogRenderInput` + `renderHostLogConfig`） |
| 3 | 下发纯手动：仅安装成功自动下发一次，其余要手点 | `internal/monitor/log_target_actions.go:328` |
| 4 | Filebeat 停止就点不了「下发配置」 | `fronted/src/views/monitor/index.vue:2462`（`canApplyFilebeatConfig` 要求 `runtime_status==='running'`） |
| 5 | 数据流是服务级、档位在流名里 → 改档位必产生新流；旧流无管理 | `internal/shared/logstream/name.go:17`、`internal/monitor/datastream_status.go:146` |
| 6 | 主机片段由 agent 全量托管、下发时清残留；ES 数据不随之清理 | `dj_agent/internal/executor/builtin_actions.go:183` |
| 7 | 清理只有按服务，不支持按日志文件；无孤儿流识别 | `internal/monitor/log_datastream_cleanup.go` |
| 8 | 模板保存是全量替换日志定义，且不带稳定 id → 改名与删除无法区分 | `internal/assets/template.go:322-326` |

## 2. 状态模型（核心）

状态拆成三层正交维度，避免用一个字段硬塞：

- **配置态（Desired vs Applied）**：`never` / `drift` / `synced`
- **运行态（agent + filebeat）**：`agent_offline` / `not_installed` / `task_pending` / `install_failed` / `stopped` / `error` / `running`
- **数据态（ES 是否有写入）**：`flowing` / `no_data` / `unknown`（按需查 ES）

再对每类对象给出**单一结论状态**（按优先级取第一个命中）。

### 2.1 主机采集目标（`monitor_log_collection_target`）

| 优先级 | 状态 | 判定 | 颜色 | 建议动作 |
|---|---|---|---|---|
| 1 | Agent 离线 | `gateway.IsOnline=false` | 灰 | 检查 dj-agent |
| 2 | 未安装 Filebeat | `agent_installed=false` 且无进行中任务 | 灰 | 离线安装 |
| 3 | 任务进行中 | `install_status ∈ {pending,running}` | 蓝 | 等待/取消 |
| 4 | 安装失败 | `install_status=failed` | 红 | 看历史/重试 |
| 5 | 待下发（从未） | `config_fingerprint=''` | 橙 | 下发配置 |
| 6 | 待下发（配置已变更） | 期望指纹 ≠ `config_fingerprint` | 橙 | 下发配置 |
| 7 | 运行异常 | `runtime_status=error` | 红 | 看 `last_error` |
| 8 | 未运行 | `runtime_status ∈ {stopped,unknown}` | 橙 | 启动服务 |
| 9 | 已下发但无数据（可选） | synced+running 但近 N 分钟无写入 | 黄 | 查体检/路径 |
| 10 | 正常采集 | 以上均不命中 | 绿 | — |

只显示"最该处理的那一个"（如 agent 离线时不再纠缠配置漂移），其余作为原因下钻。

### 2.2 逻辑服务 × 日志定义：采集状态

| 状态 | 判定 | 建议动作 |
|---|---|---|
| 已关闭 | 有效开关链=false（服务/定义/覆盖） | 需要时开启 |
| 配置不完整 | 无处理规则 / 路径宏展不开 / 无实例 | 补配置 |
| 待下发 | 任一覆盖主机为 `never/drift` | 下发 |
| 采集中 | 覆盖主机全 `synced`+`running` 且近期有写入 | — |
| 有配置无写入 | `synced`+`running` 但无数据 | 查路径/权限 |
| 历史遗留 | 定义已删但流仍有数据 | 清理/等 ILM |

### 2.3 数据流：存储状态

`active`（当前档位、有写入）/ `idle`（有配置无写入）/ `historical`（旧档位，等 ILM）/
`orphan`（维度已删）/ `expiring`（即将到期）。

### 2.4 计算与展示原则

- 状态**实算不落库**（除 `config_fingerprint` / `last_applied_time` 这类不可推导的），避免存的状态过期。
- 后端返回 `{status, reasons[], actions[]}`；前端一个徽标 + tooltip 说明原因 + 明确按钮。
- **配置态**实时算：复用 `loadHostLogRenderInput + renderHostLogConfig` 生成期望指纹，与 `config_fingerprint` 比对。
- **数据态**涉及 ES 查询，列为次要/按需（详情或体检时查），列表默认不查，避免 N 次 ES 请求。

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
  （`log_config_render.go:260`），同服务不同档位的日志本就分属不同流 → 日志级流对"保留"无增益，
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
`_delete_by_query?wait_for_completion=false&conflicts=proceed&refresh=false`（`log_datastream_cleanup.go:102`）。

## 6. 清理粒度：一个服务多个日志文件

文档携带 `log_name`（filebeat 输入注入，= 日志定义 name，`log_config_render.go:154`），因此：

- 流模式：`<prefix>-<项目>-<业务>-<环境>-<服务>-*`（`*` 覆盖历史档位）。
- 过滤：`{"terms": {"log_name": [...]}}` + 可选时间窗。
- 粒度三档：整服务（不传 `log_name`）/ 按日志文件（`log_names`）/ 时间窗（`mode=hours|days`）。
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

## 8. 分期计划

**Phase 1 · 漂移可见**

- 后端 `evaluateLogTargetConfigState`：实时渲染出期望指纹，与 `config_fingerprint` 比对 →
  `synced/drift/never`（+ 简短 diff）。
- 采集目标列表加「配置状态」列/筛选；体检 `checkLogHostConfigs` 改为内容比对，去掉过时注释。
- 「批量下发待变更」。

**Phase 2 · 下发闭环**

- 放宽 `canApplyFilebeatConfig`：仅需 agent 在线 + Filebeat 已装；`runtime_status` 只作提示；
  apply 顺带启动服务。
- 配置变更后提示"N 台主机待下发"，一键应用（是否自动下发见 §9）。
- 改档位提示："写入新流 `<新>`，旧流 `<旧>` 保留至原档位到期，不迁移"。

**Phase 3 · 清理闭环**

- 扩展清理接口支持 `log_names` + 新增 `log-names` 聚合接口。
- 前端清理弹窗：服务 + 日志文件多选 + 时间窗 + 影响预览。
- 删除日志定义：先给"疑似删除/改名"提示，显式确认后清理（依赖稳定 id，见 §9）。
- 存储水位：流清单（活跃/历史/孤儿）+ 孤儿整流删除入口。

## 9. 待确认（含建议默认）

1. 接受**方案 A 为默认**？（建议：是）
2. 删日志定义默认**不自动删数据**、只提示？（建议：是）
3. 清理默认"该日志文件全部历史"还是"保留最近 N 天"？（建议：全部历史 + 可选时间窗）
4. 清理权限是否独立 `monitor:logs:cleanup`？（建议：是）
5. 界面标注"关闭采集 = 数据保留至档位 X 天过期"？（建议：是）
6. 状态文案：中文短词（正常/待下发/离线…）还是带数值（"待下发 3 台"）？
   （建议：主机级短词 + 服务级带数值）
7. 日志定义改名是否清旧数据？（决定是否需要给模板日志行加稳定 id，以区分"改名"与"真删除"）

## 10. 相关代码 / 文档索引

- 渲染与字段：`internal/monitor/log_config_render.go`、`internal/monitor/log_management.go`
  （`standardLogFields` / ILM）
- 下发与指纹：`internal/monitor/log_target_actions.go`
- 体检：`internal/monitor/log_health.go`
- 清理：`internal/monitor/log_datastream_cleanup.go`
- 存储水位：`internal/monitor/datastream_status.go`、`internal/shared/logstream/name.go`
- 模板/日志定义：`internal/assets/template.go`、`db/schema/*/002_assets.sql`
- 架构：`docs/architecture/LOG_COLLECTION_ARCHITECTURE.md`
