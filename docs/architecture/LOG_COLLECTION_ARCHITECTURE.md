# djadmin 日志采集架构文档

Elasticsearch + Filebeat 日志采集与分析能力的设计文档。

> **迁移进行中**：
> - 搜索后端已从 OpenSearch 迁到 **Elasticsearch 8.x**：保留策略用 ILM（`_ilm/policy`）替代
>   ISM，每档位索引模板绑定 `index.lifecycle.name`，映射用 `flattened` 替代 `flat_object`，
>   集群表更名为 `monitor_elasticsearch_cluster`。本文主体已按 ES 8 更新。
> - 采集器仍在 **Filebeat**，将迁到 Filebeat（Filebeat 配置渲染已落地，agent/安装链路待做），
>   进度见 [../plans/LOG_COLLECTOR_MIGRATION.md](../plans/LOG_COLLECTOR_MIGRATION.md)。

---

## 1. 目标

把各业务主机上的应用日志集中收集到 Elasticsearch，并在 djadmin 内提供检索与错误分析，
不需要用户登录主机 `tail` 日志，也不需要跳转到 Elasticsearch Dashboards 才能看到错误分布。

核心能力：

- 在逻辑服务上一键开启/关闭日志收集
- 一条日志处理规则统一维护 Filebeat 发送前处理与 Elasticsearch Ingest 字段解析
- 日志定义只关联一条处理规则，规则可由同格式的应用日志复用
- 不输入关键词即可看到「出现了哪些错误、各多少次、集中在哪台机器」
- 自动识别新增错误与突增错误

---

## 2. 整体架构

```
业务主机（每台）                   日志服务器                    djadmin
┌──────────────────┐          ┌──────────────────┐        ┌──────────────┐
│ 应用日志文件      │          │ Elasticsearch       │        │ 配置下发      │
│      ↓           │          │  ├ ingest pipeline│◀──────│ 处理规则管理   │
│ Filebeat       │─────────▶│  ├ index template │  REST  │ 聚合查询      │
│  ├ tail          │  HTTPS   │  └ ILM policy     │        │ 日志洞察页面   │
│  ├ multiline     │   9200   │                  │        └──────┬───────┘
│  └ record_modifier│          │ Dashboards :5601 │               │
└────────▲─────────┘          └──────────────────┘               │
         │                                                        │
         │ 写 inputs.d 片段 + 触发热重载                            │
         └────────────────── dj-agent ◀───────────────────────────┘
                              gRPC
```

三条链路互不耦合：

| 链路 | 方向 | 用途 |
|---|---|---|
| Filebeat → Elasticsearch | 主机 → 日志服务器 | 日志数据写入 |
| djadmin → Elasticsearch | 控制端 → 日志服务器 | 管理 pipeline、执行聚合查询 |
| djadmin → dj-agent → 主机 | 控制端 → 主机 | 下发采集配置、触发热重载 |

---

## 3. 采集器选型

**采集器使用 Filebeat，不使用 Filebeat。**

Filebeat 从 7.14 起在 elasticsearch output 中加入服务端版本校验，检测到对端不是
Elasticsearch 会直接拒绝连接，返回 `400 Bad Request`。Elasticsearch 由 ES 7.10 fork
而来，会被该校验拦截。可用的 Filebeat 只有 7.12.1 OSS，停留在 2021 年且不再有安全更新。

| 采集器 | Elasticsearch 支持 | 单实例内存 | 结论 |
|---|---|---|---|
| Filebeat | 原生 `elasticsearch` output | 5-15 MB | 采用 |
| Vector | `elasticsearch` sink 兼容 | 30-60 MB | 备选 |
| Filebeatd | 需插件 | 60-100 MB | 过重 |
| Logstash OSS | 官方插件 | 500 MB+ | 仅适合中转层 |
| Filebeat 7.12.1 | 版本校验前的最后一版 | 60-100 MB | 不采用 |

多行边界由日志处理规则中的首行正则、续行正则和合并超时定义。平台不提供 Java、Python、
Go 等语言专用分支；Java 堆栈、普通缩进续行和其他多行格式都使用同一套通用 regex multiline
机制。

---

## 4. 索引设计

### 4.1 索引按「项目 + 环境 + 业务系统 + 逻辑服务 + 保留档位」切分

```
autoadmin-<project.code>-<environment.code>-<business_system.code>-<service.code>-<tier>

autoadmin-tib-prod-esb-tomcat-svc-hot
autoadmin-tib-prod-esb-tomcat-svc-std
autoadmin-nkg-test-esb-gateway-std
```

- 档位必须在尾部（ILM 的 `index.lifecycle.name` 靠 `autoadmin-*-<tier>` 后缀挂载），可含连字符。
- 统一构造入口：Go `monitor.LogDataStreamName(prefix, project, env, bizsys, service, tier)`，
  禁止各自拼接。
- 服务、实例、主机、日志类型**同时作为字段存储**（`service` 等字段保留，检索过滤仍靠字段）。
- **旧命名兼容**：`autoadmin-<project>-<environment>-<business_system>-<tier>`（无服务段）为
  过渡期遗留流，ILM 按保留期自然删除，不做 reindex；存储水位页对两种命名都可见，
  旧流在"未识别"判定之外正常聚合展示。
- 编码（服务/档位）**可含连字符**，流名解析禁止按 `-` 盲切，必须用数据库维度码做
  前缀匹配（见 9.0）。

### 4.2 为什么需要保留档位

同一业务内部的日志量差异就很大：接入层服务每天几十 GB，定时任务每天几 MB。
ILM 是**索引级**的，同一索引内无法按 `service` 区分保留期，统一 30 天会撞爆磁盘，
统一 7 天又丢失了小服务的历史数据。

| 档位 | 保留 | 适用 |
|---|---|---|
| `hot` | 7 天 | 高频服务，如网关、接入层 |
| `std` | 30 天 | 默认 |
| `cold` | 90 天 | 量小但需长期保留，如审计、对账 |

档位配置在**逻辑服务**上（`ApplicationService.log_retention_tier`），不配则为 `std`。
服务级默认档位还可在**模板日志表格上方**的"默认保留档位"下拉单独覆盖到某条日志
（`assets_application_service_log_setting.retention_tier_id`，null 表示继承服务默认）。

**保存链路（Go 版）**：前端逻辑服务表单提交 `log_retention_tier`（档位 ID，可为 null）
→ `assets.SaveApplicationService` 在 INSERT/UPDATE 中写入
`assets_application_service.log_retention_tier_id`；GET 返回同名字段回显。档位 ID
必须指向 `monitor_log_retention_tier` 中存在的记录，前端下拉只列 enabled 档位。

**模板日志回显（编辑弹窗）**：`GET /assets/application-services/:id/log-config/`
（Go `assets.ListServiceTemplateLogs`）除覆盖值外还返回 `resolved_path` 与
`data_stream`——`resolved_path` 用服务级 `macro_values` 替换 `${VAR}`（实例级宏因
逐实例而异不展开，未定义的宏保留原样）；`data_stream` 用 `shared/logstream.Name`
生成（`autoadmin-<项目>-<业务系统>-<环境>-<服务>-<有效档位>`），有效档位取值顺序为
日志覆盖档位 → 服务默认档位 → `is_default` 档位 → `std`，与 Filebeat 下发的
Index 命名（`monitor.LogDataStreamName`，内部委托同一 shared 实现）完全一致。
同一次响应里还带出格式认证状态（`format_state` / `format_fingerprint` /
`format_verified_*`，见 §4.8）。每行还带 **`tier_code`**（这条日志**当前生效**的档位编码，与 `data_stream` 尾段一致；日志中心页
用它区分当前档位与历史档位流）与 **`service_code`**（本服务的编码，不是这条日志的）：
日志中心页要用它按服务取水位数据（见 §9.5），从 `data_stream` 里反解既绕又与流名段序耦合。

**格式认证**：`POST /assets/application-services/:id/log-config/verify/`，
body `{"log_definition_id":<必填>,"source":"instance|sample_log|waiver","deployment_id":<实例 id>}`
（`source` 缺省为 `instance` 时必填 `deployment_id`）。通过才写库，不通过返回 **200 + `passed=false` +
`missing_fields`**（"没通过"是业务结果，不是接口错误）；取不到样例一律报错，不返回空缺失。

### 4.3 为什么不直接按服务建索引

关键差别是维度的**增长性**：

```
档位维度   固定 3 个，不随业务扩张
服务维度   无界，每增一个服务就多一批索引
```

按服务建索引，以 8 业务 × 4 服务 × 2 环境 × 保留 30 天计算约 1900 个索引，且大多数是低流量
小索引，每个仅几 MB 却占满一个分片。小分片问题比大分片更消耗集群资源。

加档位维度的组合数上限是 `业务 × 环境 × 3`，且**空组合不会创建索引**。多数业务只用 `std`，
`hot` 仅少数业务具备，`cold` 更少，实际索引数约在 20 个上下且长期稳定。

档位**不要超过三个**。档位过多等价于按服务拆分，失去意义。

环境进索引名是因为保留策略不同必须物理隔离；业务系统进索引名是因为检索和权限隔离
几乎总是先限定业务。

**例外**：单个服务量大到影响同档其他服务的 rollover 节奏时，才为其单独开索引。
这种情况应为个位数。

### 4.4 用 rollover 代替按天建索引

按天切分对低流量业务是浪费。使用 data stream + ILM，按大小或时间自动滚动：

```json
PUT _index_template/autoadmin-template
{
  "index_patterns": ["autoadmin-*"],
  "data_stream": {},
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 0,
      "index.refresh_interval": "10s",
      "index.mapping.total_fields.limit": 2000,
      "index.final_pipeline": "autoadmin-mapping-guard"
    }
  }
}
```

`index.final_pipeline` 是平台级必备字段闸门（见 §4.7 与 §5.4）：它以 final 阶段运行，
在规则自己的 pipeline（由 Filebeat 片段的 `pipeline:` 指定）之后判定必备字段齐不齐——
`tag`（默认）模式打 `mapping_violation` 标记、**不丢也不排除**，`drop` 模式直接丢弃（`LOG_MAPPING_GUARD_MODE`，非默认）。
bootstrap 会先 PUT 这个 pipeline 再 PUT 引用它的模板。

`min_primary_shard_size` 与 `min_index_age` 同时配置，先满足哪个就滚动，高低流量都能自适应。

> 模板名固定为 `<index_prefix>-template`（缺省 prefix 为 `autoadmin`）。bootstrap 的 PUT 与链路体检的 GET
> 必须用同一个名字——早期 Go 版漏了 `-template` 后缀，去查不存在的 `logs`，表现为"`autoadmin-template`
> 不存在"（2026-09-17 修复，bootstrap 同时清理历史错名模板）。

### 4.5 ILM 按档位配置

三条 policy，通过 `index.lifecycle.name` 的索引名后缀自动挂载，新建业务不需手工配置：

```json
{
  "policy": {
    "default_state": "hot",
    "states": [
      { "name": "hot",
        "actions": [{ "rollover": { "min_primary_shard_size": "30gb", "min_index_age": "1d" }}],
        "transitions": [{ "state_name": "delete", "conditions": { "min_index_age": "7d" }}] },
      { "name": "delete", "actions": [{ "delete": {} }] }
    ],
    "index.lifecycle.name": [{ "index_patterns": ["autoadmin-*-hot"], "priority": 100 }]
  }
}
```

各档 rollover 阈值需区分：

| 档位 | `min_index_age` | 原因 |
|---|---|---|
| `hot` | 1d | 量大，需更激进地滚动 |
| `std` | 7d | 默认 |
| `cold` | 30d | 量小，放宽以避免小索引 |

### 4.6 容量可按档位反推

```
hot   30GB/天 ×  7 天 = 210 GB
std    5GB/天 × 30 天 = 150 GB
cold 0.1GB/天 × 90 天 =   9 GB
```

哪个档超出预算就调哪个档的天数，不影响其他服务。

### 4.7 字段设计

同一索引内混合多种应用的日志，若每种应用解析出的字段都独立建 mapping，字段数会持续膨胀。

**索引模板里实际声明的字段（= 唯一权威清单，与 `log_management.go` 的 `standardLogFields` 一致）**
共 16 个，分三类：

| 字段 | 类型 | 谁保证它存在 |
|---|---|---|
| `@timestamp` | date | Filebeat filestream（平台；值是否被 pipeline 的 `date` processor 覆盖成日志时间另说） |
| `message` | text | Filebeat filestream（原始行） |
| `project`、`business_system`、`environment`、`service`、`application`、`instance`、`host_ip`、`log_name`、`log_path` | keyword | 下发片段里的 `fields_under_root` 注入（`log_config_render.go`；`host_ip` 在主机没采到 IP 时写空串，保证键存在） |
| `app_fields` | flattened | 平台 guard 兜空对象（业务字段一律写进这里，见下） |
| **`log_level`、`log_message`、`error_fingerprint`** | keyword / text / keyword | **必须由处理规则的 pipeline 产出**（`requiredProcessingRuleOutputs`，见 §5.2、§5.4 与 §8.4 的 mapping-guard） |
| `mapping_violation` | boolean | 平台标记字段（不是规则产出要求）：guard 在 `tag` 模式下给必备字段不齐的文档打标。**检索不做默认排除**（2026-09-19 定：guard 只当"认证失效的发现器"，日志一条都不能看不见），标记供巡检/告警用 |

> **2026-09-19 的字段变更**：删掉 `log_time`（全仓库只有模板声明它，Go/前端/文档都没有读者——
> 检索按 `@timestamp` 排序、时间归一由 pipeline 的 `date` processor 负责），加 `mapping_violation`
> （平台标记，同上是唯一读者）。删列对已有 backing index 无影响（ES mapping 只增不减），
> 只影响新滚动出来的索引；`dynamic:false` 下残留的写入会被静默丢弃。

**业务特有字段**：一律写进 `app_fields.<字段名>`（`flattened` 类型）。`error_message`、
`error_template`、`stack_trace`、`exception_*` 这类**不在**索引模板里，属于业务附加字段，
不要当固定字段用——写进顶层会被 `dynamic:false` 静默丢弃（§5.4 的 `schema_violations` 就是查这个）。

> ⚠️ **`extra_fields` 目前不影响任何东西**（2026-09-19 核实）：片段渲染
> （`log_config_render.go`）与渲染查询（`ListHostLogRenderEntries`）都没有读
> `assets_application_log_definition.extra_fields`，因此改模板里的"附加字段"既不会改主机片段、
> 也不会注入 `labels_*`（表格 §8.9 未收录该动作，正是因为它不改期望指纹）。要用它得先在渲染里
> 注入字段——而注意字段名冲突：`fields_under_root: true` 下与固定字段重名会互相覆盖。
> 当前要携带业务维度，用处理规则的 `pipeline_body` 在 ingest 阶段写 `app_fields.<字段名>`。

**"必备字段"是不可协商的一组**：`log_level`、`log_message`、`error_fingerprint` 少一个都不行
（缺了不报错、只是静默失效，所以必须显式拦截）——三处共用同一份定义（`requiredProcessingRuleOutputs`）：
规则保存时的静态校验、调试页的 `missing_fields`、索引模板挂的 `<prefix>-mapping-guard`（写入时兜底）。

### 4.8 日志格式认证（一次认证 + 指纹失效，2026-09-19 定，同日落地）

"少一个字段都不行"落地成**一次性认证**，而不是每次写入都判：新增服务/开启采集时**抽样校验一次**
这条日志的格式能否被模板上的规则解析出必备字段；**通过后不再持续检查**，只在**格式指纹变化**时
要求重新认证。这样"格式对不对"这件事在交付时回答一次，运行期不做拦截。

**认证指纹只由库里的配置算出**（`internal/assets/log_format_fingerprint.go`），不依赖主机、不查 ES，
所以"认证是否过期"是纯读库比对（`ListServiceTemplateLogs` 返回的 `format_state`：
`unverified` / `verified` / `needs_recheck`）：

| 变化 | 会不会改格式 | 能自动失效吗 |
|---|---|---|
| 换模板 / 模板增删日志定义 / 改名 / 改路径 | 会 | ✅ 指纹含 `ld.id` / `ld.name` / `ld.path_pattern` |
| 改规则（`pipeline_body`、多行参数、首行正则） | 会 | ✅ 指纹含 `rule.update_time`（规则保存一定会更新它；代价是"只改说明"也会要求重新认证一次，偏保守） |
| 换挂另一条规则 | 会 | ✅ 指纹含 `ld.processing_rule_id` |
| 应用 / 中间件版本升级 | 会 | ✅ 指纹含 `s.application_version_id`（**每次版本升级后需要重新认证一次**，这是"改格式"里唯一无法事前断言的场景） |
| 改服务级宏 `macro_values` | 会（换了另一个文件在采） | ✅ 指纹含它 |
| 改实例级 `runtime_variables` | 会 | ❌ 逐实例而异、无法在服务级定义 → 需人工重新认证 |

**分层**：编排与落库在 `assets`（`internal/assets/log_format_verify.go`），"取样例 + 跑 ES"
在 `logcollect`（`internal/logcollect/log_format_verify.go`，经 `assets.LogFormatVerifier`
接口注入）。这样分的唯一原因是依赖方向——`logcollect` 已经依赖 `assets`（凭证解密、主机读取），
反向依赖会成环；接口定义在消费方 `assets` 一侧，注入点在 `router.NewWithGateway`。

**认证依据**（写进 `assets_application_service_log_setting` 的 `format_verified_*` 四列）：

| 依据 | 样例从哪来 | 关键实现 |
|---|---|---|
| `instance` | 部署实例所在主机上的真实日志文件 | `GetLogVerifyInstanceContext` 取 host/宏 → 与采集下发同一套 `resolveMacros` 展开路径（含未展开宏即报错）→ `StatFile` + `ReadFileChunk` 反向读尾部 1MiB 窗口（`ReadFileChunk` 一次只回一个 chunk，不能读到 EOF）→ 丢掉窗口起点的残行 |
| `sample_log` | 解析规则里保存的样例日志 | 规则未配样例即报错，不做静默回退 |
| `waiver` | 无（人工确认豁免） | 不碰 agent/ES，仍记当前指纹 + 操作人（取自登录态，不接受前端自报） |

样例文本还原成 pipeline 输入的口径与规则调试页 `buildRawDocs` 一致（默认逐行成记录；开启多行后
`negate/match=after` 合并，不需要续行正则），只取尾部 50 行；唯一有意差异是正则在服务端用 RE2 编译
（Filebeat 也是 RE2），而不是浏览器的 JS 正则——认证要测的是主机上真正会跑的那套规则。
校验用规则里的 `pipeline_body` 走 inline `_ingest/pipeline/_simulate`，因此不依赖集群上是否已发布
同名 pipeline；成败判定复用调试页同一个 `missingRequiredDocumentFields`（`missing_fields` 为空即通过）。

**写回必须是 upsert**（`UpsertLogSettingFormatVerified`，冲突目标唯一键
`unique_service_log_setting(service_id, log_definition_id)`）：认证状态按 (服务 × 日志定义) 读，
而覆盖行只在"有覆盖"时才存在（`ListServiceTemplateLogs` 对 `ls` 是 LEFT JOIN），UPDATE-only
对没有覆盖行的组合会静默影响 0 行、认证结果写不进去。

**自动触发**：`SaveApplicationService` 保存成功后，若该服务开了采集，则由
`MaybeAutoVerifyServiceLogFormats` 在后台（`context.WithoutCancel` + 2 分钟超时）逐条认证；
已 `verified` 的行直接跳过（认证幂等，保存是高频操作，不该反复打扰），因此"新增服务"与
"开启采集"这两种时机都被覆盖。依据优先 `instance`（最多试 3 个实例，实例级宏可能不同），
全部取不到样例时退到 `sample_log`；都失败就保持未认证，交给人在弹窗里处理。**best-effort**：
任何失败只记日志，绝不影响保存接口的返回。

**前端入口**：逻辑服务编辑弹窗的「模板日志」表格新增「格式认证」列——未认证/需重新认证是
"发起认证"，已验证是"重新认证"（改了实例级 `runtime_variables` 这类不进指纹的变化只能人工重跑）；
弹窗里选依据、按实例抽样时选实例（候选取**库里已绑定**的实例，不取表单里未保存的勾选）。
不通过时弹窗留在原地列出缺哪几个字段，允许换依据重试。同一处顺带修掉了 `loadLogConfig` 的
静默 `catch`——加载失败与"确实没有日志定义"在界面上都是空表格，必须能区分。

**边界（必须知道）**：认证是**抽样**通过，不证明文件里每一行都合规（同一文件可能混着启动横幅、
堆栈续行等）。这正是运行期 `mapping-guard` 不可替代的作用——它不拦截、只在**格式悄悄变坏**时
留下 `mapping_violation` 标记供巡检/告警发现（见 §5.4）。另外自动触发对"新服务还没绑定部署实例"
这种情况**不会认证**（没有可抽样的实例），这是预期行为而不是遗漏。

字段名统一使用下划线，不使用点号，避免 Filebeat `record_modifier` 注入时的歧义。

---

## 5. 解析规则

### 5.1 分层原则

| 处理类型 | 位置 | 原因 |
|---|---|---|
| multiline 多行合并 | Filebeat | 必须在采集时合并，堆栈跨行到后端已无法还原 |
| 字段提取 | Elasticsearch ingest pipeline | 改规则不需要下发配置到主机，也不消耗主机 CPU |

两阶段在技术上分别执行，但在 djadmin 中只管理一条 `LogProcessingRule`：

- **发送前处理（Filebeat）**：`input_format`、`multiline_enabled`、`start_pattern`、`flush_timeout`。
  Filebeat filestream 的多行用 `negate: true, match: after` 表达"非首行并入上一行"，**不需要续行正则**；
  历史列 `continuation_pattern` 保留但已不参与渲染（可选/忽略）。
- **字段解析（Elasticsearch Ingest）**：`pipeline_body`

修改 Pipeline JSON 后会立即发布同名 Elasticsearch Pipeline，不需要下发主机配置。修改日志格式或
多行参数后，必须重新应用引用该规则的日志采集目标，使 Filebeat 片段更新。

### 5.2 统一规则与 Pipeline 生命周期

`LogProcessingRule.name` 是 djadmin 规则名称（全局唯一），**Elasticsearch Pipeline 名称按下列规则派生**：

```
<索引前缀>-<应用 code|general>-<规则名>        # 各段小写、非字母数字归一化为 -
```

- 索引前缀取该规则所属集群的 `index_prefix`（默认 `autoadmin`）；
- 应用段取应用 `code`（无 code 回落 name；未选应用的通用规则用 `general`）；
- 规则名段由 `name` 归一化；例如 `autoadmin-tomcat-app-access-err`。

规则创建或更新时，djadmin 先 `PUT _ingest/pipeline/<派生名>`，成功后保存规则；改名/换应用/换集群时
自动删除旧的派生 Pipeline。删除规则时同步删除派生 Pipeline（并顺带清理历史的裸规则名）；仍被
日志定义引用的规则禁止删除。渲染 Filebeat 的 `pipeline` 字段、链路体检、发布/删除四处必须用同一个
派生名（`processingPipelineName`），禁止各自拼接。

`ApplicationLogDefinition.processing_rule` 是**唯一**的规则关联（2026-09-19 起服务侧不再有覆盖，
见 §6）：同格式日志可复用规则，Pipeline 数量不会随部署实例增长。规则的引用保护也只数日志定义
（`SELECT COUNT(*) FROM assets_application_log_definition WHERE processing_rule_id = ?`）。

### 5.3 错误指纹

错误消息通常包含变量，直接按原文聚合会导致每条都是唯一值：

```
Connection refused to 10.25.66.207:8080
Connection refused to 10.25.66.150:8080
```

在 pipeline 中先归一化再生成指纹：

```json
{
  "processors": [
    { "gsub": { "field": "error_message", "pattern": "\\d+\\.\\d+\\.\\d+\\.\\d+",
                "replacement": "<IP>", "target_field": "error_template" }},
    { "gsub": { "field": "error_template", "pattern": "\\b\\d{4,}\\b", "replacement": "<NUM>" }},
    { "gsub": { "field": "error_template",
                "pattern": "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}",
                "replacement": "<UUID>" }},
    { "fingerprint": { "fields": ["error_type", "error_template"],
                       "target_field": "error_fingerprint" }}
  ]
}
```

归一化后两条日志聚合为同一模式 `Connection refused to <IP>:<NUM>`。

常见需归一化的变量：IP、端口、UUID、长数字 ID、时间戳、路径中的实例名、线程号。

**所有 pipeline 必须配置 `on_failure`**，否则单条日志格式不符会导致整条被拒、数据丢失：

```json
"on_failure": [
  { "set": { "field": "parse_error", "value": "{{ _ingest.on_failure_message }}" }}
]
```

processor 优先使用 `dissect`，比 `grok` 快 3-5 倍。仅在格式不规则时使用 `grok`。

### 5.4 解析规则调试

页面通过 Elasticsearch inline Pipeline `_simulate` 在保存前验证当前 JSON，不需要先创建临时
Pipeline。在线调试支持两种样例格式（原始日志会随规则保存，见 `sample_log`）：

- **原始日志**：直接粘贴包含真实换行的完整日志，页面自动包装为 `{ "message": "..." }`，与
  Filebeat filestream 实际产生的字段一致；多行按首行正则聚合。
- **文档 JSON**：输入合法 JSON 对象，用于携带额外元数据；JSON 字符串内部的换行必须写为
  `\n`，不能直接回车。

**校验以集群真实 mapping 为准**：调试时后端 `GET _index_template/<prefix>-template` 取
`mapping.properties` 的顶层字段集，输出文档里不在其中的字段会作为 `schema_violations` 报出
（`dynamic:false` 下会被静默丢弃，需改写到 `app_fields.<字段名>`）；模板取不到时回退内置标准字段。

**必备字段校验（三层，共用同一份定义 `requiredProcessingRuleOutputs`）**：产物必须包含
`log_level`、`log_message`、`error_fingerprint`——缺了都不报错，只是静默失效：

| 层 | 位置 | 行为 |
|---|---|---|
| 保存时静态检查 | `validateConfigInput` → `missingPipelineOutputs` | 缺任一必备字段直接 400 拦截发布（认 `fingerprint`/`set`/`copy`/`rename` 的 `target_field`/`field`，以及 `dissect`/`grok` 的 pattern 命名捕获；**是启发式**，判不了条件分支） |
| 调试页 | `_simulate` 的 `missing_fields` | 用真实样例给出"缺哪个字段"，与上面的集合自动同步 |
| 写入时兜底 | 索引模板 `index.final_pipeline` = `<prefix>-mapping-guard` | 在规则自己的 pipeline **之后**执行，按同一组字段判定：`tag`（默认）只打 `mapping_violation` 标记，不丢数据、不做检索排除（定位是"格式悄悄变坏"的发现器，配合巡检/告警）；`drop` 直接丢弃，非默认。规则作者改不到它 |

另有一条**巡检清单**：规则列表的 `missing_required_fields`（前端「日志解析规则」页的「必备字段」
列）把"静态看着缺字段"的规则直接标红——它是切换 `drop` 模式前必须先看的清单，因为丢弃后
ES 里不留任何痕迹，违规无法事后核查。

dj-agent 的文件通道（`internal/agent/file.go` 的 `StatFile` + `ReadFileChunk`）提供
「读取该实例最近 N 行日志」的能力，它正是 §4.8 认证里 `instance` 依据的样例来源——
认证组件把抽样还原成 pipeline 输入后走同一条 `_simulate`，所以"调试页手动贴样例"与
"认证自动取样例"的判定口径完全一致，闭环已经接上。

**这项功能优先级最高**：后续所有自动错误发现能力都建立在 fingerprint 质量之上，
归一化规则不准会导致同类错误散成数百条，聚合结果不可用。

---

## 6. 开关粒度与"谁来定什么"

一句话的分工：**模板决定"采什么、怎么采"（日志名单、路径、多行、解析规则），服务决定"采不采、
采到哪个流"（采集开关、保留档位）**。三条边界都有明确的技术理由，且都落在 schema 上。

采集范围由**两层开关，且两层的归属都在逻辑服务**：

| 层级 | 字段 | 语义 |
|---|---|---|
| 逻辑服务 | `ApplicationService.log_collection_enabled` | 该服务是否开启采集（总闸） |
| 逻辑服务 × 日志定义 | `assets_application_service_log_setting.collection_enabled` | 该服务的**这条日志**是否采集 |

```
服务总开关 ON  AND  该服务该条日志未被关闭
    → 服务下所有部署实例均采集该条日志
```

**部署模板不设采集开关**（2026-09-19 迁移 000034 删除了
`assets_application_log_definition.collection_enabled`）。理由：模板上的开关是**整模板**的，
同一个模板被多个逻辑服务复用时，无法只关某个服务的某条日志；而且渲染查询曾把它写在 JOIN
条件里（`ld.collection_enabled = TRUE`），服务级覆盖选"强制开启"也不生效——开关语义是坏的。
模板现在只描述"这条日志的路径怎么算、挂哪条处理规则"（`path_pattern` / `processing_rule` /
`extra_fields` / `name`），**是否采集完全由服务决定**。

**解析规则反过来：只由模板的日志定义决定，服务侧只读**（2026-09-19 迁移 000035 删除了
`assets_application_service_log_setting.processing_rule_id`）。理由与开关对称：同一模板下多个
服务的日志格式大概率相同，规则就该相同；要不同就另建模板（或另加一条日志定义）。删掉服务级
覆盖后，渲染查询里 `COALESCE(rule_setting.x, rule_definition.x)` 这一整族取值消失——
`pipeline` 名与三个多行参数（`multiline_enabled` / `start_pattern` / `flush_timeout`）
唯一来源是 `assets_application_log_definition.processing_rule_id`。

> **升级语义（一次性）**：迁移 000035 **直接覆盖**，不留观察期——服务上原来自选的规则不再生效，
> 相关主机进入「待下发」，重新下发后主机上的 `pipeline` 才真正换掉；已入库的历史数据不重新解析，
> 新旧解析产物会混在同一个 data stream 里。另一个连带后果：**模板日志定义没挂规则时服务无法
> "自救"**（以前可以覆盖一条规则把它采起来），必须先到模板给它挂规则——渲染跳过未挂规则的
> 日志并产出 warning（§8.4）。

**档位归服务**，因为 data stream 名里含服务 code 与档位（§4.1），模板天然决定不了"采到哪个流"。

服务级每日志开关的取值语义（与渲染查询 `ListHostLogRenderEntries` 的
`COALESCE(ls.collection_enabled, TRUE) = TRUE` 逐字一致）：

| 覆盖行 | 该条日志 | 落库方式 |
|---|---|---|
| 无覆盖行 / `collection_enabled IS NULL` | 采（**默认采**） | 服务弹窗里开关保持"采"时不落行 |
| `collection_enabled = FALSE` | 不采 | 服务弹窗关掉该条日志 → 写覆盖行 false |
| `collection_enabled = TRUE` | 采 | 与"无覆盖行"等价，UI 归一成 null 不落行 |

**默认采**是刻意的：新增模板日志定义、扩容新增实例都自动继承采集配置，不需要逐条确认；
要停某条日志就在服务弹窗里显式关掉（`docs/plans/LOG_COLLECTION_LIFECYCLE.md` §9 第 8 条的
"不漏采优先"）。给"新增日志定义"留的闸门是另一条既有约定：**未关联处理规则的日志不采集**
（§8.4 渲染会跳过并告警），所以模板里新加的日志只要先不挂处理规则，就不会被采上来。

**部署实例层不设开关**。同一逻辑服务下的实例配置一致是常态，HA 主备同样都需要采集；
确有差异时拆分为两个逻辑服务处理，不为罕见例外向所有正常场景引入配置维度。

附带收益：新增实例自动继承采集配置，扩容后不会漏采（"不漏采"指期望配置立即包含新实例；
落到主机上仍需一次「下发配置」，见 §8.9 的自动下发建议）。

---

## 7. 数据模型改动

```
ApplicationService
  + log_collection_enabled    BooleanField(default=False)
  + log_retention_tier        档位 FK（Go: assets_application_service.log_retention_tier_id
                              → monitor_log_retention_tier.id，null=按 std 处理）

ApplicationLogDefinition      已存在 path_pattern / encoding
  + processing_rule           ForeignKey(LogProcessingRule, PROTECT, nullable)
                              **解析规则的唯一来源**：服务侧不可覆盖（迁移 000035 删掉服务级覆盖）
  + extra_fields              JSONField    附加标签（⚠️ 尚未参与渲染，见 §4.7）
  （原 collection_enabled 已于迁移 000034 删除：采集开关下沉到逻辑服务，见 §6）

ApplicationServiceLogSetting  服务级覆盖，一行 = (服务 × 日志定义)
  collection_enabled          Boolean，NULL/无行 = 采；FALSE = 该服务不采这条日志
  retention_tier_id           保留档位覆盖（NULL = 继承服务默认）
  collection_filter_rule_id   采集过滤规则覆盖（⚠️ 目前不影响采集，见 §6 末）
  format_verified_at / format_verified_fingerprint / format_verified_source / format_verified_by
                              日志格式认证（迁移 000036）：认证时间、认证时的配置指纹、依据
                              （instance/sample_log/waiver）、操作人。指纹与当前配置不一致 =
                              needs_recheck（§4.8）
  （原 processing_rule_id 已于迁移 000035 删除：解析规则只由模板日志定义决定）

LogProcessingRule
  cluster                     ForeignKey(ElasticsearchCluster)
  name                        CharField(unique=True)；Pipeline 名称由 <前缀>-<应用 code|general>-<name> 派生
  description                 CharField
  input_format                CharField(text/json)
  multiline_enabled           BooleanField
  start_pattern               TextField
  continuation_pattern        TextField（保留列，Filebeat 不参与渲染）
  sample_log                  TextField；「在线调试」的原始日志样例，随规则保存
  flush_timeout               PositiveIntegerField(100-60000 ms)
  pipeline_body               JSONField；必须包含 processors 数组

LogCollectionTarget           新增，主机级
  host                        OneToOne(Host)
  managed_enabled             BooleanField
  install_status              CharField
  runtime_status              CharField
  config_fingerprint          CharField    已下发配置的 hash
  last_applied_time           DateTimeField
```

旧的 `ApplicationLogDefinition.multiline_parser`、`ingest_pipeline` 和 `retention_days` 已删除，
不保留兼容分支。保留期由索引档位决定。

Elasticsearch 连接信息由 `ElasticsearchCluster` 统一保存，不硬编码；日志处理规则明确关联目标集群。

### 7.1 管理面的写路径（Go 版最终逻辑，2026-09-16 随 SQL 迁移定型）

**保留档位 / 解析规则 / 采集过滤规则**（`/monitor/log-retention-tiers|log-processing-rules|log-filter-rules`，实现见
`internal/logcollect/config_resources.go` 与 `config_resource_writers.go`）：

- **"只写提交了的字段"这一 PATCH 语义**由「更新前读回整行 → 合并提交的字段 → 整行写」承担
  （`COALESCE(narg,col)` 表达不了"显式写入空值/删除"，例如把解析规则的 `application` 显式提交为 `null`）。
- **新建**是整行插入：请求体缺了"NOT NULL 且库级没有默认值"的列时返回 400 并列出缺哪些键
  （原先由 MySQL 严格模式报 `Field 'x' doesn't have a default value`，文案不可读）。
- 校验规则（档位 code 格式、`daily_size_gb > 0`、`retention_days ∈ [1,3650]`、`rollover_min_index_age` 形如 `30m/12h/1d`、
  规则名 `^[a-z0-9][a-z0-9._-]*$`、`pattern` 不得含换行且必须是合法正则、`flush_timeout ∈ [100,60000]`）不变；
  解析规则保存前先发布 pipeline 到集群、失败则整条请求 400 且不落库的行为不变。
- 删除仍是"逐 id + 汇总 `{count, results}`"的批量语义；档位被逻辑服务/日志设置引用、规则被日志定义引用时拒绝删除
  （引用计数用取行/计数查询，不再 `SELECT COUNT(*)` 兼职判存在）。

**Elasticsearch 集群**（`/monitor/elasticsearch-clusters/*`，实现见 `elasticsearch_config.go`）：

- "只支持一个集群"；新建前计数校验，`is_default` 提交为 true 时先清掉其它行的默认标记（事务内）。
- 密码列存的是密文：提交值等于 `******` 时保持原值，空串表示清空。
- **建集群时显式写 `last_check_message` / `storage_sync_error` / `storage_sync_status` 三列**（都是 NOT NULL 且库级
  没有默认值）：原实现从不写这三列，在严格模式下建集群恒报 1364，**该接口一直不可用**（"只支持一个集群"、
  现场早有记录，所以没人碰到）；2026-09-16 随 SQL 迁移发现并修掉。
- `GET /monitor/elasticsearch-clusters/:id/index-template/`（`GetElasticsearchIndexTemplate`）：日志存储页
  「查看 Mapping」用，`GET /_index_template/<prefix>-template` 取实际 mapping 的顶层字段（名/类型）返回；
  模板不存在时回退内置 `standardLogFields` 并标记 `exists=false`，便于对比"期望 vs 实际"。

**部署模板的日志定义**（`POST/PUT /assets/deployment-templates/*`，实现见 `internal/assets/template.go`）：

- **按 id 增量写**（2026-09-19）：提交的日志列表带 `id` 的原地更新、不带的新增、库里没提交的删除，
  不再"整表删掉重建"。这保住了两件事：日志定义 id 稳定 → 服务级覆盖行（档位/处理规则/采集开关/
  过滤规则）不会因为改模板而失效；以及删除时不会撞外键。
- 删除前**先清掉引用该定义的覆盖行**（`DeleteServiceLogSettingsByDefinitionIDs`——外键无级联），
  顺序是先删后改再插（改名可以复用同一次保存里刚删掉的名字，唯一键不打回）。
- 校验：提交的 `id` 必须属于本模板（否则 `ErrInvalidRelation`）、日志名不能为空
  （否则会生成采不到文件的片段）；**新建/复制模板时提交里的 id 一律按新增处理**
  （复制场景前端会把源模板的行原样提交，那些 id 属于源模板）。
- 其它嵌套子表（端口/路径/配置文件/控制动作/docker）仍是"提交了就整组删重建"，与本次改动无关。

---

## 8. 配置下发

### 8.1 主机侧目录结构

```
/opt/filebeat/filebeat          预编译单二进制（官方 tar.gz 解压，自带依赖）
/etc/filebeat/filebeat.yml      主配置：output.elasticsearch + filebeat.config.inputs 托管 inputs.d
/etc/filebeat/inputs.d/         后端按 服务×日志定义 生成，每实例一个 filestream input
/var/lib/filebeat/              registry（offset）与 data，必须持久化
```

主配置（agent `configure_filebeat_output` 写入，0600）：

```yaml
filebeat.inputs: []
filebeat.config.inputs:
  enabled: true
  path: /etc/filebeat/inputs.d/*.yml
  reload.enabled: true
  reload.period: 10s
output.elasticsearch:
  hosts: ['https://<es>:9200']
  username: '<user>'
  password: '<pass>'
  ssl.verification_mode: none   # verify_tls=false 时；true 用 full
```

### 8.2 日志输入片段（filestream）

按**服务×日志定义**聚合为 `/etc/filebeat/inputs.d/<app>__<svc>__<log>.yml`，
文件内**每个实例一个 filestream input**（维度字段必须按实例区分，不能合并成单 input 多 paths）：

```yaml
- type: filestream
  id: '<app>__<svc>__<log>__<instance>'
  enabled: true
  paths:
    - '/home/esb/tomcat/logs/catalina.out'
  fields_under_root: true
  fields:
    service: 'tomcat-svc'
    instance: 'kul-tib-tomcat1'
    application: 'tomcat'
    log_name: 'catalina'
    business_system: 'tib'
    project: 'kul'
    environment: 'test'
    host_ip: '192.168.201.211'
    log_path: '/home/esb/tomcat/logs/catalina.out'   # 该实例实际监听的绝对路径
  index: 'autoadmin-<项目>-<业务>-<环境>-<服务>-<档位>'
  pipeline: 'springboot-tomcat-exception'   # 可选，处理规则非空时
  parsers:                                    # 可选，处理规则开启多行时
    - multiline:
        pattern: '^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3}\s+'
        negate: true
        match: after
        timeout: '2s'
```

- 路径由 `${APP_HOME}` 等宏展开；展开后仍含 `${VAR}` 的实例跳过并记入 warnings（带病下发会静默监听不到文件）。
- 多行由 filestream multiline 在发送前完成：`negate: true` + `match: after` = 不匹配首行正则的行并入上一行；ingest pipeline 拿到的已是按行拆开的文档，无法回溯合并。
- 字段提取/时间戳归一由 Elasticsearch ingest pipeline 承担。

### 8.3 输出与索引

- **output.elasticsearch**（地址/账号/TLS）统一写在主配置，由 agent 下发；凭据不落 inputs 片段。
- **output 与是否有日志无关**：只要存在启用的默认 Elasticsearch 集群，「下发配置」就写
  `/etc/filebeat/filebeat.yml` 并启动 Filebeat；该主机当前没有启用任何日志采集时，inputs.d 为空
  （agent 会清理旧片段），不再报“没有可下发的日志片段”。之后开启采集再下发一次即可。
- **配置指纹覆盖「输出段 + 全部片段」**（2026-09 修复）：`renderHostLogConfig` 的 digest 输入的
  `outputIdentity` 由默认集群的 **地址 / 账号 / verify_tls** 组成（`filebeatOutputIdentity`），
  再加上全部片段内容。任一变化都会让指纹变化 → 重新下发主配置与片段。
  修复前 digest 只对 fragments 求和，导致「只改集群地址/账号/TLS → 指纹不变 → 下发被跳过 →
  `filebeat.yml` 停留在旧值」。
- **指纹刻意不含口令**：指纹会随 apply/preview 响应与主机列表返回给前端，把口令（即使只是哈希）
  放进去等于给出离线猜测的口子。代价是"只改口令"不判为漂移，需人工重新下发一次。
- 每个 input 用 `index` 指定 **LogDataStreamName**（服务级数据流命名 `autoadmin-<项目>-<业务>-<环境>-<服务>-<档位>`），保留档位/ILM 按此后缀生效。
- 维度字段通过 `fields_under_root` 写入文档根，服务树日志检索（buildLogQuery）按这些 term 过滤，缺字段会查不到。

### 8.4 下发流程（Go 版，log_config_render.go + apply_filebeat_config）

> **入口唯一**（2026-09 统一）：单条 `POST /log-targets/:id/apply/`（`ApplyLogTargetConfig`）与
> 「批量下发」「安装成功后自动下发」走的是**同一条** `applyLogTargetConfigRow` 全流程。
> 此前单条入口有一条旁路：只调 `configure_filebeat_output`（只写主配置，见
> `dj_agent/internal/executor/builtin_actions.go`），不下发 inputs.d 片段、不写 `config_fingerprint`，
> 却把 `runtime_status` 置 running 并清空 `last_error` —— 表现为"下发成功"但主机上没有采集片段，
> 且指纹永远为空（漂移判定会把它一直当成"从未下发"）。该旁路及其独占的两条语句
> （`GetLogTargetDefaultCluster`、`MarkLogTargetApplied`）已删除。

```
「下发配置」（applyLogTargetConfigRow，log_target_actions.go）
  → agent 在线检查
  → loadHostLogRenderInput：主机上启用采集的服务×日志定义（有效采集开关只有服务侧两处：
    s.log_collection_enabled 且 COALESCE(log_setting.collection_enabled, TRUE)——
    无覆盖行/覆盖为 NULL 即"采"，显式 FALSE 才不采，见 §6；
    deployment_template 必须匹配服务所属模板），
    带出 项目/环境/业务/服务 code、有效档位（log_setting 优先回落服务表）、
    解析规则（只来自模板日志定义 ld.processing_rule_id，服务侧无覆盖，见 §6）、
    服务级宏（macro_values）、部署模板宏定义（macro_definitions）
   → renderHostLogConfig（纯函数）：
      inputs.d/<app>__<svc>__<log>.yml —— 每实例一个 filestream input：
        路径按 服务级+实例级宏替换 ${VAR}（宏来源，优先级从低到高：
        部署模板 macro_definitions 的 value（默认值，与前端服务弹窗展示口径一致）→
        服务级 macro_values（覆盖同名项）→ 部署实例 runtime_variables；部署模板 app_home
        作为 APP_HOME 默认值；替换后仍含 ${VAR} 的实例跳过并记 warnings）；
        日志定义 name 直接作为文件名/维度值、不参与宏展开——name 含 ${...} 时跳过该定义
        并记 warnings（宏应写在 path_pattern 里，否则会生成监听不到文件的坏片段）；
        日志定义未关联处理规则（无 pipeline）时不采集：跳过该日志定义并记 warnings
        （避免"采进来了但查不到 log_level/log_message/error_fingerprint"的半成品数据）；
        fields_under_root 注入 service/instance/application/log_name/
        business_system/project/environment/host_ip/log_path（log_path 为该实例实际监听的绝对路径）；
        index = LogDataStreamName；处理规则非空带 pipeline；开启多行带 multiline parser
      指纹 = outputIdentity（集群地址/账号/verify_tls）+ 全部片段内容 的 sha256；
      全实例被跳过时不产出片段；
      片段非空时附带 /var/lib/filebeat/.keep 占位（agent MkdirAll 顺带建 registry 目录）
      warnings 随 preview/apply 响应返回，前端在「下发配置」成功/批量结果里提示（此前被丢弃）。
  → 指纹与 monitor_log_collection_target.config_fingerprint 一致则跳过（仅刷新时间）
  → agent 动作 configure_filebeat_output（写 filebeat.yml：output.elasticsearch +
    filebeat.config.inputs 托管 inputs.d，`filebeat test config` 后 restart）
  → agent 动作 apply_filebeat_config（写 inputs.d/*.yml，删残留片段，files 为空时只清理，test config 后 restart）
      —— inputs 为空不报错：output 配置照常下发，主机先纳管可用
  → 回写 config_fingerprint / last_applied_time
```

- **`runtime_status` 的写入语义（避免与旧旁路混淆）**：统一后的成功路径在**两次 agent 调用都成功**
  （`configure_filebeat_output` + `apply_filebeat_config` 已重启 Filebeat）之后才回写
  `runtime_status='running'` / 清空 `last_error`——这是"刚重启过、服务应在运行"的推断，
  不是独立探活（真实状态仍由启停/查状态的结果维护）。指纹一致走跳过分支时只刷新
  `last_applied_time`，**不动** `runtime_status`。旧单条旁路的问题是没有任何下发动作却直接置 running。
- **安装/卸载与下发的时间口径（Go 版，2026-09-16 随 SQL 迁移定型）**：作业/历史的时长由应用层算
  （历史的 `create_time` 就是派发时刻；原实现用 `TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000`），
  取消时同样按 `create_time` 算时长；作业收尾读回 `automation_execution_job.result_summary` 的 `message`
  在应用层解析（原实现是 `JSON_UNQUOTE(JSON_EXTRACT(...,'$.message'))`）；收尾语句里
  "成功则把 runtime_status 置 running" 用整数标志传（同一条语句里重复写同一个 `sqlc.arg` 会让
   MySQL 与 PG 生成的参数个数不一致）；运行态探测结果照旧按 systemctl 退出码语义映射
  （0=运行中、3=已停止、其余=异常）。
- **`agent_installed` 由收尾语句维护（2026-09-17 修复）**：它是"Filebeat 二进制已装"的持久态，
  安装成功置 TRUE、卸载成功置 FALSE，失败时传 NULL 保持原值（`COALESCE(narg, agent_installed)`）。
  此前 Go 版只写 `install_status/runtime_status`，从未维护该列，导致链路体检"主机配置"层恒报
  "Filebeat 未安装"、"采集进程"层恒空、宿主列表 `filebeat_filter` 与前端启停按钮误判。
  历史行由迁移 `000029_backfill_log_target_agent_installed` 按 `install_status` 回填
  （success→TRUE、uninstalled→FALSE，其余不动）。
- **失败文案取 stderr（关键）**：local ansible 退出码非 0 时 `runErr` 就是 `*exec.ExitError`，
  直接写 `runErr.Error()` 只会得到 `exit status 2` 这种无信息量文案。作业摘要与主机日志统一走
  `ansibleResultMessage`：优先 stderr（真正的 ansible 报错），其次 stdout，只有进程没起来/无输出
  时才回退 `runErr`。这条同样适用于 exporter 安装等所有 `automation_execution_job`。
- **Filebeat 只支持 tar.gz（官方便携包）**：软件包 `package_type=filebeat`、`package_format=tar.gz`、
  `platform_family=any`、`platform_major` 留空；选包只按资产采集的 CPU 架构
  （`assets_hosthardware.architecture` 归一为 amd64/arm64）匹配，不再区分发行版/主版本，也不支持 rpm/deb。
  架构缺失时 `pickFilebeatPackage` 会先自动补采一次。
  便携包布局：二进制 `/opt/filebeat/filebeat`、配置 `/etc/filebeat`、数据 `/var/lib/filebeat`，
  systemd 服务名固定 `filebeat.service`。
  上传文件名以记录名为前缀，接受 `<name>-<version>.<os>-<arch>.tar.gz` 与
  `<name>-<version>-<os>-<arch>.tar.gz`（也接受省略 os 的 `<name>-<version>-<arch>`），
  arch 支持 `x86_64`/`amd64`、`aarch64`/`arm64`。**必须放预编译二进制包**（解压后含 `filebeat`）。
  安装 Playbook 只做「拷贝 tar.gz → 校验 sha256 → 解压到 `/opt/filebeat` → 写 systemd unit → enable」，
  主配置/inputs 由 agent 在「下发配置」时写入（见 §8.4），playbook 不碰文件内容。
  **默认配置会写进软件包**：创建/编辑 Filebeat 包时，若「安装/卸载 Playbook 内容」和
  「systemd unit 文件内容」为空，后端用 `internal/shared/filebeat` 的默认内容
  创建 `automation_playbook_template` 并绑定、把默认 unit 写进 `service_file_content`
  （启动时也会给历史 Filebeat 包回填一次），用户可在编辑弹窗里看到并修改；
  extra_vars 为 `service_name`/`package_local_directory`/`package_file_name`/`package_sha256`/
  `service_file_content`。下方这份是等价参考。
  安装成功后后端会**自动下发一次采集配置**（等价于「下发配置」），写入 `/etc/filebeat/filebeat.yml`
  与 `inputs.d` 并启动服务，所以「安装」即可用，无需再手动点一次。
  Filebeat 安装与 exporter 安装同链路：一次派发同时写 `automation_execution_job`
  （`source='monitor_target'`）与 `monitor_target_install_history`（`automation_job_id_snapshot`
  指向该作业）。运行记录中心是单页无 tab（只有自动化任务运行记录，可按来源过滤）；
  纳管目标的「查看日志」取最新历史关联的作业 ID，跳到运行记录中心对应作业日志。

  ```yaml
  # 安装（tar.gz）
  - hosts: all
    become: true
    gather_facts: false
    vars:
      fb_home: /opt/filebeat
      archive: "/tmp/{{ package_file_name }}"
    tasks:
      - copy: { src: "{{ package_local_directory }}/{{ package_file_name }}", dest: "{{ archive }}", mode: "0644" }
      - shell: 'echo "{{ package_sha256 }}  {{ archive }}" | sha256sum -c -'
      - shell: |
          set -euo pipefail
          rm -rf {{ fb_home }}.new && mkdir -p {{ fb_home }}.new
          tar -xzf {{ archive }} -C {{ fb_home }}.new --strip-components=1
          rm -rf {{ fb_home }} && mv {{ fb_home }}.new {{ fb_home }}
      - file: { path: /var/lib/filebeat, state: directory, mode: "0755" }
      - copy:
          dest: "/etc/systemd/system/{{ service_name }}.service"
          mode: "0644"
          content: "{{ service_file_content }}"
      # 只 enable 不 start：安装成功后端会自动下发配置并启动；手动流程则点「下发配置」。
      - systemd: { daemon_reload: true, name: "{{ service_name }}", enabled: true }
  ```

  ```yaml
  # 卸载（tar.gz）
  - hosts: all
    become: true
    gather_facts: false
    tasks:
      - systemd: { name: "{{ service_name }}.service", enabled: false, state: stopped }
        failed_when: false
      - file: { path: "/etc/systemd/system/{{ service_name }}.service", state: absent }
      - file: { path: /opt/filebeat, state: absent }
      - systemd: { daemon_reload: true }
  ```

- **片段目录由 autoadmin 全量托管**：渲染结果即该主机期望的完整 inputs 片段集合，agent 落盘后会
  删除 `inputs.d` 中不在本次清单内的 `*.yml` 遗留片段（改名/维度修正/服务下线后的残留），
  删除与写入任一发生即 `filebeat test config` + `systemctl restart filebeat`。
- 预览接口：`GET /monitor/log-targets/:id/config-preview/`（GetHostLogConfigPreview），只渲染不下发。
- agent 侧动作：`configure_filebeat_output`（写主配置 output/托管段）、`apply_filebeat_config`
  （写 inputs + 清理 + 校验 + 重启）；服务启停走通用 `systemctl` 命令通道（`filebeat.service`）。

### 8.5 重载

Filebeat 的 `filebeat.config.inputs.reload.enabled: true` 支持 inputs.d 热加载；当前下发流程为保证
主配置与片段一致，仍在有变化时 `filebeat test config` 后重启 `filebeat.service`（registry 记录 offset，
重启后从断点继续，仅一到两秒间隙）。

### 8.6 清理

服务关闭采集、实例移除、服务删除时，必须删除对应的 `inputs.d` 片段；agent 全量托管会
在下次下发时清理残留。否则 Filebeat 会持续尝试采集无人管理的文件，或因文件不存在反复报错。

### 8.7 服务维度的下发（2026-09-19）

排障时想的是"把这个服务的采集配置下发一下"，而不是"把主机 A、B、C 勾上"。但**下发的最小完整单位
只能是主机**，这一条由 agent 侧的实现决定：`apply_filebeat_config` 的语义是「本次交付的文件集
就是该主机 `inputs.d` 的全量，**没交付的 `.yml` 一律删除**」（`dj_agent/internal/executor/builtin_actions.go`）。
所以"只推某个服务的片段"会把同一主机上其他服务的配置删掉；`output.elasticsearch` 也只在主机级存在。

因此服务维度做的是**扇出 + 聚合**，不是改变下发的粒度：

| 接口 | 作用 |
|---|---|
| `GET /monitor/log-targets/service-config-state/?application_service_id=` | 承载该服务的**主机清单**（区分已纳管/未纳管）+ 各主机配置态聚合 |
| `POST /monitor/log-targets/service-apply/`（body `{application_service_id}`） | 对承载该服务的全部**已纳管**主机建一次批量作业（复用 §8.8 的作业机制，进度可查） |

三个必须守住的语义：

- **解析主机时不过滤启用态**（`ListServiceLogApplyTargets` 不滤 `s.enabled` / `s.log_collection_enabled` /
  `sd.enabled` / `d.enabled`）。停用服务或关掉采集之后，恰恰需要下发一次才能**移除**主机上的旧片段
  （渲染层把停用服务排除 → agent 删掉未交付的 `.yml`，见 §8.6）。按启用态过滤主机，停用的服务
  就永远清不干净。变化后的主机是否需要真下发由渲染指纹决定（一致则跳过，见 §8.7 之后的配置态）。
- **未纳管的主机必须显式回报**（`target_id` 为 NULL 即未纳管）：下发不了它们，接口把主机名列出来，
  否则用户以为整个服务都下发了，实际少了几台。
- **状态只能是聚合，没有"服务级指纹"**：`config_fingerprint` 是主机级的，同一服务在不同主机上因
  实例级 `runtime_variables` 不同、渲染结果本就不同。接口只回"N 台里 M 台待下发"这类计数。

**幂等且廉价**：指纹一致的主机在 `applyLogTargetConfigRow` 里被整段跳过（只刷新 `last_applied_time`），
所以"把服务的主机都下发一遍"不必先筛待下发。

**文案要说实话**：按钮语义是"对承载本服务的主机重新下发"，会连带重算同主机上其他服务的配置
（渲染确定性所以内容等价；若它们本来就有未下发的改动，会被一起带上——通常是想要的）。

**入口**：日志中心页「日志配置」tab 顶部（聚合状态 + 下发按钮 + 作业进度）。按主机的单条下发
（「日志采集」页）与整批下发（`ids` 省略 = 全部待下发）都保留，三者互补。

### 8.8 配置状态（期望 vs 已下发）与链路体检（2026-09-18）

**配置态**回答"主机上的采集配置是不是当前该有的那份"，与运行态（agent/Filebeat 是否在跑）
正交：配置一致不代表进程在跑，进程在跑也不代表配置是最新的。

| 状态 | 判定 | 含义 |
|---|---|---|
| `synced` | 期望指纹 == `config_fingerprint` | 主机上的配置与当前期望一致 |
| `drift` | 已下发（指纹非空）但两者不同 | 配置变更后未重新下发 |
| `never` | `config_fingerprint` 为空 | 从未下发过 |
| `unknown` | 期望指纹算不出来 | 缺省未注入评估器，或没有启用的默认集群；原因经 `config_state_error` 回传 |

- **计算口径与下发完全一致**：期望指纹由渲染纯函数 `renderHostLogConfig` 算出
  （= 输出段 `outputIdentity` + 全部片段内容，见 §8.3），索引前缀/流名取默认集群的 `index_prefix`。
  实时算、不落库——存下来的状态会过期。
- **算力约束（500–1000 台）**：一次批量评估只做 **1 次默认集群查询 + 2 次渲染输入查询**
  （`ListHostLogRenderInstances` / `ListHostLogRenderEntries` 按 `host_id IN (…)` 批量取），
  查询次数与主机数无关，渲染是纯函数无 IO。展示场景只算当前页（`page_size` ≤30）；
  **筛选/统计场景必须一次算全量**（见下）。
- **列表暴露**（`GET /monitor/targets/host-overview/`）：`filebeat.config_state` /
  `expected_fingerprint` / `config_service_num` / `config_warnings`（未关联处理规则、路径宏展不开
  等渲染告警），页面级 `config_state_error`。未纳管的主机没有日志目标，**不参与任何配置态**
  （否则"待下发"里会混进根本没纳管 Filebeat 的机器）。
- **筛选 `config_state`**：状态是实时算出来的，SQL 无法按它过滤，所以启用筛选时先取回全部匹配
  主机、批量评估、再在内存里过滤与分页；取值只接受 `synced|drift|never|unknown`，非法值 400
  （避免前端写错时静默返回全量）。
- **体检"主机配置"层改为内容比对**：逐主机比对期望指纹与已下发指纹，`drift` 报"配置已过期，
  需重新下发"、`never` 报"从未下发"；`synced` 的明细带上覆盖的服务数与渲染告警。
  期望配置算不出来时（典型是没有启用的默认集群）**退回"是否下发过"的判断并把原因写进明细，
  不谎报"一致"**。（此前只看指纹是否为空、不比对内容，主机配置过期在体检里完全看不出来。）
- **"下发待变更"的全量口径**：`GET /monitor/log-targets/pending-summary/` 一次算完全部纳管目标
  （查询次数与主机数无关），前端按钮显示的就是这个全量台数（`drift`+`never`）；
  点击后由后端建批量作业执行。2026-09-18 之前只作用当前页，属计划 Phase 2 的半成品形态。
- **界面位置**：采集目标的日常操作与配置状态都在「日志管理 → 日志采集」
  （`fronted/src/views/monitor/log-collectors/index.vue`，2026-09-18 从「智能监控 → 纳管目标」拆出，
  见 [MENU_STRUCTURE](MENU_STRUCTURE.md)）；主机表外壳与状态逻辑与 Exporter 目标页共用
  `components/HostTargetPanel.vue` + `util/hostTargetTable.js`。
- **一次性影响**：Phase 0 改了指纹语义（纳入输出段），因此存量主机的 `config_fingerprint`
  与期望值不再一致，升级后会全部显示 `drift`，重新下发一次即恢复稳定。

### 8.9 批量动作：异步作业（2026-09-18，计划 Phase 2）

批量「下发配置」与批量「安装/重新安装」不在请求内执行，而是**入队 + 有界并发 + 进度可查 + 可续跑**
的作业。1000 台规模下这不是优化而是前提：旧实现把目标放在一个 HTTP 请求里逐台串行（每台 2 次
agent 调用、超时 60s+120s），必然超时并留下部分下发的中间态；批量安装则在 API 进程内联对每台
起一个 `go func()`，没有并发上限。

- **落库载体**：`monitor_log_batch_job`（动作/状态/计数/并发）+ `monitor_log_batch_job_item`
  （每台的状态与失败原因，`host_name`/`host_ip` 是**快照**，主机改名后进度页仍如实展示当时的目标）。
  计数**从 item 表重算**而不在应用层累加（并发下累加必然漂移）。
- **入口**：`POST /monitor/log-targets/batch-jobs/`，body `{action: "apply"|"install", ids?: [...]}`。
  `ids` 省略且 `action=apply` 表示"全部待下发"——由服务端按配置态实时算全量（一键应用 N 台待变更）。
  另有 `GET /monitor/log-targets/batch-jobs/:id/`（进度 + 逐台明细）、
  `GET /monitor/log-targets/batch-jobs/active/?action=`（页面刷新后接着看进度；没有进行中的作业时 `data: null`，
  不用 404——那是每次进页面都会调的查询，404 会在前端弹无意义的错误提示）、
  `GET /monitor/log-targets/pending-summary/`（全量"待下发"计数：drift + never）。
- **队列与消费方**：消息投到 `rabbitmq.LogCollectRoute`（`autoadmin.logcollect.execute`），
  **由 api 角色消费**。原因：这些动作要通过 agent gRPC 会话在主机上执行，而 agent 会话是 api
  进程内的 `Gateway` 会话表，放到 worker 角色上 `IsOnline` 恒为 false、作业会全部失败。
  计划任务那条队列（`autoadmin.job.execute`）仍归 worker 角色。
  消费者的"停"是显式失败：连接断开导致消息流关闭时 `ConsumeVia` 返回错误而不是静默收工
  （否则队列从此无人消费、批量作业永久排队），由 `Restart=on-failure` 拉起；
  停机期间（ctx 已取消）的返回不算失败，避免一次干净的 SIGTERM 被记成异常退出。
- **有界并发**：`LOG_BATCH_INSTALL_CONCURRENCY`（默认 20）与 `LOG_BATCH_APPLY_CONCURRENCY`
  （默认 5）分开配置——下发会重启 Filebeat，并发放大等于让全网同时抖动（计划 §9 第 9 条）。
  `LOG_BATCH_PREFETCH`（默认 2）是同时在跑的作业数。
- **分片与可续跑**：一条消息只跑 `LOG_BATCH_BUDGET`（默认 15 分钟）就投一条续跑消息并返回
  （必须明显小于 RabbitMQ 的 `consumer_timeout` 30 分钟，否则会被服务端强断并重投）。
  消息只在分片跑完/已让出后才 ack；进程被杀时消息未 ack、重启后重投，而执行只挑仍是
  `pending` 的 item（续跑前把上一轮遗留的 `running` 回落为 `pending`），**已成功的不会重跑**。
  **进入执行前先读作业状态**：终态 → ack 掉（跑完的一批不会被跑第二遍）；`pending` → 用
  `pending → running` 的条件更新认领，抢不到就 ack；`running` → 说明这是自己上一个分片投出的
  续跑消息，继续推进。**续跑这一支不能靠 UPDATE 的行数判断**——MySQL 的 UPDATE 返回"实际改变的
  行数"（未开 `CLIENT_FOUND_ROWS`），把 `running` 再写成 `running` 会返回 0 行，
  那样分片续跑会被误判成"抢不到执行权"而永远停在 `running`（此坑是真库验证时逮到的）。
- **失联对账**：作业每 30 秒刷新一次头表心跳；`StartReaper`（api 进程内每分钟一次）把
  "running 且 5 分钟没有心跳"的作业回落为 `pending`、把遗留的 `running` item 落回 `pending`，再重投——
  执行进程消失后作业不会永久停在 running。"没有待处理项但计数不满"（只有 `running` item）时
  执行器**不收尾也不重投**，让出给对账收敛：既不把半途而废的一批报成完成，也不会变成热循环。
  注意失联判定的前提是**单实例部署 api 角色**（仓库既有假设，对账本身也是进程内单 goroutine）；
  若将来并行多实例，需要把对账改成带 leader 选举或让 Redis/DB 做租约。
- **同一作业的分片串行**：续跑消息与当前分片可能在同一个进程内并发被取到（prefetch>1），
  执行器按作业 id 加进程内互斥锁后才动手，避免两个分片同时从 item 表挑 `pending` 项、把同一台跑两遍。
- **批量启停/删除仍是同步接口**：单台只是一次 30 秒超时的 `systemctl` 调用或一条 DELETE，
  逐台串行的代价可接受；分钟级的「下发 / 安装」才需要作业化。
- **前端**：`GET pending-summary` 的全量计数驱动「一键下发全部待变更（N）」按钮；作业创建后用
  作业详情接口每 2 秒轮询一次，进度弹窗展示百分比、成功/失败台数与**逐台失败原因**，
  关闭弹窗不中断作业；页面重新进入时用 `batch-jobs/active/` 恢复未跑完作业的进度视图。
- **回归**：`log_batch_job_test.go` 覆盖终态判定与分片/续跑/对账分支；`smoke_log_batch_test.go`
  （`MONITOR_SMOKE_DSN` 触发）在真库上跑整套语句与对账链路——`ClaimLogBatchJob` 的"影响行数"语义
  就是真库冒烟逮出来的（见上文续跑那一支）。
- **下发门槛放宽**：单台「下发配置」只要 `agent 在线 + Filebeat 已装`即可（原来还要求
  `runtime_status=running`）。下发本身会写 `filebeat.yml` 与 `inputs.d` 并重启 Filebeat，
  要求"先在跑"会让停机待修的主机永远无法通过下发恢复。
- **改档位提示**：档位在服务/日志定义上改动时提示"写入新流，旧流停写并按原档位保留到期、不迁移"，
  并提示需重新下发采集配置（计划 Phase 2 的第四项）。

### 8.10 变更 → 影响矩阵与「配置自动下发」建议（2026-09-19）

§8.7 的配置态回答"主机上的配置是不是当前该有的那份"；本节回答另一半：**改了什么配置会导致
什么样的主机动作与数据流后果**，并对每个动作给出**是否值得改成配置自动下发**的建议。

先记住三条机制（推导全部来自它们）：

1. **期望指纹** = 输出段标识（默认集群地址/账号/`verify_tls`）+ 全部片段内容（§8.3）；
   凡是能改动其中任一项的动作都让主机进入 `drift`（页面「待下发（已变更）」）。
2. **下发是目录级全量对账**：agent 把 `/etc/filebeat/inputs.d` 恢复成"本次清单"，
   不在清单里的 `*.yml` 一律删除（含手工放进去的文件、改名后的残留文件）；
   有写入或删除才 `filebeat test config` + 重启（`builtin_actions.go:223-249`）。
3. **指纹一致则跳过**：不碰主机、不重启，只刷新 `last_applied_time`（`log_target_actions.go:711`）。

片段文件粒度是**服务 × 日志定义**（`<app code>__<svc code>__<日志定义 name>.yml`，
`log_config_render.go:65`），文件内每个部署实例一个 filestream input——所以"关掉一个服务"
等于删掉它的全部 `<app>__<svc>__*.yml`，不会碰到别的服务。

| 你做的动作 | 期望指纹 | 配置态 | 下发时主机上的动作 | 数据流 / 数据 | 配置自动下发建议 |
|---|---|---|---|---|---|
| 新增部署实例（扩容） | 变（多一个 input） | 待下发 | 该实例的 input 加进片段，Filebeat 重启 | 流不变，新实例数据自动并入 | ✅ **建议自动（第一批，优先级最高）**：§6 承诺"新增实例自动继承采集配置、扩容后不会漏采"，现状是每次扩容都留一次人工下发，是**漏采的主要来源** |
| 关闭采集（服务总开关 / 服务弹窗里关掉该服务的某条日志） | 变（该服务片段消失） | 待下发 | 该服务全部 `*.yml` 被删，Filebeat 重启；其他服务不受影响 | 流停写；已写入数据按档位到期，**不清** | ✅ 建议自动（第一批）：纯减法、可逆、不丢数据 |
| 改日志路径 / 宏值（模板 `path_pattern`、服务级 `macro_values`、实例 `runtime_variables`） | 变 | 待下发 | 同名片段内容更新（`paths` 换新路径），Filebeat 重启 | 流不变；`log_path` 从新文档起变；**旧路径不再采集**；新路径按 Filebeat 规则从头读 | ✅ 建议自动（第一批）：改动局限在该服务实例，用户的意图就是"立即生效" |
| 改多行参数（模板日志定义上的 `multiline_enabled` / `start_pattern` / `flush_timeout` / `input_format`） | 变（`parsers:` 段变） | 待下发 | 片段内容更新，Filebeat 重启 | 流不变；多行合并行为在下发后才变 | ✅ 建议自动（第一批）：改完不生效是明显的用户困惑（多行只能在发送前合并） |
| 开启采集（重新开启） | 变回 | 待下发 → 已同步 | 片段重新写入（内容与关闭前相同也照写），Filebeat 重启 | **同一个流继续写入**（流名未变，不新建流）；registry 保留 offset，通常从断点继续，暂停期间的日志会被补采（除非文件被轮转/截断/改名） | ✅ 建议自动（**第二批**）：与关闭对称，但它引入**新写入**（ES 容量、档位预算需先有预检与量级提示） |
| 新增日志定义（模板加一条日志） | 变（多一个片段） | 待下发 | 新增 `<app>__<svc>__<新日志>.yml`，Filebeat 重启 | 流不变（档位相同则同流） | ✅ 建议自动（**第二批**）：模板日志定义没有开关，新增后该模板下所有服务**默认采**（§6），未挂处理规则时才不采；因此自动下发前必须先做预检，否则会静默开始采集 |
| 改解析规则，**仅改 `pipeline_body`** | **不变** | **不进入待下发** | **无主机动作、不重启**：保存规则时已 `PUT _ingest/pipeline`（§5.2） | 新写入立即用新解析；**历史文档不重新解析**（无 reindex） | ➖ 无需下发（本来就即时生效，唯一"已经自动"的一类） |
| 换规则标识（改名 / 换应用 / 换集群 → `pipeline` 名变），或在模板日志定义上换挂另一条规则 | 变（`pipeline:` 行变） | 待下发 | 片段内容更新，Filebeat 重启 | 流不变；新旧数据的解析产物不一致 | ⛔ 保持人工：模板换规则影响所有引用服务（服务侧没有覆盖，§6）；规则本身还可被多个模板复用 |
| 改保留档位（服务级或日志级） | 变（`index` 行变） | 待下发 | 片段内容更新，Filebeat 重启并开始写新流 | **新流**；旧流停写并按原档位到期，**不迁移数据** | ⛔ 保持人工：语义不可逆，必须显式确认 |
| 删除模板里的日志定义 | 变（片段消失） | 待下发 | 对应 `*.yml` 被删，Filebeat 重启 | 流不变，已有数据仍在服务流里、仍可检索 | ⛔ 保持人工：删定义会**级联清掉各服务对它的覆盖值**（档位/规则/开关/过滤），影响面跨服务 |
| 改日志定义 name（模板日志改名） | 变（片段**文件名**变） | 待下发 | 旧 `<app>__<svc>__<旧名>.yml` 被删、新文件写入，Filebeat 重启 | 流不变；`log_name` 从新文档起变（历史文档保留旧名，按 `log_name` 清理时要注意两代名字） | ⛔ 保持人工：id 不变所以服务级覆盖全部保留，但主机的片段文件与 `log_name` 维度会换一茬 |
| 改模板的宏默认值 / 应用主目录（`macro_definitions` / `app_home`） | 变（未在服务/实例覆盖该宏的实例路径变） | 待下发 | 片段 `paths` 换新路径，Filebeat 重启 | 同"改路径"：流不变、`log_path` 变、旧路径停采 | ✅ 建议自动（第一批，与改路径同类） |
| 换服务所属部署模板 | 变（期望配置整体换模板） | 待下发 | 旧模板的片段被删（同名文件则被覆盖），新模板的片段写入，Filebeat 重启 | 流不变（档位不再变时）；新模板的日志定义**默认全采**（§6） | ⛔ 保持人工：一次改动影响该服务全部主机与全部日志。注意编辑既有服务时，弹窗里的日志表格仍显示旧模板的行，需重开弹窗才刷新 |
| 停用部署模板（`enabled=false`） | **不变** | 不影响配置态 | 无 | 不变 | ➖ 无需下发：渲染查询不检查模板 enabled，停用只让新建/改服务时下拉选不到它，**已在采的服务继续采** |
| 停用 / 删除逻辑服务、删除部署模板 | 变（批量片段消失） | 待下发 | 该服务（或该模板下全部服务）的片段被删，Filebeat 重启 | 流变孤儿（维度已删 → 存储水位页"未识别"） | ⛔ 保持人工：影响面是该服务的全部主机，且与资产生命周期动作耦合 |
| 默认 ES 集群输出段变更（地址 / 账号 / `verify_tls` / `index_prefix`） | 变 | **全部纳管主机**待下发 | 重写 `filebeat.yml` + 全部片段，Filebeat 重启 | 前缀变则流名全变 | ⛔ 保持人工（最危险）：全网影响、新集群不可达时整体采集中断 |
| 只改 ES 口令 | **不变** | 不进入待下发 | 无 | 不变 | ⛔ 人工重新下发一次（指纹刻意不含口令，§8.3） |
| 停 Filebeat 进程 / 卸载 Filebeat | 不变 | 不影响配置态（属运行态） | 无 | 停写 | ⛔ 人工（安装/卸载已作业化） |

几个与直觉不符的点：

- **采集开关只有服务侧，且默认是"采"**：模板日志定义上没有开关（§6，迁移 000034 删列）。
  服务弹窗里那条日志的开关默认为开，关掉才写覆盖行（`collection_enabled=false`）。
  推论：**给模板新增一条日志、给服务新增一个实例，都会自动进入采集范围**——这正是"不漏采
  优先"的取舍，闸门是"未挂处理规则不采集"（模板里新加的日志先不挂规则就不会被采上来）。
- **解析规则只归模板，服务侧只读**（2026-09-19，迁移 000035）：服务弹窗的「处理规则（模板）」列
  是只读文本，渲染取 `ld.processing_rule_id`（§6）。所以**改模板的处理规则一定对所有引用服务生效**
  （不再有"服务覆盖过的服务不受影响"这回事）；只改规则自身的 `pipeline_body` 仍然不经过模板
  （即时生效、不需下发）。连带注意两点：模板日志定义**没挂规则时服务无法自救**，必须回模板挂；
  而一条规则可以被多个日志定义/模板复用，所以改 `pipeline_body` 的影响面可能超过当前模板。
- **模板保存是"按 id 增量"，不是整表删重建**（2026-09-19 改）：保存模板时后端把提交的日志列表
  分成增/改/删三组（`applyTemplateLogWrites`）——带 `id` 的原地更新（**id 不变，服务级覆盖行
  因此不会失效**）、不带 `id` 的新增、库里存在但本次没提交的删除。删除时会**先级联清掉引用该
  定义的覆盖行**再删定义（外键无 `ON DELETE CASCADE`，不清就删不掉）；顺序是先删后改再插，
  这样"改名到刚删掉的名字"也能一次保存成功。两条相关约束：提交的 `id` 必须属于本模板
  （跨模板 id 会被拒），日志定义**名称不能为空**（空名会生成 `__svc__.yml` 这种采不到文件的片段）。
  历史背景：改动前是每存一次就换一批 id，于是（a）改个路径也会让服务的档位/规则覆盖失效，
  （b）"改名"与"删除"在配置层无法区分，（c）只要任何服务留过覆盖值，保存就会被外键拒绝
  （计划 §1 第 8 条）。
- **未挂处理规则的日志不采集**：渲染跳过并产出 warning，下发时该片段文件被删（§8.4）。
- **路径清空不跳过**：只有"展开后仍含 `${VAR}`"才会跳过该实例；`path_pattern` 为空串会生成
  `paths: - ''` 的片段且**没有告警**（要么 Filebeat 校验/启动失败、下发停在待下发，要么起一个采不到文件的 input）。
- **关掉采集 ≠ 立刻停写**：开关只改期望；主机上的 Filebeat 仍按旧配置采集，直到真正下发。

**自动下发的判据**（四条同时看）：① 影响面是否只落在**一个逻辑服务的部署实例**上（几十台级）；
② 语义是否可逆（减法/幂等 vs 不可逆）；③ 是否引入新写入（容量）；④ 误配置能否**静默**（停采/漏采）。

据此的建议是三档：

- **建议自动（第一批）**：新增部署实例、关闭采集、改路径/宏、改多行参数。都在单服务范围内、可逆、
  用户意图就是"改完生效"；其中**新增实例优先级最高**（现状 = 每次扩容都留一次人工下发，漏采风险最大）。
- **建议自动（第二批，需先具备预检护栏）**：开启采集、新增日志定义。与"关闭/删除"对称，
  但前者引入新写入、后者未挂规则会静默不采集。
- **保持人工**：换规则标识/换集群、改档位、删除日志定义（前置未完成）、停用删除服务与模板、
  集群输出段、口令、安装卸载。计划 §9 第 8 条"不自动"的原始理由
  （误配置瞬间放大到全网、重启 Filebeat 的噪声无法收敛）**只对这一批成立**。

若落地自动下发，护栏缺一不可：① 触发点在资产写路径**提交成功之后**，不在渲染路径里；
② 复用批量作业队列 + 有界并发（`LOG_BATCH_APPLY_CONCURRENCY` 默认 5）+ **时间窗合并**
（同一主机 N 分钟内多次变更合并成一次下发），避免连续改配置把 Filebeat 反复重启；
③ 预检不通过不自动（渲染 warning 非空、片段为空、没有启用的默认集群、agent 离线）→ 落回
"待下发 + 原因"，可见性不变；④ 幂等靠现有指纹比对（一致即跳过）；⑤ 失败不回滚配置、
停在 `drift`，保留「一键下发」兜底。

> **状态**：以上**是建议，尚未实现**。当前实现只有"安装成功后自动下发一次"（§8.4）
> 与人工单条/批量一键下发（§8.8）；配置保存路径上没有任何自动下发触发。

---

## 9. 查询与洞察

### 9.0 存储水位（data stream 运行态）

前端「日志管理 → 存储水位」（`/monitor/logging/overview`，迁移 000019），左侧层级树
`顶层 → 项目 → 业务系统 → 环境 → 逻辑服务`，右侧按层级展示。

**数据口径（关键）**：真实磁盘占用/rollover 状态的原子粒度是 data stream
（命名 = `autoadmin-<项目>-<业务系统>-<环境>-<档位编码>`，见 4.1）。树的顶层/项目/业务系统/环境层
都是流的真实聚合；**逻辑服务层只有写入量（文档数）口径**——服务是流内字段不是索引维度，
不存在按服务的真实磁盘拆分，UI 必须明示该差异。

- 数据来源：`GET /monitor/elasticsearch-clusters/:id/log-storage-overview/`
  （`datastream_status.go`）聚合 `_cat/indices`（流大小/docs/健康）、
  `_ilm/explain`（rollover/ILM 状态）、`_cat/allocation`（节点磁盘水位，
  失败不阻塞总览），并从 MySQL 带回项目/业务系统/环境维度数据，前端组装树。
  `dims` 含四层：projects / business_systems / environments / **services**。services 曾在
  2026-09-18 因"前端从未消费"被删（服务层用流名解析出的服务码渲染）；2026-09-19 日志中心的
  容量统计需要一个"每层成员清单"来**把没有日志的分组也列出来**（"这个项目一条日志都没有"
  本身就是要看的信息），并且"某业务系统下的环境"只能由它名下服务反推——环境是服务上的属性
  （`assets_business_environment` 不挂在业务系统下）。消费方出现了，所以连同查询一起加回来；
  契约测试 `TestStorageOverviewDimsCarryAllFourLevels` 钉住"别再把这份 payload 当死数据删掉"。
- **"已停用 / 未开启采集"按逻辑服务标注（2026-09-18）**：每条流带
  `service_enabled` 与 `service_collection_enabled`（来自逻辑服务行，是**配置事实**，
  不需要查 ES）。服务停用或服务级采集开关关闭时，树上服务节点与右侧明细都标「已停用」/
  「未开启采集」，tooltip 说明"已写入的数据按保留档位到期、不会自动清理"——这是计划 §3 的语义
  （关闭采集 = 停写 + 保留到期，不删数据），避免运维把停用服务的存量数据当成待清理对象。
  正常采集不标（"在采集"是数据态，按 §2.4 不在未查询 ES 时断言）。旧命名流没有服务段，
  识别不出服务，因此不带这两个标记。
- 流名解析：后备索引名形如 `.ds-<流名>-<代数>`（或传统 `<流名>-<YYYY.MM.DD>`），先剥离
  `.ds-` 前缀与后缀还原流名，再用数据库维度码做前缀匹配（`streamNameMatcher`，编码可含
  连字符，禁止按 `-` 盲切）：新命名按 服务 维度命中，流即服务本身；旧命名（无服务段）
  要求剩余段恰好是已知档位。两种都命中不了才归"未识别"节点（手工建的、维度已删/改名的），不丢数据。
- **识别候选集不按 `enabled` 过滤（2026-09-18 修复）**：构造候选用 `ListServiceStreamDims` /
  `ListServiceStreamRows` / `ListRetentionTierCodes`，三条都**不过滤** `enabled`。
  识别回答的是"这条已有的流属于哪个已知服务"，与"这个服务现在是否在采集"无关：停用是可逆状态，
  存量流要么还在写（尚未重新下发配置）要么停写并保留到 ILM 到期（§0 原则「停止采集 ≠ 删除数据」）。
  早先 `ListServiceStreamDims` 带 `WHERE s.enabled = TRUE`，停用服务的流在页面上直接从所属
  项目/业务系统掉进「未识别」，等于把暂停采集的存量数据标成了待清理的孤儿
  （真实案例：停用 `yilake nginx` 后 `logs-yilake-tib-poc-nginx-wuhan-test` 被判未识别）。
  对照：**下发**路径（`ListHostLogRenderEntries`）照旧过滤 `s.enabled = TRUE`——停用的服务
  不该再往主机推片段。守卫用例：`internal/logcollect/stream_recognition_guard_test.go`。
- **逻辑服务层（新命名流）展示真实磁盘占用**（流即服务本身）；旧流的叶子层只有写入量
  （文档数）口径，UI 明示"未按服务分流的旧流"。
- `GET .../log-service-usage/?business_system=&environment=`：环境节点展开时按需调用，
  `service` 字段 terms 聚合文档数（默认近 30 天，仅叶子层使用），索引匹配用
  `<prefix>-*<业务系统>-<环境>-*`（项目段通配）。
- 手动刷新，无轮询（聚合查询对集群有成本）。

### 9.1 自动错误清单

不需要输入关键词，只限定时间范围与级别：

```json
{
  "size": 0,
  "query": { "bool": { "filter": [
    { "terms": { "log_level": ["ERROR", "SEVERE", "FATAL"] }},
    { "range": { "@timestamp": { "gte": "now-1h" }}}
  ]}},
  "aggs": {
    "patterns": {
      "terms": { "field": "error_fingerprint", "size": 50 },
      "aggs": {
        "sample":   { "top_hits": { "size": 1,
                      "_source": ["error_type", "error_template", "service", "instance"] }},
        "services": { "terms": { "field": "service" }}
      }
    }
  }
}
```

### 9.2 新增错误识别

比「哪个错误最多」更有价值的是「新出现的错误」，通常意味着刚发布引入了问题。
使用 `significant_terms` 对比历史背景频率，自动排除常态错误：

```json
{
  "size": 0,
  "query": { "range": { "@timestamp": { "gte": "now-1h" }}},
  "aggs": {
    "unusual_errors": {
      "significant_terms": {
        "field": "error_fingerprint",
        "size": 20,
        "background_filter": { "range": { "@timestamp": { "gte": "now-7d", "lt": "now-1h" }}}
      }
    }
  }
}
```

不需要设置阈值。

### 9.3 突增检测

`terms` 嵌套 `date_histogram` 取时序，在 djadmin 侧比对最近一个桶与前 N 个桶的均值。
不需要 ML 插件。

### 9.4 实例分布下钻

```json
"aggs": {
  "by_error": {
    "terms": { "field": "error_type", "size": 10 },
    "aggs": { "by_instance": { "terms": { "field": "instance" }}}
  }
}
```

用于区分「代码缺陷」（各实例均匀分布）与「单机环境问题」（集中在某台）。

### 9.5 页面形态

**两个入口，同一批判定口径**：

1. **日志中心**（`/monitor/logging/center`，菜单「日志管理 → 日志中心」，迁移 000037）：左侧是所有
   资产页共用的服务树（`ServiceTree`，用它的 `groupByProject` 选项显示项目层级），右侧三个 tab：

   | tab | 数据来源 | 说明 |
   |---|---|---|
   | 日志配置 | `GET /assets/application-services/:id/log-config/` | 日志名/路径/处理规则/采集开关/档位/格式认证状态/data stream + **服务级采集总开关**（`log_collection_enabled`，响应顶层字段）。总开关与逐条开关是两层：前者关掉后该服务下所有日志都不采集、逐条开关不生效（配置意图保留）。两者都可直接改（见下方「两条写路径」）；认证入口复用共享组件 `LogFormatVerifyDialog`（与编辑弹窗同一个，见 §4.8） |
   | 日志查询 | `LogQueryPanel`（与入口 2 同一个组件） | 检索接口硬性要求 `application_service_id`，由树的选中节点提供 |
   | 存储水位 | 选中服务时 `.../log-storage-overview/?service_code=<服务编码>`（后端收窄 ES 查询）；未选中时取全量再按树的层级（项目/业务系统/环境）用 dims 映射到编码过滤 | **按层级聚合的容量统计**（全部→项目、项目→业务系统、业务系统→环境、环境→逻辑服务；占用降序 + 占比 + 其中历史档位 + **同名饼图**）、**集群 + 数据时间**、统计（流数/文档数/总占用拆分活跃与历史/**健康异常流**）、**Elasticsearch 节点磁盘水位**、流表（占用/文档数/ILM/**状态**/**后备索引展开**）。选中服务时另给历史流的切回/清理动作；未选中时列出**未识别流**（不归属任何服务，任何层级都保留，不进聚合） |

   **聚合分组的集合来自 dims，不是来自流**：每层**已配置的成员都列出来**（含 `streams = 0` 的，标「无日志」），再补上"流里有、维度表里没有"的编码（维度停用/删除后的遗留流，标「维度已停用」）——两个方向都不能漏，漏了前者看不到空分组、漏了后者这些流会从统计里消失而明细表里还在。饼图只画**有占用**的分组（占比对 0 没有意义，零值分组仍留在表格里），超过 8 项合并为"其他 N 项"。其余口径：未识别流不进聚合；`historical` 由后端判定（见下）。 |

   三个 tab 共用一个服务上下文，这是把它们合成一个页面的理由：日志检索接口必填
   `application_service_id`，水位与日志配置也都以服务为维度，而原先它们分散在三处（存储水位页、
   服务树页的日志查询 tab、逻辑服务编辑弹窗的模板日志表），看同一个服务的日志要来回跳。

   **水位按服务收窄是后端做的**：带 `service_code` 时后端用识别环节算好的维度段把 ES 查询收窄成
   `<前缀>-<项目>-<业务系统>-<环境>-<服务>-*`（`scopeIndexPattern`），而不是取全量再前端过滤——
   `_cat/indices` + ILM explain 是这一页最大的成本。**不带该参数时行为与以前完全一致**（全量视图，
   存储水位页在用）。服务编码在库里全局唯一，所以它是可靠的连接键；编码不存在时接口返回空视图
   而**不回落到全量**（否则调用方会以为看到的是"这个服务的流"）。水位只在切到该 tab 或手动刷新时取一次。

   **旧的「存储水位」菜单的能力已全部并入本页**（集群与数据时间、节点磁盘水位、健康异常流计数、
   未识别流容器、后备索引明细）；旧页保留是因为它还有"顶层 → 项目 → 业务系统 → 环境 → 逻辑服务"
   的层级树导航，适合"从容量视角自顶向下看"，而本页是"从服务视角看"。
   两页消费的是同一个接口，所以数字永远一致。

   **历史流必须看得出来**：改档位不会迁移数据——旧流停止写入、按原档位保留到期后由 ILM 删除。
   所以同一个服务常常同时有"当前档位"与"历史档位"两条流，而且历史流往往是占地最大的那条
   （现场：nginx 从 `wuhan-test` 改成 `hot` 之后，旧的 `...-nginx-wuhan-test` 留着 1.0 GB / 475 万条，
   新流只有 55 KB）。页面靠 `log-config` 响应里的 **`tier_code`**（生效档位 = 覆盖档位 → 服务默认档位
   → `is_default` → `std` 的 COALESCE 链）判断：流的档位不在本服务当前生效档位集合里，就是历史流。
   判定顺序是"先历史、后服务开关"，否则停写的老流会被标成绿色的"采集中"。
   **判定在后端做**（`historical` 字段，见 `isHistoricalStream`）：只有后端知道该服务当前生效的
   档位集合（覆盖档位 → 服务默认 → `is_default` → `std` 的 COALESCE 链，与 `log-config` 的
   `tier_code` 同源），而且这个判定在**全局视图**里同样成立——前端拿不到全量视图的生效档位，
   自己推会把所有流都误标成历史档位，这正是把判定下沉的原因。字段缺失（旧构建）时按"不是历史流"
   处理：宁可少标，也不要把正在写的流错标成停写。
   汇总里的磁盘占用会把历史流的条数单独点出来，避免用户看不懂占用为什么这么大。

   注意：这个判定在**流/档位**粒度上是精确的（同一档位下多条日志共用一条流），但存储水位页
   （全量视图）没有逐条日志的生效档位信息，所以那页不做这个标记——它只有服务级开关事实。

   **历史流怎么处置**（页面给了两个动作）：

   | 情况 | 做法 |
   |---|---|
   | 以后可能切回该档位 | 什么都不用做。流名由生效档位决定（`LogDataStreamName(..., 档位)`），切回该档位就**继续写入同一条流**，不会新建一条空流；「历史档位」标签是活状态，切回来自动变回「采集中」。页面提供「切回该档位」按钮：勾选要改的日志（同档位多条日志共享一条流），逐条按行提交。 |
   | 永远不切回 | 多数情况也不用管：档位 ILM 的 hot 阶段 rollover 带 `max_age`（= 档位的 `rollover_min_index_age`，**档位表单必填**、校验 `^\d+[mhd]$`、库里 NOT NULL），按时间触发不依赖写入，所以停写的流照样 rollover 并进入 delete 阶段到期删除。只有"保留期长 + 数据大 + 现在就要释放"才需要动手：历史流行上的「立即清理」按 (服务, 档位) 删掉这条流的文档（见 §9.6）。 |

   两个动作都要说清一件事：**下发对历史流无效**——它已不在下发范围内，下发只影响未来的写入。
   占用在汇总里拆成"活跃 / 历史"两个数（前者是当前成本，后者是待释放的沉淀），合成一个数
   既看不出问题、也没法判断该不该清理。

   **没有日志定义时也不隐藏存量流**：模板里的日志定义被删光之后，这个服务什么都不采了，但 ES 里
   的存量流还在。水位查询用的是 `log-config` 响应**顶层**的 `service_code`（而不是 `logs[0].service_code`），
   因此照样能查到；此时没有生效档位，页面把所有存量流都标成历史档位。判定里还留了一条：后端
   没给生效档位（旧构建）时不妄判，宁可沿用“采集中”也不要把在写的流错标成停写。

   **边界**：本服务水位只含“按新命名能识别到本服务”的流；旧命名流与未识别流没有服务归属，
   仍要看「存储水位」页的全量视图与它的「未识别」分组。**下发**在本页有服务级入口（聚合状态 +
   一键重下发承载主机），但底层仍是逐台全量下发，见 §8.7；按单台主机下发仍在「日志采集」页。

#### 9.5.1 两条写路径，以及为什么不合并

日志覆盖值（采集开关 / 保留档位）有两条写入通道，**语义不同、各有明确适用范围**：

| 通道 | 语义 | 允许的调用方 |
|---|---|---|
| `SaveApplicationService` 的 `log_settings`（`PATCH /assets/application-services/:id/`） | **整表替换**：提交的集合即该服务覆盖行的全量，实现是 `DELETE WHERE service_id` 后逐行重插；与服务主体同一个事务 | **只允许“完整表单”**（逻辑服务编辑弹窗，它渲染并提交模板下的全部日志行） |
| `POST /assets/application-services/:id/log-config/settings/`（`SaveServiceLogOverride`） | **按行 upsert**：只写这一条 `(服务 × 日志定义)` 的采集开关 + 档位，不影响同服务其他行，也不碰 `format_verified_*` | 按行操作（日志中心页的内联开关/档位、将来的批量开关） |
| `POST /assets/application-services/:id/log-collection/`（`SetServiceLogCollection`） | **单列 UPDATE**：只写服务行的 `log_collection_enabled` + `update_time`，不碰服务其他属性 | 服务级采集总开关（日志中心页顶部那个开关） |

为什么不把编辑弹窗也切到按行写（统一成一条路）：那会丢掉“服务主体 + 日志设置在同一事务里提交”的
原子性——弹窗的保存是一个表单动作，改成按行就得发 N 个请求、部分失败无从回滚。而整表替换在
“调用方提交的是完整集合”这个前提下既正确又更简单。所以两者的边界靠**接口语义**划开：
整表替换只接受完整集合，按行接口在表达上根本做不到“删除其他行”。

代价是“往整表替换接口发部分集合会静默删覆盖值”。这条靠三处兜住：弹窗是唯一调用方且它渲染全量；
按行接口只写两列；以及守卫测试 `TestUpsertServiceLogOverrideKeepsFormatVerification` 钉住
“按行 upsert 的 SQL 里不许出现 `format_verified_*`”（改采集开关/档位不进认证指纹，认证状态必须保留）。

另外**按行接口把提交体当作该行覆盖值的全集**：缺的那一列会被写成 NULL（= 不覆盖），
所以调用方要么传全，要么合并当前值（前端 `saveOverride` 做的就是这件事）。

服务级总开关是第三条通道，原因和第二条一样：它原先只能随**整份服务表单**
（`UpdateApplicationService`）提交，而那个接口的 PATCH 校验要求应用/版本/模板/名称/编码等全字段，
"只想开关采集"就得伪造一份完整表单——既容易覆盖掉别的字段，也没法在页面上做成一个开关。
守卫测试 `TestUpdateServiceLogCollectionOnlyTouchesItsColumn` 钉住这条 UPDATE 只写那一列。

2. **服务树内的「日志查询」tab**（`/assets/service-tree`）：保留的快捷入口，排障时从服务/实例节点
   直接进，且选中实例节点会自动预填 `instance` 过滤。它满足的是"进入即展示，无需用户输入"：

```
近 1 小时
  新增错误 N 类       ← significant_terms
  突增错误 N 类       ← date_histogram 比对
  错误模式 / 次数 / 趋势 / 实例分布   ← terms + top_hits
```

告警接入现有 `AlertRoute` / `AlertMedia` 通知链路。

**Elasticsearch 能力边界**：Elasticsearch 的 `categorize_text` 聚合可做完全无监督的日志
聚类，Elasticsearch 由 ES 7.10 fork，不包含该功能。因此聚类质量完全取决于 ingest 阶段的
fingerprint 归一化质量。

### 9.6 数据流清理（Go 版，2026-09-17）

逻辑服务维度的历史数据清理，让用户自助释放存储，不用 DBA 上 ES 手删。

- **按流清理（可选 `tier`，2026-09-19）**：`POST /monitor/log-datastreams/cleanup/` 的 body 支持
  可选 `tier`（档位编码），把范围从"该服务所有档位"收窄到**某一条流**。用于回收"改了档位、
  不打算再切回"的历史流——它本来会按自己档位的保留期由 ILM 到期删除，但保留期长/数据大的时候
  用户希望立刻释放。**档位编码必须先命中档位表**才能拼进索引模式（它直接进 ES 的索引模式，
  放行任意字符串等于把"删任意索引"的口子重新开出来）；语义仍是"删这条流里的文档"而不是
  "删掉 data stream 对象"——流本身留着（变空），与 `mode=all` 的既有行为一致，切回该档位继续写它。
  入口在日志中心页「本服务水位」tab 的历史流行上（只对历史流给这个动作）。
- 入口：**日志查询界面**（`LogQueryPanel`，选中逻辑服务/部署实例后）头部的「清理日志数据」按钮。
  该组件现在有两处宿主：服务树页的「日志查询」tab 与日志中心的「日志查询」tab（§9.5），
  两处入口、同一份清理逻辑与确认流程；后端 `POST /monitor/log-datastreams/cleanup/`
  （`log_datastream_cleanup.go`，继承 `monitor:view`）。不放编辑弹窗（那里是编辑态，不适合破坏性操作）。
- 请求体只接受 `{service_id, mode, amount}`：`mode=all` 清空；`mode=hours|days` 保留最近
  N 小时/天（`amount` 上限 87600 小时 / 3650 天）。**不接受客户端传索引名**，流名由后端按
  service_id 解析维度码后拼 `<prefix>-<项目>-<业务>-<环境>-<服务>-*`（档位段 `*` 通配，
  覆盖换过档位的历史流）。
- 执行方式：对匹配的数据流发 `_delete_by_query?wait_for_completion=false&conflicts=proceed&refresh=false`，
  异步后台执行（同步删大量数据会超时），**保留 data stream 本身**（不删 backing index/流），
  避免 Filebeat 重建流时的空窗与 ILM 绑定问题；按 `@timestamp` 判定（与写入时区无关，存 UTC）。
- 失败语义：没有任何匹配的数据流（ES 404 `index_not_found`）视为已清理、返回 `matched=false`；
  其余 ES 报错原样返回 400。返回 `{stream_pattern, mode, amount, task, matched}`。
- 破坏性操作，前端二次确认（不可恢复提示 + 范围选择）。
- **范围必填、无服务端默认**：`mode` 是必填参数，`all` 也必须由用户显式选择——计划中"清理不默认全清"
  的结论与现状一致（[LOG_COLLECTION_LIFECYCLE](../plans/LOG_COLLECTION_LIFECYCLE.md) §9 第 3 条）。
- **权限现状与目标**：当前该接口只继承 `monitor:view`（`router.go:366`），意味着只读权限即可删数据。
  计划按"破坏性动作整体拆权限"修正（数据清理 / 目标删除 / 停止服务 / 配置下发），见计划文档 §7、§9 第 4 条。

---

## 10. 代码组织

采集与存储两条链路的实现在 **`autoadmin/internal/logcollect/`**，不在 `internal/monitor/`。
拆分的依据是职责边界：日志采集/存储自成一套对象（Filebeat 纳管目标、ES 集群、日志定义、数据流），
与监控域的 exporter 目标、告警/通知、软件包仓库没有共享状态。

| 文件 | 职责 |
|---|---|
| `log_config_render.go` | 片段渲染与指纹（**纯函数、不访问数据库**，与 SQL 解耦便于单测） |
| `log_target_actions.go` | 纳管目标的安装/卸载、启停、配置下发、批量操作 |
| `log_health.go` | 链路体检六层对账 |
| `log_management.go` | 索引模板、ILM 策略、bootstrap |
| `log_datastream_cleanup.go` | 数据流按服务/日志文件/时间窗清理 |
| `datastream_status.go` | 存储水位：流级运行态与维度树 |
| `elasticsearch.go` | Elasticsearch 客户端、日志检索、聚合 |
| `elasticsearch_config.go` / `elasticsearch_pipeline.go` | 集群配置 CRUD、pipeline 模拟 |
| `config_resources.go` / `config_resource_writers.go` / `config_resources_typed.go` | 保留档位 / 处理规则 / 采集过滤规则的通用配置资源 CRUD |
| `collect_target_dialect_{mysql,postgres}.go` | `createLogCollectionTargetIfAbsent` 的按方言实现（`INSERT IGNORE` vs `ON CONFLICT DO NOTHING`） |
| `smoke_log_test.go` | 真库冒烟（`MONITOR_SMOKE_DSN`，事务内回滚） |

配套的共享与归属说明：

- `internal/shared/logstream`：data stream 命名的唯一构造函数，`logcollect` 与 `assets` 共用
  （任何生成或解析流名的地方都不得自行拼接）。
- `internal/shared/filebeat`：Filebeat 内置安装/卸载 Playbook 与 systemd unit 内容。
  软件包仓库（`internal/monitor`，创建/编辑/回填软件包配置）与采集派发（`logcollect`，unit 兜底）共用。
- `internal/monitor/` 保留：exporter 纳管目标、告警历史与通知、Prometheus 代理、软件包仓库、
  主机列表与安装历史（主机列表按 SQL 直读采集目标列，不反向依赖 `logcollect`）。
- 依赖方向：`router` 构造两域共用的依赖（数据库、gateway、automation、凭据加解密器、软件包根目录）
  后分别注入；`logcollect` 不 import `monitor`。两域挂在同一个 `/monitor` 路由组下共用组级鉴权，
  但 URL 完全不变。
- 各域自备同名小工具（`parseID` / `ids` / `firstNonEmpty` / `deleteRowsAffected` 等）是仓库既有约定
  （assets、automation、monitor 等包此前就是各自一份），不为两行函数建公共包。

---

## 11. 复用现有能力

| 需求 | 复用 |
|---|---|
| Filebeat 安装、卸载、状态检查 | monitor 的 `SoftwarePackage` + `MonitorTarget` 纳管体系 |
| 配置文件下发 | dj-agent gRPC 文件写入 `write_open` / `write_chunk` / `write_close` |
| 服务状态检查 | dj-agent `check_exporter_status` 模式 |
| 路径变量展开 | `${APP_HOME}` / `${INSTANCE_NAME}` 现有机制 |
| 告警通知 | `AlertRoute` / `AlertMedia` |

不新建独立的安装纳管体系。

---

## 12. 实施状态

| 阶段 | 内容 | 状态 | 说明 |
|---|---|---|---|
| 1 | Elasticsearch 连接、index template、ILM policy | 已完成 | 支持集群连接测试和幂等 bootstrap |
| 2 | 统一日志处理规则、Pipeline 发布、`_simulate` 调试 | 已完成 | 页面明确区分发送前处理与 Ingest，仍只保存一条规则 |
| 3 | 数据模型与迁移 | 已完成 | `LogProcessingRule` + 单一 `processing_rule` 外键 |
| 4 | Filebeat 软件包仓库、离线安装和状态检查 | 已完成 | 按平台、主版本和架构精确匹配，不依赖目标主机联网 |
| 5 | 配置生成、指纹比对、下发和热重载 | 已完成 | 输入、offset、输出按四段 Tag 隔离；单条与批量走同一条全流程，指纹覆盖输出段 |
| 6 | 服务级开关、批量应用、清理和实例日志读取 | 已完成 | 经 dj-agent gRPC 执行；批量下发/安装已作业化（入队 + 有界并发 + 进度 + 续跑，见 §8.8），批量启停/删除仍是同步批量 |
| 7 | 日志洞察页面与告警接入 | 进行中 | 聚合查询接口已具备，页面和告警闭环继续完善 |

解析规则调试仍是后续扩展的回归基线：新增日志格式必须先用真实样例通过 `_simulate`，再关联
日志定义并应用 Filebeat 配置。

---

## 13. 风险与注意事项

| 项 | 说明 |
|---|---|
| 日志读取权限 | 应用日志属于 `esb` 等业务用户，Filebeat 需以 root 运行或配置 ACL。安装检查时应一并验证可读性，避免配置下发成功但无数据 |
| 同主机路径冲突 | 同一主机上多个实例日志文件名可能相同，下发前必须校验展开后的绝对路径唯一，否则 Filebeat 会产生 harvester 冲突 |
| mapping 字段膨胀 | 业务附加字段统一使用 `labels_` 前缀；需聚合的字段提升为固定字段，并设置 `total_fields.limit` |
| 时间戳 | 必须在 pipeline 中用 `date` processor 覆盖 `@timestamp`，否则记录的是采集时间而非日志产生时间 |
| 容器化采集器 | 若 Filebeat 以容器运行，仅能看到挂载路径。下发前需校验目标路径落在已挂载前缀内 |
| 磁盘水位 | Elasticsearch 磁盘超过水位会将索引置为只读，生产环境需保留水位检查并配置 ILM 自动清理 |
| TLS 证书 | 自签证书阶段使用 `tls.verify Off`，生产需分发 CA 证书并开启校验 |
| 规模（500–1000 台） | 批量下发当前在**单个 HTTP 请求内串行**执行、每台 2 次 agent gRPC（超时 60s+120s）；批量安装/重试对每台内联起 goroutine、**无并发上限**且绕过 `worker` + `WorkerPrefetch` 限流。主机列表服务端分页（`page_size` ≤30），但"按配置状态筛选"需全量计算。改造方案见 [计划](../plans/LOG_COLLECTION_LIFECYCLE.md) §8 规模基线 |
