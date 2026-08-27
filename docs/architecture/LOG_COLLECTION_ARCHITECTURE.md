# djadmin 日志采集架构文档

OpenSearch + Fluent Bit 日志采集与分析能力的设计文档。

---

## 1. 目标

把各业务主机上的应用日志集中收集到 OpenSearch，并在 djadmin 内提供检索与错误分析，
不需要用户登录主机 `tail` 日志，也不需要跳转到 OpenSearch Dashboards 才能看到错误分布。

核心能力：

- 在逻辑服务上一键开启/关闭日志收集
- 一条日志处理规则统一维护 Fluent Bit 发送前处理与 OpenSearch Ingest 字段解析
- 日志定义只关联一条处理规则，规则可由同格式的应用日志复用
- 不输入关键词即可看到「出现了哪些错误、各多少次、集中在哪台机器」
- 自动识别新增错误与突增错误

---

## 2. 整体架构

```
业务主机（每台）                   日志服务器                    djadmin
┌──────────────────┐          ┌──────────────────┐        ┌──────────────┐
│ 应用日志文件      │          │ OpenSearch       │        │ 配置下发      │
│      ↓           │          │  ├ ingest pipeline│◀──────│ 处理规则管理   │
│ Fluent Bit       │─────────▶│  ├ index template │  REST  │ 聚合查询      │
│  ├ tail          │  HTTPS   │  └ ISM policy     │        │ 日志洞察页面   │
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
| Fluent Bit → OpenSearch | 主机 → 日志服务器 | 日志数据写入 |
| djadmin → OpenSearch | 控制端 → 日志服务器 | 管理 pipeline、执行聚合查询 |
| djadmin → dj-agent → 主机 | 控制端 → 主机 | 下发采集配置、触发热重载 |

---

## 3. 采集器选型

**采集器使用 Fluent Bit，不使用 Filebeat。**

Filebeat 从 7.14 起在 elasticsearch output 中加入服务端版本校验，检测到对端不是
Elasticsearch 会直接拒绝连接，返回 `400 Bad Request`。OpenSearch 由 ES 7.10 fork
而来，会被该校验拦截。可用的 Filebeat 只有 7.12.1 OSS，停留在 2021 年且不再有安全更新。

| 采集器 | OpenSearch 支持 | 单实例内存 | 结论 |
|---|---|---|---|
| Fluent Bit | 原生 `opensearch` output | 5-15 MB | 采用 |
| Vector | `elasticsearch` sink 兼容 | 30-60 MB | 备选 |
| Fluentd | 需插件 | 60-100 MB | 过重 |
| Logstash OSS | 官方插件 | 500 MB+ | 仅适合中转层 |
| Filebeat 7.12.1 | 版本校验前的最后一版 | 60-100 MB | 不采用 |

多行边界由日志处理规则中的首行正则、续行正则和合并超时定义。平台不提供 Java、Python、
Go 等语言专用分支；Java 堆栈、普通缩进续行和其他多行格式都使用同一套通用 regex multiline
机制。

---

## 4. 索引设计

### 4.1 索引按「环境 + 业务系统 + 保留档位」切分

```
logs-<environment.code>-<business_system.code>-<tier>

logs-prod-tib-hot
logs-prod-tib-std
logs-test-esb-std
```

服务、实例、主机、日志类型**不进索引名，作为字段存储**。

### 4.2 为什么需要保留档位

同一业务内部的日志量差异就很大：接入层服务每天几十 GB，定时任务每天几 MB。
ISM 是**索引级**的，同一索引内无法按 `service` 区分保留期，统一 30 天会撞爆磁盘，
统一 7 天又丢失了小服务的历史数据。

| 档位 | 保留 | 适用 |
|---|---|---|
| `hot` | 7 天 | 高频服务，如网关、接入层 |
| `std` | 30 天 | 默认 |
| `cold` | 90 天 | 量小但需长期保留，如审计、对账 |

档位配置在**逻辑服务**上（`ApplicationService.log_retention_tier`），不配则为 `std`。

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

按天切分对低流量业务是浪费。使用 data stream + ISM，按大小或时间自动滚动：

```json
PUT _index_template/logs-template
{
  "index_patterns": ["logs-*"],
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

### 4.5 ISM 按档位配置

三条 policy，通过 `ism_template` 的索引名后缀自动挂载，新建业务不需手工配置：

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
    "ism_template": [{ "index_patterns": ["logs-*-hot"], "priority": 100 }]
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
  business_system, environment
  service, instance, host_ip
  application, version, log_name

业务特有字段
  labels_<key>    由日志定义的 extra_fields 注入
```

业务附加字段统一增加 `labels_` 前缀，避免与平台固定字段冲突。需要聚合或告警的字段应提升
为固定字段，并在索引模板中预先定义 mapping，不能让任意业务字段无边界增长。

字段名统一使用下划线，不使用点号，避免 Fluent Bit `record_modifier` 注入时的歧义。

---

## 5. 解析规则

### 5.1 分层原则

| 处理类型 | 位置 | 原因 |
|---|---|---|
| multiline 多行合并 | Fluent Bit | 必须在采集时合并，堆栈跨行到后端已无法还原 |
| 字段提取 | OpenSearch ingest pipeline | 改规则不需要下发配置到主机，也不消耗主机 CPU |

两阶段在技术上分别执行，但在 djadmin 中只管理一条 `LogProcessingRule`：

- **发送前处理（Fluent Bit）**：`input_format`、`multiline_enabled`、`start_pattern`、
  `continuation_pattern`、`flush_timeout`
- **字段解析（OpenSearch Ingest）**：`pipeline_body`

修改 Pipeline JSON 后会立即发布同名 OpenSearch Pipeline，不需要下发主机配置。修改日志格式或
多行参数后，必须重新应用引用该规则的日志采集目标，使 Fluent Bit 片段更新。

### 5.2 统一规则与 Pipeline 生命周期

`LogProcessingRule.name` 同时是 djadmin 规则名称和 OpenSearch Pipeline 名称，仅允许小写字母、
数字、点、下划线和连字符，发布后不可改名。例如：

```
springboot-tomcat-exception
nginx-access
```

规则创建或更新时，djadmin 先调用 OpenSearch `PUT _ingest/pipeline/<rule.name>`，成功后保存
规则。删除规则时同步删除同名 Pipeline；仍被日志定义引用的规则禁止删除。平台不再提供独立的
Pipeline 写入、删除入口，避免数据库规则与 OpenSearch Pipeline 形成两个配置源。

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

页面通过 OpenSearch inline Pipeline `_simulate` 在保存前验证当前 JSON，不需要先创建临时
Pipeline。在线调试支持两种样例格式：

- **原始日志**：直接粘贴包含真实换行的完整日志，页面自动包装为 `{ "log": "..." }`，与
  Fluent Bit `tail` 插件实际产生的字段一致。
- **文档 JSON**：输入合法 JSON 对象，用于携带额外元数据；JSON 字符串内部的换行必须写为
  `\n`，不能直接回车。

Pipeline 若需要兼容页面调试中的 `message` 和真实采集的 `log`，应先使用 `rename` processor
把 `log` 统一为 `message`，并配置 `ignore_missing`。调试结果直接展示 OpenSearch 返回的完整
文档，包括解析字段和 `_ingest` 信息。

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
  + log_retention_tier        CharField(choices=[hot/std/cold], default='std')

ApplicationLogDefinition      已存在 path_pattern / encoding / collection_enabled
  + processing_rule           ForeignKey(LogProcessingRule, PROTECT, nullable)
  + extra_fields              JSONField    附加标签

LogProcessingRule
  cluster                     ForeignKey(OpenSearchCluster)
  name                        CharField(unique=True)；同时作为 Pipeline 名称
  description                 CharField
  input_format                CharField(text/json)
  multiline_enabled           BooleanField
  start_pattern               TextField
  continuation_pattern        TextField
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

OpenSearch 连接信息由 `OpenSearchCluster` 统一保存，不硬编码；日志处理规则明确关联目标集群。

---

## 8. 配置下发

### 8.1 主机侧目录结构

```
/etc/fluent-bit/fluent-bit.conf       主配置，安装时一次写入
/etc/fluent-bit/inputs.d/             djadmin 按实例的每条日志下发
/etc/fluent-bit/outputs.d/            djadmin 按应用、逻辑服务和日志名下发
/var/lib/fluent-bit/                  offset 数据库，必须持久化
```

主配置：

```ini
[SERVICE]
    Flush                  5
    Log_Level              info
    Hot_Reload             On
    HTTP_Server            On
    HTTP_Listen            127.0.0.1
    HTTP_Port              2020
    storage.path           /var/lib/fluent-bit/storage/

@INCLUDE inputs.d/*.conf
@INCLUDE outputs.d/*.conf
```

### 8.2 日志输入片段

每个实例的每条日志独立生成片段，文件名为
`<application>__<service>__<instance>__<log_name>.conf`。多行规则直接渲染为通用
`MULTILINE_PARSER`：

```ini
[MULTILINE_PARSER]
    Name          multiline_tomcat.tomcat-svc.kul-tib-tomcat1.catalina
    Type          regex
    Flush_Timeout 2000
    Rule          "start_state" "/^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3}\s+/" "continuation"
    Rule          "continuation" "/^(?!\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3}\s+)/" "continuation"

[INPUT]
    Name              tail
    Path              /home/esb/tomcat/apache-tomcat-9.0.35/logs/catalina.out
    Tag               tomcat.tomcat-svc.kul-tib-tomcat1.catalina
    DB                /var/lib/fluent-bit/tomcat__tomcat-svc__kul-tib-tomcat1__catalina.db
    Multiline.parser  multiline_tomcat.tomcat-svc.kul-tib-tomcat1.catalina
    Encoding          utf-8
    Refresh_Interval  10
    Skip_Long_Lines   On

[FILTER]
    Name    record_modifier
    Match   tomcat.tomcat-svc.kul-tib-tomcat1.catalina
    Record  business_system tib
    Record  environment     test
    Record  service         tomcat
    Record  instance        kul-tib-tomcat1
    Record  application     tomcat
    Record  version         9.0.35
    Record  host_ip         192.168.201.211
    Record  log_name        catalina
```

日志路径由 `${APP_HOME}` 等变量展开得到，与部署模板保持一致。

规则的 `input_format=json` 时额外生成 `Parser json`；文本格式不指定 Parser。未启用多行时不生成
`MULTILINE_PARSER` 和 `Multiline.parser`。

### 8.3 输出片段

Fluent Bit 的 `Pipeline` 参数不支持按记录动态取值，因此按**应用 + 逻辑服务 + 日志名**生成
OUTPUT，同一逻辑服务的同名日志多实例共用一个输出。Tag 固定为
`<application>.<service>.<instance>.<log_name>`，避免不同服务、实例或日志交叉路由。

```ini
[OUTPUT]
    Name                opensearch
    Match               tomcat.tomcat-svc.*.catalina
    Host                ${OS_HOST}
    Port                ${OS_PORT}
    HTTP_User           ${OS_USER}
    HTTP_Passwd         ${OS_PASSWORD}
    tls                 On
    Index               logs-test-tib
    Pipeline            springboot-tomcat-exception
    Suppress_Type_Name  On
    Retry_Limit         5
```

输出片段文件名为 `<application>__<service>__<log_name>.conf`。日志定义未关联处理规则时不生成
`Pipeline` 指令，日志仍可按原文写入 OpenSearch。

凭据通过 systemd `Environment=` 注入，不写入配置文件。

### 8.4 下发流程

```
开启逻辑服务的日志采集
  → 检查目标主机 Fluent Bit 安装状态，未安装则走软件包安装流程
  → 汇总该主机上所有启用的实例，生成各自的 inputs.d 片段
  → 从日志定义关联的 processing_rule 渲染格式、多行参数和 Pipeline 名称
  → 校验同主机内日志路径不重复
  → 计算配置指纹，与 LogCollectionTarget.config_fingerprint 比对，一致则跳过
  → 经 dj-agent 写入配置文件
  → 调用 POST http://127.0.0.1:2020/api/v2/reload
  → 更新配置指纹与下发时间
```

### 8.5 热重载

Fluent Bit 的热重载是**全局重新初始化所有 pipeline**，不是单 input 独立重载。
由于 `DB` 记录了 offset，重载后从断点继续，不会丢日志，仅有一到两秒采集间隙。

**不使用 `systemctl restart`**。

### 8.6 清理

服务关闭采集、实例移除、服务删除时，必须删除对应的 `inputs.d` 片段并触发重载。
否则 Fluent Bit 会持续尝试采集无人管理的文件，或因文件不存在反复报错。

---

## 9. 查询与洞察

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

**OpenSearch 能力边界**：Elasticsearch 的 `categorize_text` 聚合可做完全无监督的日志
聚类，OpenSearch 由 ES 7.10 fork，不包含该功能。因此聚类质量完全取决于 ingest 阶段的
fingerprint 归一化质量。

---

## 10. 复用现有能力

| 需求 | 复用 |
|---|---|
| Fluent Bit 安装、卸载、状态检查 | monitor 的 `SoftwarePackage` + `MonitorTarget` 纳管体系 |
| 配置文件下发 | dj-agent gRPC 文件写入 `write_open` / `write_chunk` / `write_close` |
| 服务状态检查 | dj-agent `check_exporter_status` 模式 |
| 路径变量展开 | `${APP_HOME}` / `${INSTANCE_NAME}` 现有机制 |
| 告警通知 | `AlertRoute` / `AlertMedia` |

不新建独立的安装纳管体系。

---

## 11. 实施状态

| 阶段 | 内容 | 状态 | 说明 |
|---|---|---|---|
| 1 | OpenSearch 连接、index template、ISM policy | 已完成 | 支持集群连接测试和幂等 bootstrap |
| 2 | 统一日志处理规则、Pipeline 发布、`_simulate` 调试 | 已完成 | 页面明确区分发送前处理与 Ingest，仍只保存一条规则 |
| 3 | 数据模型与迁移 | 已完成 | `LogProcessingRule` + 单一 `processing_rule` 外键 |
| 4 | Fluent Bit 软件包仓库、离线安装和状态检查 | 已完成 | 按平台、主版本和架构精确匹配，不依赖目标主机联网 |
| 5 | 配置生成、指纹比对、下发和热重载 | 已完成 | 输入、offset、输出按四段 Tag 隔离 |
| 6 | 服务级开关、批量应用、清理和实例日志读取 | 已完成 | 经 dj-agent gRPC 执行 |
| 7 | 日志洞察页面与告警接入 | 进行中 | 聚合查询接口已具备，页面和告警闭环继续完善 |

解析规则调试仍是后续扩展的回归基线：新增日志格式必须先用真实样例通过 `_simulate`，再关联
日志定义并应用 Fluent Bit 配置。

---

## 12. 风险与注意事项

| 项 | 说明 |
|---|---|
| 日志读取权限 | 应用日志属于 `esb` 等业务用户，Fluent Bit 需以 root 运行或配置 ACL。安装检查时应一并验证可读性，避免配置下发成功但无数据 |
| 同主机路径冲突 | 同一主机上多个实例日志文件名可能相同，下发前必须校验展开后的绝对路径唯一，否则 Fluent Bit 会产生 harvester 冲突 |
| mapping 字段膨胀 | 业务附加字段统一使用 `labels_` 前缀；需聚合的字段提升为固定字段，并设置 `total_fields.limit` |
| 时间戳 | 必须在 pipeline 中用 `date` processor 覆盖 `@timestamp`，否则记录的是采集时间而非日志产生时间 |
| 容器化采集器 | 若 Fluent Bit 以容器运行，仅能看到挂载路径。下发前需校验目标路径落在已挂载前缀内 |
| 磁盘水位 | OpenSearch 磁盘超过水位会将索引置为只读，生产环境需保留水位检查并配置 ISM 自动清理 |
| TLS 证书 | 自签证书阶段使用 `tls.verify Off`，生产需分发 CA 证书并开启校验 |
