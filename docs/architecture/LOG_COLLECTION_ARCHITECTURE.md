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
      "index.mapping.total_fields.limit": 2000
    }
  }
}
```

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

```
固定字段（所有日志一致，可聚合）
  @timestamp, message, log_level, logger_name, thread_name, process_id
  error_message, error_template, error_fingerprint, stack_trace
  exception_type, exception_message, root_cause_type, root_cause_message
  project, business_system, environment
  service, instance, host_ip
  application, version, log_name, log_path

业务特有字段
  labels_<key>    由日志定义的 extra_fields 注入
```

业务附加字段统一增加 `labels_` 前缀，避免与平台固定字段冲突。需要聚合或告警的字段应提升
为固定字段，并在索引模板中预先定义 mapping，不能让任意业务字段无边界增长。

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

`ApplicationLogDefinition.processing_rule` 是日志定义唯一的规则关联。同格式日志可复用规则，
Pipeline 数量不会随部署实例增长。

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

**必备字段校验**：处理规则产物必须包含 `error_fingerprint`（错误清单/聚类按它聚合），
调试结果里缺失会报 `missing_fields`；保存时后端静态检查 `pipeline_body` 是否含写入
`error_fingerprint` 的 processor（`fingerprint`/`set`/`copy`/`rename`，见 §5.3），没有直接 400
拦截发布。

dj-agent 具备文件读取能力，可实现「读取该实例最近 N 行日志」直接作为样例输入，
形成闭环。

**这项功能优先级最高**：后续所有自动错误发现能力都建立在 fingerprint 质量之上，
归一化规则不准会导致同类错误散成数百条，聚合结果不可用。

---

## 6. 开关粒度

采集范围由两层开关决定：

| 层级 | 字段 | 语义 |
|---|---|---|
| 部署模板 · 日志定义 | `ApplicationLogDefinition.collection_enabled` | 该条日志是否采集 |
| 逻辑服务 | `ApplicationService.log_collection_enabled` | 该服务是否开启采集 |

```
服务开关 ON  AND  日志定义 collection_enabled ON
    → 服务下所有部署实例均采集该条日志
```

**部署实例层不设开关**。同一逻辑服务下的实例配置一致是常态，HA 主备同样都需要采集；
确有差异时拆分为两个逻辑服务处理，不为罕见例外向所有正常场景引入配置维度。

附带收益：新增实例自动继承采集配置，扩容后不会漏采。

---

## 7. 数据模型改动

```
ApplicationService
  + log_collection_enabled    BooleanField(default=False)
  + log_retention_tier        档位 FK（Go: assets_application_service.log_retention_tier_id
                              → monitor_log_retention_tier.id，null=按 std 处理）

ApplicationLogDefinition      已存在 path_pattern / encoding / collection_enabled
  + processing_rule           ForeignKey(LogProcessingRule, PROTECT, nullable)
  + extra_fields              JSONField    附加标签

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
  → loadHostLogRenderInput：主机上启用采集的服务×日志定义（有效采集开关 =
    COALESCE(log_setting.collection_enabled, log_definition.collection_enabled)，
    即覆盖行为 NULL/"继承"时跟随模板值判断，而非要求覆盖行自身为 TRUE；
    且 log_definition.collection_enabled 且 deployment_template 匹配服务模板），
    带出 项目/环境/业务/服务 code、有效档位（log_setting 优先回落服务表）、
    有效处理规则（同上优先级）、服务级宏（macro_values）、部署模板宏定义（macro_definitions）
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

- **片段目录由 backend 全量托管**：渲染结果即该主机期望的完整 inputs 片段集合，agent 落盘后会
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

### 8.7 配置状态（期望 vs 已下发）与链路体检（2026-09-18）

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
- **"下发待变更"目前只作用于当前页**：前端「下发本页待变更（N）」用现有批量下发接口，N 为本页
  `drift`+`never` 的台数。跨全量的一键下发涉及 500–1000 台的批量执行，必须后端异步化，
  属计划 Phase 2（见 [LOG_COLLECTION_LIFECYCLE](../plans/LOG_COLLECTION_LIFECYCLE.md) §8 规模基线）。
- **界面位置**：采集目标的日常操作与配置状态都在「日志管理 → 日志采集」
  （`fronted/src/views/monitor/log-collectors/index.vue`，2026-09-18 从「智能监控 → 纳管目标」拆出，
  见 [MENU_STRUCTURE](MENU_STRUCTURE.md)）；主机表外壳与状态逻辑与 Exporter 目标页共用
  `components/HostTargetPanel.vue` + `util/hostTargetTable.js`。
- **一次性影响**：Phase 0 改了指纹语义（纳入输出段），因此存量主机的 `config_fingerprint`
  与期望值不再一致，升级后会全部显示 `drift`，重新下发一次即恢复稳定。

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
  失败不阻塞总览），并从 MySQL 带回项目/业务系统/环境/服务维度数据，前端组装树。
- 流名解析：后备索引名形如 `.ds-<流名>-<代数>`（或传统 `<流名>-<YYYY.MM.DD>`），先剥离
  `.ds-` 前缀与后缀还原流名，再用数据库维度码做前缀匹配（`streamNameMatcher`，编码可含
  连字符，禁止按 `-` 盲切）：新命名按 服务 维度命中，流即服务本身；旧命名（无服务段）
  要求剩余段恰好是已知档位。两种都命中不了才归"未识别"节点（手工建的、维度已删的），不丢数据。
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

入口挂在服务树的逻辑服务节点，进入即展示，无需用户输入：

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

- 入口：**日志查询界面**（服务树内嵌的 `LogQueryPanel`，选中逻辑服务/部署实例后）头部
  「清理日志数据」按钮；后端 `POST /monitor/log-datastreams/cleanup/`（`log_datastream_cleanup.go`，
  继承 `monitor:view`）。不放编辑弹窗（那里是编辑态，不适合破坏性操作）。
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
| 6 | 服务级开关、批量应用、清理和实例日志读取 | 已完成 | 经 dj-agent gRPC 执行；**批量应用仍是请求内串行、无并发上限**，1000 台规模下的异步化见计划 §8 |
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
