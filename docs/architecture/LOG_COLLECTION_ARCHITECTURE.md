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

**正则方言是 Go RE2，不是浏览器/Java 方言**（2026-09-19 定案）。首行正则最终由**主机上的
Filebeat** 编译执行（`parsers.multiline.pattern`，见 §8），而 Filebeat 与服务端用的是同一个引擎
——Go 的 `regexp`（RE2）。所以"规则合不合法"只能按 RE2 判，且**必须在保存时就判**：

| 环节 | 行为 |
|---|---|
| 保存/发布规则 | 按 RE2 编译 `start_pattern` 与 `continuation_pattern`，编译不过直接拒绝，错误文案给出等价写法（`\u4e00` → `\x{4e00}` 等），见 `internal/logcollect/regex_pattern.go` |
| 规则编辑页 | 同一套预检（`util/re2Pattern.js`），并且把 RE2 语法翻译成浏览器能编译的形式，让「在线调试」能用 RE2 写法试跑（`\x{4e00}` → `\u4e00`）。预览只是预览，权威仍是服务端那次编译 |
| 格式认证 | 用 RE2 编译首行正则还原样例（`log_format_verify.go`，见 §4.8） |
| 配置下发 | 渲染前再按 RE2 编译一次；编译不过的日志定义**跳过并告警**（与"未关联处理规则""路径含未定义宏"同样处理）。不跳的话坏片段会让 Filebeat 起不来，**整台主机的日志一起停** |

RE2 与浏览器/Java 方言的主要差异（现场踩过前两条）：不支持 `\uXXXX`（用 `\x{XXXX}`）、
**不支持任何断言**（`(?= / (?! / (?<= / (?<!`）、不支持反向引用与八进制转义（`\1`）、
不支持原子组 `(?>` 与占有量词（`a*+`）、`\A`/`\z` 有而 `\Z` 没有。平台不做"方言自动改写"：
改写只能覆盖其中一部分（断言、反向引用在 RE2 里根本没有对应写法），会让用户以为平台什么
Java 正则都吃。续行正则尤其要注意：`^(?!首行正则)` 是最自然的写法，而这里**不需要**它——
Filebeat 用 `negate: true, match: after` 表达"不以首行正则开头的行并入上一行"，续行正则留空即可
（该字段平台不消费，保留列只为兼容历史数据）。

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
  过渡期遗留流，ILM 按保留期自然删除，不做 reindex；水位视图对两种命名都可见，
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
服务级默认档位还可在日志中心「日志配置」的**日志表上方**"默认保留档位"下拉单独覆盖到某条日志
（`assets_application_service_log_setting.retention_tier_id`，null 表示继承服务默认）。

**保存链路（Go 版）**：前端逻辑服务表单提交 `log_retention_tier`（档位 ID，可为 null）
→ `assets.SaveApplicationService` 在 INSERT/UPDATE 中写入
`assets_application_service.log_retention_tier_id`；GET 返回同名字段回显。档位 ID
必须指向 `monitor_log_retention_tier` 中存在的记录，前端下拉只列 enabled 档位。

**模板日志回显（日志中心「日志配置」）**：`GET /assets/application-services/:id/log-config/`
（Go `assets.ListServiceTemplateLogs`）响应顶层带 **`log_collection_enabled`**（采集总开关）与
**`log_retention_tier`**（服务级默认保留档位，`null` = 由平台默认档 `is_default` 决定；
界面靠它把"继承服务默认"写成"继承服务默认（标准 30 天）"，见 §9.5.0），
每行除覆盖值外还返回 `resolved_path` 与 `data_stream`——`resolved_path` 按**与渲染同一套顺序**展开（`shared/logmacro`：模板
`macro_definitions` 的 value → 服务 `macro_values`，模板 `app_home` 作 `APP_HOME` 默认值），
仍是"服务这一层能解析的部分"：实例级 `runtime_variables` 因逐实例而异不展开，
剩下的宏由 `pending_macros` 报出来（界面标"实例上展开"，见 §9.5 的原始/解析后开关）；`data_stream` 用 `shared/logstream.Name`
生成（`autoadmin-<项目>-<业务系统>-<环境>-<服务>-<有效档位>`），有效档位取值顺序为
日志覆盖档位 → 服务默认档位 → `is_default` 档位 → `std`，与 Filebeat 下发的
Index 命名（`monitor.LogDataStreamName`，内部委托同一 shared 实现）完全一致。
同一次响应里还带出格式认证状态（`format_state` / `format_fingerprint` /
`format_verified_*`，见 §4.8）。每行还带 **`tier_code`**（这条日志**当前生效**的档位编码，与 `data_stream` 尾段一致；日志中心页
用它区分当前档位与历史档位流）与 **`service_code`**（本服务的编码，不是这条日志的，仅展示用）：
日志中心页按服务取水位数据改用**服务 id**（`application_service_id`，见 §9.5），因为编码的唯一域是
`(业务系统, 环境)`、可跨业务重复，不再能当全局连接键。

**「模板日志」表已从逻辑服务编辑弹窗移除**（2026-09-20）：逐条日志配置（采集开关、保留档位、
采集过滤、格式认证）**统一走日志中心「日志配置」**——那里有勾选批量、配置差异、下发入口与链路诊断，
两处并存只会让人不知道该信哪一处。弹窗只保留服务级的两个日志默认值（开启采集、默认保留档位）
和一句指向日志中心的说明；保存时**不提交 `log_settings`**（整表替换语义，提交空集合会删掉已有覆盖值）。

历史（供排查旧问题参考）：那张表的行来自模板**详情**——模板**列表**接口不带 `logs`
（嵌套结构只在 `GET /assets/application-deployment-templates/:id/` 里，列表只给 `log_count`），
直接从列表记录读 `logs` 永远读到空数组，表现为"新建服务时那张表一直空着、保存后重新编辑才出现"。
日志中心那张表不需要这套推导：它读的是服务自己的 `log-config`（服务必然已存在）。

**格式认证**：`POST /assets/application-services/:id/log-config/verify/`，
body `{"log_definition_id":<必填>,"source":"instance|sample_log|waiver","deployment_id":<实例 id>}`
（`source` 缺省为 `instance` 时必填 `deployment_id`）。通过才写库，不通过返回 **200 + `passed=false` +
`missing_fields`**（"没通过"是业务结果，不是接口错误）；取不到样例一律报错，不返回空缺失。

**路径通配按需展开**：`GET /assets/application-services/:id/log-config/glob/?log_definition_id=<必填>`
（Go `assets.PreviewServiceLogGlob`，实现注入自 logcollect）。只读展示：逐台承载实例展开宏后
由 backend 用 agent `ListFiles` 逐层展开通配（`logcollect/log_glob_remote.go`），按实例分组
回给界面「解析后」列（见 §4.8）。未接线、主机离线、路径匹配不到都以可读信息呈现，不静默返回空。
**展开不依赖 agent 版本**（只用早已存在的 `ListFiles`），已部署的 agent 无需升级。

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

`index.refresh_interval` 有意设为 **10s**（ES 默认 1s）：用搜索实时性换写入吞吐。代价是文档
**已写入 ES、但最多 10s 后才可被检索**——现场容易误判成"没采到"。需要立即查看时走按需刷新
（日志查询面板的「刷新索引并查询」，见 §9.8），不改变写入侧行为。

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

**保留期 = 值 + 单位，单位支持「天」与「小时」**（迁移 000045，2026-09-20）：

- 库里是两列 `retention_value` + `retention_unit`（`d` / `h`，默认 `d`；迁移把原来的
  `retention_days` 改名成 `retention_value` —— 值 + 单位才是完整语义，留着 "days" 这个名字
  在两列并存时会误导）。存量档位全部按天，**生成的 min_age 与改动前逐字节相同**（`30d`）。
- 改名这一步在真库上要先摘掉 Django 生成的 CHECK 约束：真库的 `monitor_log_retention_tier`
  带着 Django 4.1 为 `PositiveIntegerField` 自动加的
  `CHECK (retention_days >= 0)`，而 **MySQL 不允许重命名被 CHECK 引用的列**（Error 3959），
  不摘掉迁移必挂（2026-09-20 现场）。该约束在 `int unsigned` 列上恒真、折叠快照里也没有，
  所以按 information_schema 现查现删、不重建（做法与教训见 SQL_DESIGN §4.6.2 ①）。
- 生成 ILM 时拼成 `<值><单位>`：`min_age: "12h"`。ES 的 `min_age` 本来就接受 `s/m/h/d`，
  所以这是"把单位带进去"，不是新机制；`rollover_min_index_age` 的校验 `^\d+[mhd]$`
  也一直允许小时。
- **不影响已有数据流**：流名与 ILM 策略名用的都是档位 **code**，不是天数，所以换单位/改保留期
  只影响该档位**新数据**的到期时间，不重建任何流。
- 只在写入路径接受 `d`/`h`（其它值 400），读取/生成时异常值一律按天兜底——**宁可沿用旧行为，
  也不要生成非法或意外的保留期**。
- 范围按**折算成小时**判：`1 小时 ≤ 保留期 ≤ 3650 天`。
- 精度提醒：ES 的 ILM 默认每 10 分钟轮询一次（`indices.lifecycle.poll_interval`），
  所以小时级保留的实际到期时间有 ~10 分钟粒度；**分钟级单位不提供**（轮询粒度决定了它没有意义）。
- 档位保存/删除后自动异步重推所有启用集群的 ILM 策略与索引模板（`syncAllClusterLogStorage`），
  不需要手工发布。

**档位模板的 priority 约定与遗留修复（2026-09-21）**：ES 的可组合索引模板在多个模板的
`index_patterns` 匹配同一索引时**只取 priority 最高者**（不合并，所以档位模板必须自包含，
见 §4.4），且"同 priority + patterns 重叠"在 PUT 时直接 400。平台约定：基础模板
`<prefix>-template`（patterns `<prefix>-*`）priority=0 兜底，档位模板
`<prefix>-<档位>-template`（patterns `<prefix>-*-<档位>`）priority=200 压过基础模板。
旧版建档位模板时未写 priority（ES 默认 0），这些遗留模板与基础模板同层重叠，导致
bootstrap 重写基础模板时报"存在冲突的模板"（现场 hot/std/cold/wuhan-test 四个全停在 0）。
修复：`bootstrapElasticsearchStorage` 在基础模板冲突检查**之前**执行
`repairLegacyTierTemplatePriorities` 自愈——识别口径是**命名 + pattern 形状双吻合**
（`legacyTierTemplateName` 解析 `<prefix>-<code>-template`，code 可含连字符如
`wuhan-test`；`isTierPatternShape` 要求全部 patterns 严格等于 `<prefix>-*-<code>`，
恰好同名的第三方模板不命中），对命中且 priority≠200 的模板 GET 原定义 → 只改
priority=200（mappings/settings/ILM 绑定原样保留，采集与保留行为不变）→ PUT 回写。
自愈是尽力而为：单条失败不中止，流程继续落到冲突检查的可读报错。
**两个 ES 版本差异都要防**：`_cat/templates` 的 `priority` 列在部分版本恒为空（2026-09-21
现场），所以自愈判"要不要修"读的是 GET 模板定义里的 priority，冲突检查
（`conflictingIndexTemplates`）同理——cat 只做候选筛选（名字 + patterns 重叠），
真实 priority 逐个 GET 定义确认（缺省 = 0；读不到定义时按冲突处理，宁可误报不放行真冲突）。

### 4.6 容量可按档位反推

```
hot   30GB/天 ×  7 天 = 210 GB
std    5GB/天 × 30 天 = 150 GB
cold 0.1GB/天 × 90 天 =   9 GB
短期   5GB/天 × 12 小时 =  2.5 GB
```

哪个档超出预算就调哪个档的保留期，不影响其他服务。**小时档位必须折算成天**（12h = 0.5 天）：
不折算会把预估占用虚高 24 倍。服务端 `estimatedTierTotalGB` 与前端
`util/logRetention.js` 的 `estimatedTotalGB` 是同一口径（两处都要改，别只改一边）。

### 4.7 字段设计

同一索引内混合多种应用的日志，若每种应用解析出的字段都独立建 mapping，字段数会持续膨胀。

**索引模板里实际声明的字段（= 唯一权威清单，与 `log_management.go` 的 `standardLogFields` 一致）**
共 16 个，分三类：

| 字段 | 类型 | 谁保证它存在 |
|---|---|---|
| `@timestamp` | date | Filebeat filestream（平台；值是否被 pipeline 的 `date` processor 覆盖成日志时间另说） |
| `message` | text | Filebeat filestream（原始行） |
| `project`、`business_system`、`environment`、`service`、`application`、`instance`、`host_ip`、`log_name` | keyword | 下发片段里的 `fields_under_root` 注入（`log_config_render.go`；`host_ip` 在主机没采到 IP 时写空串，保证键存在） |
| `log_path` | keyword | **不是静态注入**：input 级 `copy_fields` 处理器把 Filebeat 的 `log.file.path`（**该事件实际来自哪个文件**）拷到顶层 `log_path`。路径含通配时静态值只会是带 `*` 的模式，日志详情必须显示具体文件（2026-09-20） |
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
| `instance` | 部署实例所在主机上的真实日志文件 | `GetLogVerifyInstanceContext` 取 host/宏 → 与采集下发同一套 `resolveMacros` 展开路径（含未展开宏即报错）→ `StatFile` + `ReadFileChunk` 反向读尾部 1MiB 窗口（`ReadFileChunk` 一次只回一个 chunk，不能读到 EOF）→ 丢掉窗口起点的残行。**路径允许写通配**（`/var/log/*.log`、`/var/log/*/*/*.log`）：backend 用 agent 的 `ListFiles` 逐层展开（`logcollect/log_glob_remote.go`，不依赖 agent 版本；只支持单层通配，不支持 `**`）。**认证范围可选单个实例或全部实例**（`all_deployments`）；且**无论哪种范围，该实例上所有匹配到的日志文件都逐一认证**（不是只抽最新那个）——Filebeat 会 tail 全部匹配文件，任一文件解析不出必备字段即不通过 |
| `sample_log` | 解析规则里保存的样例日志 | 规则未配样例即报错，不做静默回退 |
| `waiver` | 无（人工确认豁免） | 不碰 agent/ES，仍记当前指纹 + 操作人（取自登录态，不接受前端自报） |

认证返回**报告**（`assets.LogFormatVerifyReport`）：全部目标的缺失字段并集 + 逐实例明细
（`targets[]`，含实例名、参与认证的 `log_files`、该实例的 `missing_fields`、取不到样例时的 `error`）。
某台主机离线只写它自己那一项的 `error`，不阻断其他实例；**任一目标缺字段或出错即整体不通过、不写库**。

样例文本还原成 pipeline 输入的口径与规则调试页 `buildRawDocs` **逐字一致**（默认逐行成记录；
开启多行后 `negate/match=after` 合并，不需要续行正则）。**截断按"记录"而不是按"行"**：
单行模式取尾部 50 行；多行模式**先把整段 1MiB 窗口按首行正则还原成记录、再只保留尾部 50 条记录**
（2026-09-20 修）——若先按行截断，Tomcat/Nacos 的 `log_error.log` 尾部一条超长堆栈（几十上百行
`\tat …`）会把尾 50 行占满、全是续行，于是误报"样例日志未命中首行正则，还原不出任何记录"，
与规则本身无关。**多行模式下，第一条命中
首行正则的记录之前的行两边都丢掉**（2026-09-19 修）——反向读取的 1MiB 窗口几乎必然切在某条多行记录的中间，窗口起点
之后的堆栈续行（`\tat …`）不是一条记录，真实采集时 Filebeat 会把它们并进上一条；一旦当成独立记录
送进判定，就会让"每一条记录都要齐必备字段"的判定报 **"规则解析不出这些必备字段 —— log_level"**，
与规则本身无关（现场：tomcat 的 catalina.out 认证一直被它挡着）。正则在服务端用 RE2 编译
（Filebeat 也是 RE2），而不是浏览器的 JS 正则——认证要测的是主机上真正会跑的那套规则。
**首行正则编译不过就直接报错并给出等价写法**（`\u4e00 → \x{4e00}` 这类提示由
`regex_pattern.go` 统一产出，与保存校验同一份，见 §2）。
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

**"调试页通过、认证却不过"这个坑（2026-09-19 又踩一次）**：两处的**判定**本来就是同一个函数
（`simulatePipeline` + `missingRequiredDocumentFields`），差别只在**输入**——调试页是你粘的样例、
认证是主机文件尾部。会分叉的只有两件事，都已经对齐：
① **采样语义**（多行模式下前导残尾行两边都丢，见上）；
② **呈现**：调试页缺必备字段时是红色错误 + "判定不通过"（此前 toast 写的是"运行成功"，
会被读成"规则没问题、认证在挑刺"）；
③ **输入形状**（2026-09-19 补）：两处的样例文档都按 Filebeat 的真实事件补齐
（`sample_event.go`；此前只有 `{"message": ...}`，于是"处理器把 message 覆盖成对象"这类错误
在两边都看不见——认证同样过、主机上全丢）。所以**把文件尾部几十行原样粘进调试页跑一次，结论就等于认证的结论**。

**前端入口**：日志中心「日志配置」的日志表有「格式认证」列（**唯一入口**，2026-09-20 起逻辑服务
编辑弹窗不再编辑逐条日志配置）——未认证/需重新认证是"发起认证"，已验证是"重新认证"
（改了实例级 `runtime_variables` 这类不进指纹的变化只能人工重跑）；
弹窗里选依据，按实例抽样时再选**认证范围：单个实例（默认，下拉选一个已绑定实例）/ 全部实例**
（候选取**库里已绑定**的实例，不取表单里未保存的勾选）。无论哪种范围，每个实例上匹配到的**每个
日志文件**都会被认证。不通过时弹窗留在原地：先给缺失字段并集，再按实例列出 `targets`（实例名、
参与认证的文件、该实例的缺失字段或取不到样例的原因），允许换依据重试。同一处顺带修掉了
`loadLogConfig` 的静默 `catch`——加载失败与"确实没有日志定义"在界面上都是空表格，必须能区分。

**批量认证（2026-09-20）**：日志中心「日志配置」勾选多条日志后，批量栏里的「格式认证」把选中的
日志一起交给**同一个** `LogFormatVerifyDialog`（它接受 `targets`；单条就是"只有一条的批量"，
所以依据/范围/后果文案只有一份）。行为：逐条串行调同一个认证接口（每条都要去主机取样例，
没有批量接口也没有必要），弹窗里显示"正在认证第 x/y 条"；**单条失败不中断整批**（主机离线这类
硬失败也记下来继续跑），结束后把没通过的那几条连同原因列出，全部通过才关闭并回写状态列。
**没挂解析规则的日志会被跳过并明确告知跳过了几条**——静默少认证几条比拒绝更糟（用户以为都验过了）。

> **编辑弹窗里认证通过后保存服务，认证结果必须还在（2026-09-20 修）**：`SaveApplicationService`
> 的整表替换若 delete-all 会抹掉 `format_verified_*`，表现为"认证通过 → 保存 → 又变未认证，必须去
> 日志中心再认证一次"。现在整表替换已改为逐行 upsert（不碰认证列）+ 只删未提交行，前端也不再过滤
> 全 null 覆盖行（见 §9.5.1）。

**路径通配的按需展开（「解析后」列）**：路径含 `*` / `?` / `[` 时，服务这一层只能解析出
模式，具体采哪些文件要到主机上才知道。日志中心「日志配置」的「解析后」列给出「展开文件」，点击后调
`GET /assets/application-services/:id/log-config/glob/?log_definition_id=`，backend 逐台承载
实例展开宏（与采集下发同一套 `resolveMacros`）→ 用 agent `ListFiles` 逐层展开通配 →
**按实例分组**列出真实文件（`fronted/src/components/LogGlobPreview.vue`，两页共用）。
**为什么不进首屏**：展开要逐台主机调 agent，慢且依赖主机在线；`log-config` 是纯读库的表格
接口，塞进去会让整张表等主机。某台主机离线/路径匹配不到只在它那一项写 `error`，其余实例
照常返回（只读展示，不能因一台失败就整条日志不给看）。弹窗里只在**编辑态**（服务已有 id）
出现——新建服务还没有绑定实例，无从展开。

**状态标签的颜色就是"要不要人去处理"**（`fronted/src/util/logFormatState.js`，日志中心的日志配置
tab（以及格式认证弹窗的按钮文案与批量认证失败列表）共用同一份，避免多处说法不一致）：`已验证`=绿（不用管）、
`需重新验证`=橙（配置指纹变了，抽样重跑一次）、**`未验证`=红**（2026-09-19 由灰改红）——
没验证就采集是**静默坏数据**：日志查得到，但级别/消息列为空、关键词搜不到、错误清单失效；
按约定"开启采集前必须验证一次"，所以它是一个待办事项，灰色标签混在表格里只会被忽略。
tooltip 里写清后果与下一步（点这一行的「发起认证」；没挂解析规则的先到部署模板挂规则）。

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

#### 5.2.1 处理器参数的引擎方言（Elasticsearch ⇄ OpenSearch）

Pipeline 是**原样发给集群**的，所以处理器参数名必须与目标集群的引擎一致。平台的目标集群换过引擎：
早期是 ES 7.10 的 fork（OpenSearch 系，水位用 `_plugins/_ism/explain`），后来迁到真正的
Elasticsearch（改 `_ilm/explain`，见 LOG_COLLECTOR_MIGRATION.md）。当年在旧集群上写的规则搬到新
集群会直接 400，而报错只说"不支持这个参数"，看不出该改成什么（现场：三条规则的
`rename.override_target`）。

| 参数（OpenSearch） | 参数（Elasticsearch） | 语义 | 核实方式 |
|---|---|---|---|
| `rename.override_target` | `rename.override` | 目标字段已存在时覆盖；**源字段不存在时两个都不动目标字段** | 在 ES 8.13.0 上用 `_simulate` 验证过：`override` 被接受，`override_target` 400；样例证明只在 `log` 存在时才覆盖 `message` |

- 别名表与提示在 `pipeline_compat.go`，**报错处补一句"该改成什么"**，不做自动改写：存储文本与
  实际发出的内容不一致会让"期望配置指纹"体系（§8.6）失去意义；加新别名必须先在本仓库支持的两个
  引擎上各验证一次。
- 接线点是 ES 边界的两处收口：`simulatePipeline`（调试页与格式认证共用）与
  `publishProcessingPipeline`（规则保存），所以调试、认证、发布都会带提示。
- **不要用"先 remove 目标字段再 rename"来替代覆盖**：源字段不存在时那句 `remove` 会把目标字段
  一起删掉（已用 `_simulate` 复现：文档变成 `{}`），等于静默丢日志内容。
- 换集群引擎后，"规则还在但集群上没有对应 pipeline"是可见的：链路体检的 `pipelines` 层会报
  「集群上不存在该 pipeline，日志不会被解析」或「与页面配置不一致，需重新发布」（`log_health.go`），
  修完规则要重新发布一次才会重新 PUT 到集群。

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
  Filebeat filestream 实际产生的字段一致；多行按首行正则聚合（用 `util/re2Pattern.js` 把 RE2
  写法翻译成浏览器可编译的形式，翻译不了的写法明确报"预览失真"而不是给一个错的预览，见 §2）。
- **文档 JSON**：输入合法 JSON 对象，用于携带额外元数据；JSON 字符串内部的换行必须写为
  `\n`，不能直接回车。

**校验以集群真实 mapping 为准**：调试时后端 `GET _index_template/<prefix>-template` 取
`mapping.properties` 的顶层字段集，输出文档里不在其中的字段会作为 `schema_violations` 报出
（`dynamic:false` 下会被静默丢弃，需改写到 `app_fields.<字段名>`）；模板取不到时回退内置标准字段。

**必备字段校验（四层，共用同一份定义 `requiredProcessingRuleOutputs`）**：产物必须包含
`log_level`、`log_message`、`error_fingerprint`——缺了都不报错，只是静默失效：

| 层 | 位置 | 行为 |
|---|---|---|
| 保存时静态检查 | `validateConfigInput` → `missingPipelineOutputs` | 缺任一必备字段直接 400 拦截发布（认 `fingerprint`/`set`/`copy`/`rename` 的 `target_field`/`field`、`dissect`/`grok` 的 pattern 命名捕获，以及 `script` 源码里对 `ctx.<字段>` 的**赋值**；**是启发式**，判不了条件分支）。粗筛宁可漏判（漏了还有 guard 兜），不能误判——2026-09-19 就因为不认 script 把一个能跑的规则拦在发布门外（ES 服务端 JSON 日志：键里带点、ES 8.13 的 `json` 处理器没有 `expand_dots`，只能用 painless 取字段） |
| 调试页 | `_simulate` 的 `missing_fields` | 用真实样例给出"缺哪个字段"，与上面的集合自动同步 |
| **运行期试跑** | 采集链路的「解析规则」层 → `rulePipelineRunVerdict` | 用**规则自带的样例日志 + Filebeat 真实事件载荷**在**已发布的那份 pipeline** 上跑一次；缺必备字段即判 `drift`，detail 写明"会被 ES 拒收"并给出最短原因（如 `message` 被搬成了对象）。**没配样例日志就不判**（detail 里标"未试跑"），不猜 |
| 写入时兜底 | 索引模板 `index.final_pipeline` = `<prefix>-mapping-guard` | 在规则自己的 pipeline **之后**执行，按同一组字段判定：`tag`（默认）只打 `mapping_violation` 标记，不丢数据、不做检索排除（定位是"格式悄悄变坏"的发现器，配合巡检/告警）；`drop` 直接丢弃，非默认。规则作者改不到它 |

**试跑这一层为什么必须存在（2026-09-19 现场，kul 的 tomcat）**：上面三层都判不出"处理器把
`message` 覆盖成对象"这种**运行时**错误——静态检查是启发式，调试页与认证喂的样例文档当时只有
`{"message": <原始行>}`，而 `ignore_missing: true` 的 rename 在"字段不存在"时是静默跳过的。
于是平台上处处绿灯，主机上每一条事件都被 ES 以 `document_parsing_exception`（400）拒收、Filebeat
只能丢弃（`Cannot index event (status=400): dropping event!`），data stream 永远是 0 条。
**教训：校验的输入不真实，绿就是噪声。** 因此：

- 样例文档必须带 **Filebeat 在每个事件上都会写的字段**（`internal/logcollect/sample_event.go`
  的 `filebeatEventFields`：`log.file.path`/`log.offset`/`host`/`agent`/`ecs`/`event`/`input`，
  外加下发片段 `fields_under_root` 注入的维度字段）。调试页、格式认证、链路试跑三处**共用同一份**，
  调用方自己给的字段优先（"样例文档 JSON"模式可以自己造）。
- 判定侧要把这些字段当"本来就在"：`schema_violations` 豁免 Filebeat 自有字段（`dynamic:false`
  丢掉它们是设计如此），否则每条规则都会平白多出 6 条噪音。
- **遗留规则的 `rename log → message` 一律要清掉**：那是 Fluent Bit 时代的写法（当年原始行在
  `log` 里），Filebeat 下 `log` 已是对象、原始行是 `message`——Fluent Bit → Filebeat 迁移时
  这一条"字段形状变了"没被任何校验覆盖，是这次现场的直接原因。

另有一条**巡检清单**：规则列表的 `missing_required_fields`（前端「日志解析规则」页的「必备字段」
列）把"静态看着缺字段"的规则直接标红——它是切换 `drop` 模式前必须先看的清单，因为丢弃后
ES 里不留任何痕迹，违规无法事后核查。

规则页**左侧应用列表带筛选框**（2026-09-19 加）：按应用名称或编码匹配（不区分大小写），
**「全部规则」始终保留**——它是回到全量的入口，筛没了会让人无路可退；一条都没命中时列表下方
给出提示。匹配逻辑在 `fronted/src/util/applicationFilter.js`（纯函数，带单测），页面只负责接线。
筛选只影响左侧列表，右侧表格仍由选中项决定（点哪个应用看哪个应用的规则）。

dj-agent 的文件通道（`internal/agent/file.go` 的 `StatFile` + `ReadFileChunk`）提供
「读取该实例最近 N 行日志」的能力，它正是 §4.8 认证里 `instance` 依据的样例来源——
认证组件把抽样还原成 pipeline 输入后走同一条 `_simulate`，所以"调试页手动贴样例"与
"认证自动取样例"的判定口径完全一致，闭环已经接上。

> **通配路径**：`path_pattern` 允许写 `*`（如 `/var/log/*.log`、`/var/log/*/*/*.log`），
> Filebeat 会展开；backend 侧展开**放在 `logcollect/log_glob_remote.go`**：用 agent 早已存在的
> `ListFiles` 逐层列目录、按路径段 `filepath.Match`，得到全部匹配的普通文件（按路径排序）。
> 认证抽样取其中 mtime 最新的一个读尾部；界面「解析后」列的按需展开列出全部（见 §4.8）。
> **不依赖 agent 版本**（无需升级 agent）；只支持单层通配，不支持 `**`。

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

### 6.1 采集过滤（include / exclude，2026-09-19 接入）

**历史**：这套功能在 Fluent Bit 时代是真生效的（`backend/djadmin/monitor/fluent_bit.py` 渲染
`[FILTER] grep`：`Regex log <pattern>`，在 tail 读取、多行合并**之后**过滤）。换 Filebeat 时只搬了
配置、没搬渲染，于是长期"能建规则、能选，却不生效"（服务弹窗那列因此被临时隐藏）。迁移 000039
把结构补齐、渲染接上 `include_lines` / `exclude_lines`，语义与当年一致。

**两层 + 三态**（`internal/logcollect/log_collection_filter.go`）：

| 层 | 字段 | 语义 |
|---|---|---|
| 模板默认（日志定义） | `filter_include_rule_id` / `filter_exclude_rule_id` | NULL = 该方向不过滤 |
| 服务覆盖（服务 × 日志定义） | `collection_filter_rule_id` / `collection_exclude_filter_rule_id` | **NULL 继承模板 / 0 显式关闭该方向 / >0 指定规则** |

- **0 是有效取值，所以这两列不带外键**（2026-09-19 修复）。列上若挂着应用层外键，数据库会把 `0` 当成
  "指向 id=0 的规则"，保存直接外键失败、被 `translate()` 翻成**「关联资产不存在」**——一句与过滤毫无关系、
  也指不出该改哪里的报错。现场触发路径：日志中心「日志配置」把某行的「采集过滤（保留）」从
  "继承模板（common-error）"改成"不过滤"（正是迁移 000039 升级说明推荐的做法）。真库上 include 列挂着
  Django 时代的外键，迁移 **000040** 摘掉（exclude 列自 000039 起就没有）；折叠快照
  `db/schema/*/002_assets.sql` 的列注释与守卫 `internal/assets/log_filter_rule_fk_guard_test.go`
  一起防止"以后再把外键补回来"。引用完整性改由渲染侧降级承担（见下条"宁可多采不可不采"）：
  规则被删/停用 → 该方向不过滤 + 告警。

- **方向由规则自己声明**（`monitor_log_collection_filter_rule.rule_type` = include/exclude），落槽时校验一致；
  白名单落进 exclude 槽会**反转语义**（只采噪声、丢掉正常日志），所以类型不符时忽略该方向并告警。
- 两个方向各自独立：Filebeat 上本来就是同一 input 的两个参数，include 先跑、exclude 后跑，同时命中 → 丢弃，
  所以"先框白名单、再排噪声"是常规用法。
- **过滤是采集侧的、丢弃不可逆**：被滤掉的记录不进 ES，平台里查不到也补不回来。因此：
  - 规则 CRUD 页带**试算**（粘样例看保留/丢弃哪些行，用 RE2 预览编译，与主机同一方言）；
  - 保存时按 RE2 校验（与首行正则共用 `regex_pattern.go`），并拒绝**空正则**——空模式在 Filebeat 里
    匹配全部：include 会放行一切、exclude 会丢弃一切；
  - `enabled=false` 的规则 = 不生效（不是"过滤成空"）。
- **正则按"合并后的整条记录"匹配**（Filebeat 文档原话：multiline message is combined into a single line
  before the lines are filtered），所以 `^` 只匹配记录开头，要匹配记录中间的行得用 `(?m)`。
- **宁可多采不可不采**：规则被删、被停用、方向不符、正则编译不过（历史数据/直接改库）时一律降级成
  "该方向不过滤 + 告警"，绝不让 Filebeat 起不来、也不因此停采这条日志——这是与首行正则**有意不同**的
  处理（首行正则编译不过是跳过该日志定义，因为多行合并没有它就整个错）。告警走渲染 `warnings`
  （`config_warnings`，日志采集页与链路体检可见）。
- **随时可改，但要重新下发**：过滤是期望配置的一部分，改完必须重新下发才到主机上；改规则会让承载
  主机全部变成"待下发"（指纹变了）。三个入口共用同一份规则与同一套三态，**口径不许分叉**：
  部署模板「路径与文件 → 日志」两个下拉（模板级默认值，选模板时的初始态）、
  日志中心「日志配置」的「采集过滤（保留/排除）」两列（服务级覆盖，按行即时保存）、
  日志中心「日志配置」同名两列（服务级覆盖，按行即时保存）。
  后两处显示的是同一条 (服务 × 日志定义) 的配置：弹窗是"这个服务要用的配置"，日志中心是
  "这条日志现在到底怎么采"。这列在弹窗里曾**长期隐藏**（当时过滤还没接入渲染，藏着以免误导成
  "过滤在起作用"），接入渲染后必须放出来——同一个表里能看到采集开关与档位却看不到过滤，
  只会让人以为功能没做。

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
  + filter_include_rule_id / filter_exclude_rule_id   采集过滤的**模板级默认值**（迁移 000039）
                              各自独立、NULL = 该方向不过滤；服务级可覆盖或显式关闭，见 §6.1
  + extra_fields              JSONField    附加标签（⚠️ 尚未参与渲染，见 §4.7）
  （原 collection_enabled 已于迁移 000034 删除：采集开关下沉到逻辑服务，见 §6）

ApplicationServiceLogSetting  服务级覆盖，一行 = (服务 × 日志定义)
  collection_enabled          Boolean，NULL/无行 = 采；FALSE = 该服务不采这条日志
  retention_tier_id           保留档位覆盖（NULL = 继承服务默认）
  collection_filter_rule_id / collection_exclude_filter_rule_id
                              采集过滤的服务级覆盖（迁移 000039 起**已真正接入渲染**，见 §6.1）。
                              **三态**：NULL 继承模板 / 0 显式关闭该方向 / >0 指定规则
                              （两列**不带外键**：0 是有效取值，迁移 000040 摘掉了真库上 include 列的
                              Django 时代外键，见 §6.1）
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
- 校验规则（档位 code 格式、`daily_size_gb > 0`、`retention_value ≥ 1` 且**折算成小时后 ≤ 3650 天**、
  `retention_unit ∈ {d,h}`、`rollover_min_index_age` 形如 `30m/12h/1d`、
  规则名 `^[a-z0-9][a-z0-9._-]*$`、`pattern` 不得含换行且必须是合法正则、`flush_timeout ∈ [100,60000]`）不变；
  解析规则保存前先发布 pipeline 到集群、失败则整条请求 400 且不落库的行为不变。
- 删除仍是"逐 id + 汇总 `{count, results}`"的批量语义；档位被逻辑服务/日志设置引用、规则被日志定义引用时拒绝删除
  （引用计数用取行/计数查询，不再 `SELECT COUNT(*)` 兼职判存在）。
- **解析规则的引用关系读路径**（2026-09-21 加）：`GET /monitor/log-processing-rules/usage/`
  （`ListProcessingRuleUsages`，sqlc 查询双方言同名）。一次全量返回每条规则被哪些部署模板的
  日志定义引用，关联链为 `monitor_log_processing_rule ← assets_application_log_definition
  .processing_rule_id（迁移 000035 后唯一来源）→ deployment_template_id → 模板`，
  另带 `service_count`（引用该模板的逻辑服务数，量化"改这条规则影响谁"）。
  行 = (规则 × 模板日志定义)，按 `rule.name, template.name, log.name` 排序；未被引用的规则
  不在结果里（删除时的拒绝提示已足够发现它们）。不分页：规则量级小、行数随引用数线性，
  前端在「日志处理规则」页新增的同名 tab（懒加载，进入 tab 才拉取）里按左侧应用筛选做客户端过滤。
  tab 内交互（2026-09-21）：`所属应用` 列按 application id 从页面已加载的应用列表就地反查名称；
  关键字框匹配 模板名 / 日志定义名 / 路径（与左侧应用筛选叠加，均为客户端过滤）；
  `影响服务数 > 0` 渲染为链接，点击调 `GET /assets/application-deployment-templates/:id/services/`
  （`ListApplicationServicesByTemplate`，按 deployment_template_id 查逻辑服务，join
  business_system → project 反查项目名，行含 项目/业务系统/环境/服务名/enabled，全量不分页）
  弹出服务清单；点服务名带 query（application_service_id/service_name/business_system_id/
  environment_id/environment_name）跳转服务树页，该页 onMounted 读 query 直接把 scope
  定位到该服务节点（深链，见 assets/service-tree/index.vue 的 applyQueryScope）。

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
  processors:                                # 把真实文件路径拷到顶层 log_path（不是路径模式）
    - copy_fields:
        fields:
          - from: log.file.path
            to: log_path
        fail_on_error: false
        ignore_missing: true
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

**期望指纹（`config_fingerprint`）的构成**——`fingerprint = sha256("output:" + outputIdentity + "\n" + fmt.Sprintf("%v", fragments))`，**主机级、含该主机上所有服务的片段**（不是按服务）：

| 组成部分 | 具体内容 | 变化会不会改指纹 |
|---|---|---|
| 输出段 `outputIdentity` | `url=<默认集群的**第一个**地址 scheme://host:port>`、`username=<账号>`、`verify_tls=<bool>`（`filebeatOutputIdentity` / `firstElasticsearchURL`） | 会 |
| 输出段**不含** | ES 口令、CA cert、`request_timeout`、地址列表里第一个之外的地址 | 不会（只改口令不判漂移，需人工重发一次） |
| 片段集合 `fragments` | 该主机上**全部**"启用且在采集"的 服务×日志定义 各一个片段，按路径排序；每项为 `{Path, Content}` | 会 |
| 片段文件名 `Path` | `/etc/filebeat/inputs.d/<app>__<service>__<logname>.yml`（换应用/换服务/改日志名都会变） | 会 |
| 片段内容 `Content` | `paths`（宏展开后的绝对路径）、`index`（data stream 名）、`pipeline` 名、multiline parser、`include_lines`/`exclude_lines`、`fields_under_root` 维度字段、`processors`（`log.file.path → log_path`） | 会 |
| 目录占位 | 有片段时额外追加 `/var/lib/filebeat/.keep`（空内容） | 仅"有无片段"改变时 |
| **不算入** | Filebeat/agent 版本、索引模板 mapping、pipeline 内容、规则样例日志、格式认证状态、主配置里固定不变的 `filebeat.config.inputs.reload.*` | 不会 |

推论：主机指纹含该主机上**所有服务**的片段，所以改服务 B（即使还没下发）会让**这台主机的期望指纹**变——这是
主机视图（「日志采集」页 / 体检 / 一键下发）的正确行为（整机确实没同步）。但**服务视图不能跟着把它算到 A 头上**，
否则共享主机上改 B、A 也会提示"期望指纹变了/待下发"（现场问题）。为此引入下面的服务级子指纹。

**服务级子指纹（`monitor_log_collection_target.service_fingerprints`，2026-09-20）**

`service_fingerprints` 是一个 JSON `{service_id: subfp}`，存**每个服务在该主机上**的已下发子指纹：

| 项 | 内容 |
|---|---|
| 子指纹定义 | `subfp(host, service) = sha256("output:" + outputIdentity + "\n" + 该服务在该主机的全部片段)` |
| **含** 输出段 `outputIdentity` | 改 ES 地址/账号/TLS 时所有服务都正确判 `drift` |
| **不含** `.keep` | 它只看"整机有没有片段"（主机级信号）；算进去的话新增 B 会让 A 的子指纹也变，又回到老问题 |
| 归属 | 一个片段文件 `<app>__<service>__<serviceID>__<logname>.yml` 只属于一个服务，按服务归组无歧义 |

> key 用**服务 id** 而不是编码：编码的唯一域是 `(业务系统, 环境)`（000044 迁移），同一主机上
> （尤其跨业务系统共享主机时）可能出现两个同 code 的服务，用编码作 key 会互相覆盖。片段文件名
> 同理带上 id，避免两个同 code 服务的 Filebeat input id 撞车。000044 迁移把存量 `{code: subfp}`
> 统一清空，下发一次后按新 key 恢复。

- **下发**：`applyLogTargetConfigRow` 成功后**整体替换**为该主机本次渲染出的服务集合（消失的服务被移除，
  服务视图据此判"需要下发一次清理"）；跳过判定除主机指纹外**还要**比对服务级指纹——否则存量目标
  （`service_fingerprints='{}'`）会一直跳过、服务级记录永远补不上。
- **服务视图状态**（`buildServiceHostStates` → `EvaluateServiceLogConfigStates`）：期望取 `subfp(本服务)`、
  已下发取该主机的 `service_fingerprints[本服务id]` → `synced / drift / never`。**共享主机上改 B 只影响 B**。
- 语义边界：本服务在该主机没有片段（如停采），期望为空——已下发也为空 → `synced`；已下发非空 → `drift`（主机上还残留旧片段，需下发一次清理）。
- **主机视图 / 体检 / `pending-summary` / 一键下发仍用主机指纹**：它回答的是"整机是否最新"。
- **一次性影响**：存量目标该列为 `{}`；服务视图先用**整机指纹兜底**（整机与期望一致即 `synced`，不误报
  `never`），该主机任意一次下发后补上服务级记录、转入服务级判定。

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
        **保存时就拦**：`CheckServiceLogConfigConsistency`（logcollect，反向注入给 assets，
        router 里 `SetLogConfigConsistencyChecker`）在**保存逻辑服务的事务里**把该服务各承载主机的
        期望配置渲染一遍，出现"硬问题"就**整体回滚 + 400**（`renderedHostLogConfig.Errors`）。
        硬问题 = 只能回到配置里改的那些：日志定义名含未展开宏、首行正则/采集过滤正则编译不过、
        实例路径含未定义宏、同主机上展开成同一路径。**软告警不挡保存**：未挂解析规则所以不采集、
        主机还没纳管采集目标这些是按约定允许存在的状态（"先建模板、后纳管"是正常顺序）。
        为什么必须在这一步拦：渲染遇到这些问题是"跳过并告警"，可那已经是下发那一步了——
        用户要等到某台主机少采了、或同一条日志进了两次 ES 才发现。
        **同一主机上同一个绝对路径只允许一条 input**（2026-09-19 补回 Django 版的守卫，迁 Go 时丢了）：
        两个 filestream 各自维护 offset，同一份日志会进 ES 两次（instance 字段不同），下游错误聚类与
        容量统计跟着翻倍。多实例服务最容易踩——实例没配各自的 APP_HOME 就展开成同一条路径。
        处理与"未展开宏""未挂规则"一致：**保留先出现的、跳过重复并告警**（说清两侧是谁 + 怎么改），
        不带病下发也不静默重复采集。
        路径按 服务级+实例级宏替换 ${VAR}（宏来源，优先级从低到高：
        部署模板 macro_definitions 的 value（默认值，与前端服务弹窗展示口径一致）→
        服务级 macro_values（覆盖同名项）→ 部署实例 runtime_variables；部署模板 app_home
        作为 APP_HOME 默认值；替换后仍含 ${VAR} 的实例跳过并记 warnings）；
        **宏的"必填"边界（2026-09-19）**：模板给了默认值 → 服务可继承；模板没给默认值 →
        保存逻辑服务时就要求本服务填（服务弹窗拦保存，并标红"必填"）——否则下发时那条路径
        要么让整台实例被跳过、要么被替换成空串拼出坏路径（如 `/catalina.out`），Filebeat
        监听不到文件而采集静默为空，两者都是"事后才发现"的坑；
        日志定义 name 直接作为文件名/维度值、不参与宏展开——name 含 ${...} 时跳过该定义
        并记 warnings（宏应写在 path_pattern 里，否则会生成监听不到文件的坏片段）；
        日志定义未关联处理规则（无 pipeline）时不采集：跳过该日志定义并记 warnings
        （避免"采进来了但查不到 log_level/log_message/error_fingerprint"的半成品数据）；
        多行首行正则 Filebeat 编译不过（RE2，见 §2）时同样跳过并记 warnings——坏片段会让
        Filebeat 起不来，整台主机的日志一起停，比少采一个文件严重得多；
        采集过滤（§6.1）写上 `include_lines` / `exclude_lines`（各一个正则，来自规则表）；
        规则被删/停用/方向不符/正则编译不过时**忽略该方向**（不写这两项）并记 warnings——
        过滤是可选优化，忽略它的后果只是采多了，绝不能因此让 Filebeat 起不来或停采这条日志；
        fields_under_root 注入 service/instance/application/log_name/
        business_system/project/environment/host_ip；另加 input 级 copy_fields 处理器把
        log.file.path 拷到顶层 log_path（该事件实际来自的文件，不是路径模式）；
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

#### 8.4.1 路径冲突校验覆盖的写路径

判据：同一台主机上，两条「服务 × 日志定义 × 实例」展开后的**日志文件路径相同或 glob 重叠**，即视为冲突
（`*` 不跨 `/` 分段；`**` 平台不支持，保存时直接拒绝——避免"Filebeat 会展开、平台不识别"的口径分叉）。
冲突会让两个 filestream 监听同一文件、同一份日志进 ES 两次，所以必须**保存即报错**，不能等到下发才"跳过并告警"。

| 写路径 | 检查内容 | 现状 |
|---|---|---|
| 保存逻辑服务 `SaveApplicationService` | 该服务各承载主机渲染，任意两项重叠即拒 | 已有（精确串）；升级为 glob 重叠 |
| 保存部署实例 `SaveApplicationDeployment`（改 `runtime_variables`/`APP_HOME`/换主机） | 该实例绑定的**所有服务**按主机渲染校验 | 缺口，新增 |
| 保存部署模板 `SaveDeploymentTemplate` | ① 模板内：提交的日志定义两两 `path_pattern` 重叠即拒（不需要主机）②（推荐）fan-out：引用该模板的所有服务按主机渲染校验（因为改 `app_home`/`macro_definitions` 会挪动所有服务路径） | 缺口，新增 |

**为什么必须覆盖到部署实例与部署模板**：冲突不一定在"保存服务"时引入——改实例的 `runtime_variables`
（宏、`APP_HOME`）、或改模板的 `app_home`/宏默认值/日志定义路径，都会挪动展开后的路径；只堵"保存服务"
会漏掉这两条入口，冲突仍会被静默写入，直到下发时以"跳过 + 删片段"暴露。

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
| `GET /monitor/log-targets/service-config-state/?application_service_id=` | 承载该服务的**主机清单**（区分已纳管/未纳管）+ 各主机配置态聚合 + `agent_online` / `runtime_status`（原样带出，供页面按服务展示与刷新） |
| `GET /monitor/log-targets/service-collection-chain/?application_service_id=` | **按服务的采集链路诊断**（"为什么没日志"，见 §9.5） |
| `POST /monitor/log-targets/service-apply/`（body `{application_service_id}`） | 对承载该服务的全部**已纳管**主机建一次批量作业（复用 §8.8 的作业机制，进度可查） |

三个必须守住的语义：

- **解析主机时不过滤启用态**（`ListServiceLogApplyTargets` 不滤 `s.enabled` / `s.log_collection_enabled` /
  `sd.enabled` / `d.enabled`）。停用服务或关掉采集之后，恰恰需要下发一次才能**移除**主机上的旧片段
  （渲染层把停用服务排除 → agent 删掉未交付的 `.yml`，见 §8.6）。按启用态过滤主机，停用的服务
  就永远清不干净。变化后的主机是否需要真下发由渲染指纹决定（一致则跳过，见 §8.7 之后的配置态）。
- **未纳管的主机必须显式回报**（`target_id` 为 NULL 即未纳管）：下发不了它们，接口把主机名列出来，
  否则用户以为整个服务都下发了，实际少了几台。
- **状态按 (主机 × 服务) 判，聚合只给计数**：服务视图用**服务级子指纹**逐台判 `synced/drift/never`
  （见 §8.3）——共享主机上其他服务的改动不会把本服务带成待下发。接口只回"N 台里 M 台待下发"这类计数，
  **不暴露单一"服务级指纹"**：同一服务在不同主机上因实例级 `runtime_variables` 不同、渲染结果本就不同。

**幂等且廉价**：指纹一致的主机在 `applyLogTargetConfigRow` 里被整段跳过（只刷新 `last_applied_time`），
所以"把服务的主机都下发一遍"不必先筛待下发。

**文案要说实话**：按钮语义是"对承载本服务的主机重新下发"，会连带重算同主机上其他服务的配置
（渲染确定性所以内容等价；若它们本来就有未下发的改动，会被一起带上——通常是想要的）。

**入口**：日志中心页「日志配置」tab 顶部（聚合状态 + 下发按钮 + 作业进度）。按主机的单条下发
（「日志采集」页）与整批下发（`ids` 省略 = 全部待下发）都保留，三者互补。

### 8.8 配置状态（期望 vs 已下发）与链路体检（2026-09-18）

**配置态**回答"主机上的采集配置是不是当前该有的那份"，与运行态（agent/Filebeat 是否在跑）
正交：配置一致不代表进程在跑，进程在跑也不代表配置是最新的。

> 体检（下面六层，**集群维度**）与日志中心的**按服务**采集链路（§9.5）共用同一批评定函数：
> `pipelines` 层用 `judgeRulePipeline`、`data_flow` 层用 `queryLogDataFlow`、主机三层用
> `buildServiceHostStates`。改判据只改一处，避免两个入口对同一台主机/同一条规则给出不同结论。

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
  `expected_fingerprint` / `config_service_num` / `config_warnings`（未关联处理规则、路径宏展不开、
  首行正则 Filebeat 编译不过等渲染告警），页面级 `config_state_error`。未纳管的主机没有日志目标，
  **不参与任何配置态**（否则"待下发"里会混进根本没纳管 Filebeat 的机器）。
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

#### 8.8.1 差异内容：怎么看"这次下发会改什么"（2026-09-20）

配置态只说"一致/待下发"，**不回答差在哪**——用户不知道要改什么就没法判断该不该点下发。
`GET /monitor/log-targets/:id/config-diff/?application_service_id=<可选>` 补上这一环：

- **期望侧**：与下发**同一条渲染路径**（`renderHostLogConfig`，同索引前缀、同宏展开、同排序）
  → 一串 `{path, content}` 片段，所以看到的就是 agent 将写入的内容。
- **已下发侧**：用 agent 读回主机上 `/etc/filebeat/inputs.d/*.yml`（`ListFiles` 列目录 +
  `StatFile` 取大小 + `ReadFileChunk` 读内容）。**为什么必须读主机**：库里只落指纹
  （`config_fingerprint` / `service_fingerprints`），没有内容，纯库内不可能算出差异内容；
  唯一的替代是"把下发过的内容也存一份"，那等于再造一份会漂移的真相。
- **逐文件判定**：`added`（期望有、主机没有 → 会新增）/ `removed`（主机有、期望没有 → **会被删掉**，
  因为 agent 侧的全量替换语义，见 §8.7）/ `changed`（两边都有但内容不同）/ `unchanged`。
  两侧内容都原样返回，行级 diff 由前端算（`fronted/src/util/configDiff.js`，LCS；展示样式不该由后端定死）。
- **读不准时不给结论**：主机离线/未接线 → `read_error` 有话说，`files` 只含期望侧
  （界面明说"这是**将要下发**的内容，无法与现状对比"）；单个文件读失败/超过 256 KB 被截断 →
  该文件标 `read_error` / `applied_truncated` 并另计 `unread`（既不说"一致"也不说"不一致"）。
- **他服务片段也一并给出**：`service_id` / `service_code` 从文件名解析
  （`< app >__< service >__< serviceID >__< log >.yml`），界面默认"只看本服务"、其余折叠并标「他服务」
  ——下发是主机级全量替换，本服务之外的片段确实会一起被重写，瞒着用户反而更危险。
- **只读**：不写库、不下发、不改主机上的任何文件；读回的文件名过白名单
  （`safeConfigBaseName`：只允许 `[A-Za-z0-9_.-]`、禁 `..`），与下发写入的范围严格对称。
- **入口**：日志中心「日志配置」tab 的下发区「查看配置差异」，默认落在**待下发的那台**主机上
  （那才是用户点开它要问的问题），弹窗里可换主机。差异是"某台主机的现状"，多台各不相同。

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
| 停用 / 删除逻辑服务、删除部署模板 | 变（批量片段消失） | 待下发 | 该服务（或该模板下全部服务）的片段被删，Filebeat 重启 | 流变孤儿（维度已删 → 水位视图"未识别"） | ⛔ 保持人工：影响面是该服务的全部主机，且与资产生命周期动作耦合 |
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

**前端入口是「日志管理 → 日志中心」的水位 tab**（`/monitor/logging/center`，迁移 000037）：
选中服务时是"本服务水位"，未选中时是"存储水位"（全量视图，再按左侧服务树选中的层级过滤，
并给出该层级的容量聚合）。页面形态与两个视图的差异见 §9.5。

> 曾经还有一个独立的「存储水位」页（`/monitor/logging/overview`，迁移 000019），它的能力
> （层级树导航、集群与数据时间、节点磁盘水位、统计、流明细、未识别流）已逐条并入日志中心，
> 2026-09-19 由迁移 000038 删除该菜单并移除页面组件；旧地址保留 redirect 到日志中心。
> **接口不删**——两个视图消费的是同一个接口，数字永远一致。

**数据口径（关键）**：真实磁盘占用/rollover 状态的原子粒度是 data stream
（命名 = `autoadmin-<项目>-<业务系统>-<环境>-<档位编码>`，见 4.1）。流的项目/业务系统/环境层
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
- `GET .../log-service-usage/?business_system=&environment=`：`service` 字段 terms 聚合文档数
  （默认近 30 天），索引匹配用 `<prefix>-*<业务系统>-<环境>-*`（项目段通配）。
  **当前无消费方**：水位视图的逻辑服务层直接用流名解析出的服务聚合（口径更准，且不额外打 ES），
  这个"只按文档数、按需查"的接口是更早那版层级树的遗留；保留接口是因为它对大集群仍是
  最省成本的"某环境下各服务的写入量"查询，将来要给它接入口时不必再造。
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

**唯一入口：日志中心**（`/monitor/logging/center`，菜单「日志管理 → 日志中心」，迁移 000037）：
左侧是所有资产页共用的服务树（`ServiceTree`，用它的 `groupByProject` 选项显示项目层级），
右侧三个 tab。**tab 顺序按使用频率排：日志查询（默认打开）→ 日志配置 → 存储水位**，
所以选中服务后第一眼就是最常用的检索框：

| tab | 数据来源 | 说明 |
|---|---|---|
| 日志查询 | `LogQueryPanel`（`views/monitor/log-center/LogQueryPanel.vue`，从服务树目录迁入，原服务树页的查询面板）；非服务节点是 `LogLevelOverview`（见 §9.5.2） | 检索接口硬性要求 `application_service_id`，由树的选中节点提供。**默认 tab**。未选服务（项目/业务系统/环境/全部）时不给空态而是**层级视图**：该范围的服务清单 + 最近写入，点行进入该服务的检索面板（检索本身仍要求选到具体服务，见 §9.5.2）。关键词框有**两种模式可切换**（见 §9.7）：`正文`（默认，只在 `log_message` 里搜）/ `Lucene`（完整语法，可按字段过滤） |
| 日志配置 | 服务节点：`GET /assets/application-services/:id/log-config/` + `GET /monitor/log-targets/service-config-state/` + `GET /monitor/log-targets/service-collection-chain/`；非服务节点：`POST /assets/application-services/log-status-summary/` + 服务/项目/业务系统名单（见 §9.5.2） | 日志名/路径/**所在主机**/处理规则/采集开关/档位/格式认证状态/data stream + **服务级采集总开关**（`log_collection_enabled`，响应顶层字段）+ **采集链路状态条**（见下）。总开关与逐条开关是两层：前者关掉后该服务下所有日志都不采集、逐条开关不生效（配置意图保留）。两者都可直接改（见下方「两条写路径」）；**表格有勾选列，勾选后出现批量栏**（打开/停止采集、档位、采集过滤、格式认证，见 §9.5.0「批量」）；认证入口复用共享组件 `LogFormatVerifyDialog`（与编辑弹窗同一个，见 §4.8）。`force-render`：切走再切回不重新取数 |
| 存储水位 | 选中服务时 `.../log-storage-overview/?application_service_id=<服务 id>`（后端收窄 ES 查询；兼容旧的 `service_code` 参数作兜底）；未选中时取全量再按树的层级（项目/业务系统/环境）用 dims 映射到编码过滤 | **按层级聚合的容量统计**（全部→项目、项目→业务系统、业务系统→环境、环境→逻辑服务；占用降序 + 占比 + 其中历史档位 + **同名饼图**）、**集群 + 数据时间**、统计（流数/文档数/总占用拆分活跃与历史/**健康异常流**）、**Elasticsearch 节点磁盘水位**、流表（占用/文档数/ILM/**状态**/**后备索引展开**）。**每条流都有操作列**（见下）；未选中时列出**未识别流**（不归属任何服务，任何层级都保留，不进聚合） |

> **原「服务树页 → 日志查询」tab 已删除**（2026-09-19）：同一件事有两个入口时，用户会在两个地方
> 看到不同版本（一个带项目层级的树、一个不带）的同一个检索面板，配置类操作又只在日志中心有。
> 现在检索只在日志中心，服务树页只保留「资源详情」。

**聚合分组的集合来自 dims，不是来自流**：每层**已配置的成员都列出来**（含 `streams = 0` 的，标「无日志」），再补上"流里有、维度表里没有"的编码（维度停用/删除后的遗留流，标「维度已停用」）——两个方向都不能漏，漏了前者看不到空分组、漏了后者这些流会从统计里消失而明细表里还在。饼图只画**有占用**的分组（占比对 0 没有意义，零值分组仍留在表格里），超过 8 项合并为"其他 N 项"。其余口径：未识别流不进聚合；`historical` 由后端判定（见下）。

三个 tab 共用一个服务上下文，这是把它们合成一个页面的理由：日志检索接口必填
`application_service_id`，水位与日志配置也都以服务为维度，而原先它们分散在三处（存储水位页、
服务树页的日志查询 tab），看同一个服务的日志要来回跳。

**「路径」列有个小开关：原始 ↔ 解析后**（2026-09-19 加）。日志中心「日志配置」与通配展开
（`LogGlobPreview`）共用 `fronted/src/util/logPathMacro.js`：

| 档位 | 取值 | 含义 |
|---|---|---|
| 解析后（默认） | `resolved_path`，**编辑弹窗里按表单实时重算** | 按**与渲染同一顺序**展开到"服务这一层能解析的部分"：模板 `macro_definitions` 的 value → 模板 `app_home`（`APP_HOME` 默认值）→ 服务 `macro_values` 覆盖。弹窗里改「宏」时前端用同一套顺序**当场重算**（`frontend/src/util/logPathMacro.js` 的 `resolvePathMacros`，服务端 `shared/logmacro` 的移植）：否则改完宏要保存后重进弹窗才看到新路径，用户会以为"改了没生效"（2026-09-19 现场）。日志中心那列是只读展示，直接用接口返回的 `resolved_path` |
| 原始 | `path_pattern` | 模板里存的路径模式，一个宏都不展开（排查"这个宏是哪一层给的"时用） |

- **合并顺序只有一份实现**：`internal/shared/logmacro`（`Resolve`/`Merge`/`TemplateDefaults`/
  `InstanceValues`/`Pending`），下发渲染（`logcollect`）与界面展示（`assets` 的 log-config）都调它。
  两边各写一份迟早出现"界面显示的路径与主机上实际采的不一样"——那比显示占位符更糟。
- **实例级变量不猜**：`runtime_variables` 只有到主机上才知道，所以解析后仍留着的宏由后端
  `pending_macros` 列出来，界面用橙色标签标注（"`${APP_HOME}` 实例上展开"）+ tooltip 指向真实路径
  的出处（「日志采集」页的主机配置预览；认证按实例抽样用的是同一套展开）。
  主机的真实路径由渲染算：`resolveMacros(entry.ResolvedPath, …)` 再叠实例变量。

**「所在主机」列回答"这条日志在哪几台机器上"**（2026-09-19 加，现场反馈"日志配置里看不到日志所在
服务器的 IP"）：主机取值与下发区同源（`service-config-state` 的 `hosts` + `unmanaged_hosts`，即
"承载该服务的全部主机"），**不是**从日志定义推的——日志定义的 `path_pattern` 是模板级、路径里的宏
在每台主机上各自展开，所以同一份日志定义就落在这几台机器上，表格里每行内容相同。显示上首台给
`主机实例名（IP）`、多台折叠成"N 台"、全部明细进 tooltip；**未纳管的主机一并列出并标注**（它们只在
资产里绑了实例、还没纳管日志采集，配置下发不到），漏掉会让用户以为这个服务就这几台机器。

**「采集链路」状态条回答"为什么没日志"**（2026-09-19 加，现场反馈"查不到日志时不知道断在哪"）。
按选中服务逐层给结论，异常层在颜色与明细里直接可见（`GET /monitor/log-targets/service-collection-chain/`）：

| 层 | 判据 | 数据来源 |
|---|---|---|
| Agent 在线 | 该主机的 agent 会话在不在 | 网关实时（`gateway.IsOnline`），不是库里的字段 |
| 采集进程 | Filebeat `running / stopped / error / 未知` | `monitor_log_collection_target.runtime_status`，**落库快照**（未知时页面自动查一次，见下） |
| 主机配置 | 期望指纹 vs 已下发指纹（`synced / drift / never / unknown`） | 与下发区同一份评估（`buildServiceHostStates`） |
| 解析规则 | 该服务日志定义引用的规则，其派生 pipeline 在**规则自己的集群**上是否存在、与页面是否一致、**且能不能跑通**（用规则自带样例 + Filebeat 真实载荷试跑一次） | `judgeRulePipeline`（与体检 `pipelines` 层同一个函数） |
| 数据写入 | 最近 30 分钟内按 `service` 字段收窄的文档数 | `queryLogDataFlow`（与体检 `data_flow` 同一查询），查**默认启用的集群** |

几条刻意的设计：
- **判定只在后端一处**（复用的函数名写在表里），前端只呈现。两处各算一套必然出现"体检说没事、
     日志中心说没发布"。前端不自己拼状态文案，刷新运行态后重新拉链路。
- **"状态未知" ≠ "已停止"**：`runtime_status` 为空是"没查过"，处置是刷新一次；说成"已停止"
     会把人引去重启一个本来在跑的服务。所以要分开计数（`filebeat_unknown`）。
- **运行态快照的新鲜度由页面自动维持**（2026-09-19 改，现场反馈"日志配置里每次修改了东西都要手动
  刷新运行态，麻烦"）：`runtime_status` 是落库快照，只有"查状态"动作会写它，所以两处**自动查**——
  ① 链路读回来后，若存在**状态未知**的已纳管在线主机（新纳管、或刚下发重启过还没确认）就自动查一次；
  ② 下发作业结束时自动查一次（下发会重启 Filebeat，重启后的进程态谁都不知道，而这正是用户最想看的）。
  节流：同一个页面 60 秒内最多自动查一次（主机侧命令失败时快照会一直为空，没有节流就会反复打主机），
  且**状态已知时一次都不查**——自动刷新不能变成"每打开一次页面就把全网主机 `systemctl` 一遍"。
  「刷新运行态」按钮保留，用于强制再查一次。链路条上的时间标签是**本次检查**的时间，
  不是 Filebeat 状态的时间（那份快照的时间不落库，靠上面两条自动流水保持新鲜）。
- **未纳管主机在每一层都出现且是 `warn`**：它不是配置漂移（报 `drift` 会让人去点下发，
     而那台机器根本没有采集目标），但也不能因为未纳管就报 `ok`——"3 台里 1 台没采"就是要看见的问题。
- **服务停用 / 服务级采集开关关闭单独提示**：它是"没日志"最常见的原因，却不在上面五层里。
- 集群不可用时只有 ES 两层报错，主机三层照常给结论（它们与集群无关），不整体失败。
- **"解析规则"层判红 ≠ 一定没配规则**：2026-09-19 现场（kul 的 tomcat）四层全绿、`数据写入` 才是
  warn，根因却在这一层——规则把事件弄成了 ES 拒收的形状（见 §5.2 的"试跑"判据）。所以这一层
  的 detail 会把"会被 ES 拒收"与最短原因一起写出来，而不是只报"缺字段"。

**水位按服务收窄是后端做的**：带 `application_service_id` 时后端用识别环节算好的维度段把 ES 查询
收窄成 `<前缀>-<项目>-<业务系统>-<环境>-<服务>-*`（`scopeIndexPattern`），而不是取全量再前端过滤——
`_cat/indices` + ILM explain 是这一页最大的成本。**不带该参数时是"全量视图"**（未选中服务，
或选中的是项目/业务系统/环境）：取全量、前端按树的层级过滤与聚合。连接键用 **id 而非编码**：
服务编码的唯一域是 `(业务系统, 环境)`（见 assets 000044 迁移），允许跨业务/环境重复，只凭编码
会命中错的维度段；旧的 `service_code` 参数仅作兜底。服务不存在时接口返回空视图而**不回落到全量**
（否则调用方会以为看到的是"这个服务的流"）。水位只在切到该 tab 或手动刷新时取一次。

**流表的「操作」列：每条流都能清，清理分两条路径**（2026-09-19 扩到所有流，此前只有
「选中服务 + 历史档位流」才有入口）。**列本身恒在**——曾按"是否选中服务"整列开关，
于是全量视图/项目/业务系统/环境节点下根本没有这一列（2026-09-19 修；当时的用例只调了
组件方法、没断言渲染出来的列，所以没拦住）。「切回该档位」是例外：它要改**这个服务的哪条日志定义**，
只有选中服务时才有上下文，全量视图下只给清理（清理按服务维度执行，服务 id 由响应里的 dims 映射得到，
不需要先选中服务）：

| 情况 | 动作 | 走哪条接口 | 为什么 |
|---|---|---|---|
| 历史档位流（已停写） | 切回该档位 / 清理数据 | 服务维度 `POST /monitor/log-datastreams/cleanup/`（带 `tier`） | 档位能切回来，清理前先给"回收"的机会。按钮文案与在写的流**统一成「清理数据」**（2026-09-20）：那是同一个动作（都按流删文档），差别只在后果，放在 tooltip 里说 |
| 采集中 / 已停用的**已识别**流 | 清理数据 | 同上（带 `tier`，只清这一条流） | 服务维度能自己拼流名并**按档位收窄**，不会误伤同服务的其他档位 |
| **未识别流**（以及档位段不在档位表里的历史流） | 清理数据 | 按流名 `POST /monitor/log-datastreams/cleanup-stream/` | 没有服务可归属，只能按名字；服务端**只收一条具体的数据流名**：通配符 / `.` 开头（后备索引、系统索引）/ 前缀之外的名字一律 400，且必须真的存在于集群（不存在报 404），绝不退化成"删任意索引"的口子 |

两条路径的**删除语义完全一致**：`_delete_by_query` 异步删文档、**保留 data stream 对象**
（继续采集/切回档位仍写入这条流）；`mode` 也共用（`all` / `hours` / `days`，解析在
`parseCleanupMode` 一处，页面上目前只用 `all`）。

**清理的文案按危险度分级**（同一个按钮，三种后果不同）：历史档位流说清"只影响这个档位、流对象保留"；
**活跃流必须说明"清理不会停止采集，之后新写入的日志仍会进入这条流"**——这是最容易误解的一点
（用户以为清理=停采）；未识别流说明"不归属任何服务、清理不影响任何采集"。

**要不要给所有流都开清理入口？要**（2026-09-19 定）：真实需求有两类——误采/测试写入的垃圾、
以及"没人认领的未识别流"（旧服务下线后留下的流，没有任何服务的开关能管到它）。限制入口反而会
逼用户去 ES 手删（更危险）。安全性由上面那三条服务端校验兜住，而不是靠"少给入口"。

**两个视图（按服务 / 全量）覆盖了旧「存储水位」页的全部能力**，所以那个入口在 2026-09-19
由迁移 000038 下线（菜单 + 页面组件删除，旧地址 redirect 到本页）。逐条对照：集群与数据时间、
节点磁盘水位、健康异常流计数、未识别流容器、后备索引明细都与旧页一致（同一个接口，数字永远一致）；
旧页的"顶层 → 项目 → 业务系统 → 环境 → 逻辑服务"层级树由左侧服务树 + 按当前层级的聚合表
等价承担——从容量视角自顶向下看，选到某一层就看这一层的下一层聚合（全部→项目，项目→业务系统…），
选中服务则直接看它自己的流。

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

注意：这个判定在**流/档位**粒度上是精确的（同一档位下多条日志共用一条流），而且对
按服务视图与全量视图都成立——`activeTiers` 覆盖全部服务，每条流按自己的归属服务查生效档位集合
（`datastream_status.go`）。全量视图里前端不需要知道生效档位，直接用后端给的 `historical` 标注；
旧「存储水位」页当年没消费这个字段，日志中心两个视图都消费。

**历史流怎么处置**（页面给了两个动作）：

| 情况 | 做法 |
|---|---|
| 以后可能切回该档位 | 什么都不用做。流名由生效档位决定（`LogDataStreamName(..., 档位)`），切回该档位就**继续写入同一条流**，不会新建一条空流；「历史档位」标签是活状态，切回来自动变回「采集中」。页面提供「切回该档位」按钮：勾选要改的日志（同档位多条日志共享一条流），逐条按行提交。 |
| 永远不切回 | 多数情况也不用管：档位 ILM 的 hot 阶段 rollover 带 `max_age`（= 档位的 `rollover_min_index_age`，**档位表单必填**、校验 `^\d+[mhd]$`、库里 NOT NULL），按时间触发不依赖写入，所以停写的流照样 rollover 并进入 delete 阶段到期删除。只有"保留期长 + 数据大 + 现在就要释放"才需要动手：历史流行上的「清理数据」按 (服务, 档位) 删掉这条流的文档（见 §9.6）。 |

两个动作都要说清一件事：**下发对历史流无效**——它已不在下发范围内，下发只影响未来的写入。
占用在汇总里拆成"活跃 / 历史"两个数（前者是当前成本，后者是待释放的沉淀），合成一个数
既看不出问题、也没法判断该不该清理。

**没有日志定义时也不隐藏存量流**：模板里的日志定义被删光之后，这个服务什么都不采了，但 ES 里
的存量流还在。水位查询用的是 `log-config` 响应**顶层**的 `service_code`（而不是 `logs[0].service_code`），
因此照样能查到；此时没有生效档位，页面把所有存量流都标成历史档位。判定里还留了一条：后端
没给生效档位（旧构建）时不妄判，宁可沿用“采集中”也不要把在写的流错标成停写。

**边界**：本服务水位只含“按新命名能识别到本服务”的流；旧命名流与未识别流没有服务归属，
要看它们就把树选到项目/业务系统/环境或全部（水位 tab 切到全量视图，未识别流在那里被单独提示
并标注）。**下发**在本页有服务级入口（聚合状态 +
一键重下发承载主机），但底层仍是逐台全量下发，见 §8.7；按单台主机下发仍在「日志采集」页。

#### 9.5.0 这一页的改动语义（2026-09-19 统一）

日志中心「日志配置」上可改的东西不少，但**语义只有两条轴**，界面上此前没说清（"有的直接入库、
有的没保存"的观感就来自把这两条轴混在一起看）；现在页头有一段固定说明 + 各列 tooltip 统一措辞：

| 轴 | 规则 |
|---|---|
| **入库** | **改一下即时入库**（这一页没有"保存"按钮）：服务采集总开关（`SetServiceLogCollection`）、逐条采集开关 / 保留档位 / 采集过滤（`SaveServiceLogOverride` 按行 upsert）。改完**立刻回读**页头的"待下发 N 台"**并重拉采集链路**——不回读的话用户改完看不到任何反馈，会以为没保存成功；不重拉链路的话「主机配置」层还停在"一致"的旧结论上，用户会去点「刷新运行态」来顺带刷新它（2026-09-19 现场）。重拉链路只是读库 + 读 ES，**不会**为此去打扰主机 |
| **生效** | **入库 ≠ 生效**：还要点页头「下发本服务的采集配置」才写到主机上（agent 侧是全量替换，最小单位是主机，见 §8.7）；指纹变了所以待下发计数会立刻变化 |
| **只读** | 日志名称 / 路径 / 处理规则跟着**部署模板**走（要改去模板的「路径与文件 → 日志」）；格式认证是**动作**不是配置，走每行的「发起认证」弹窗 |

**这一页每个可写控件都先确认、确认后才入库**（2026-09-19，统一成一套节奏）：
服务采集总开关、逐条采集开关（只有"关"要确认，"开"是回默认不拦）、保留档位、采集过滤（两个方向）。
共用 `confirmConfigChange`，确认框固定三段：**这一处会怎样** + **能不能回头** + **即时入库、
点页头「下发本服务的采集配置」后才在主机上生效**。各处的"会怎样"如实写：关服务采集=该服务下全部
日志停采（存量不删）、改档位=写入新流且旧流停写（不迁移数据）、加过滤=被滤掉的记录不进 ES
且事后补不回。

为什么统一成"都弹"而不是只给最危险的过滤加：这一页每次改动都是"入了库但还没生效"，
后果又各不相同，靠用户记哪些要紧不可靠；而且**只给一部分加**正是上一版让人怀疑
"哪些改动真的保存了"的原因。逐条日志配置只有这一处入口（2026-09-20 起编辑弹窗不再编辑它们），
所以这套节奏不会在别处出现另一种语义。

**「保留档位」列的"继承"必须写清继承到哪一档**（2026-09-20，现场反馈"我怎么知道默认是什么呢"）：
有效档位的继承链是 **日志覆盖档位 → 服务默认档位（`log-retention-tier_id`）→ 平台默认档
（`is_default`）→ `std`**（见 §4.1）。下拉里那个 `null` 选项以前只写"继承服务默认"，用户无法知道
这条日志实际保留多久。现在三级都写出来：

| 情况 | 那个选项的文案 |
|---|---|
| 服务配了默认档位 | `继承服务默认（标准 30 天）`（名字来自 `log-config` 响应顶层的 `log_retention_tier`） |
| 服务没配默认档位 | `继承平台默认（热（7 天））`（名字来自档位列表里 `is_default` 的那一档） |
| 两者都没有 | `继承平台默认档位`（后端仍有 `std` 兜底） |

同一份文案用在三处：行内下拉、批量弹窗（"应用到 N 条"的说明）、改动确认框的 summary。
行上的 tooltip 还会给出**当前生效**的档位（有覆盖就是覆盖那一档，没覆盖就是上面这条链的结果，
后端已按同一条链算进 `tier_code`）。**逐条档位只有日志中心这一处入口**（2026-09-20 起服务编辑弹窗
不再编辑逐条日志配置，见下）。

**按行保存接口的字段必须与 `ServiceLogOverrideInput` 逐项对齐**（2026-09-19 现场）：
handler 的绑定结构漏了采集过滤两列 → 前端选完过滤后刷新"没保存"（请求带着值、绑定丢掉、
upsert 写成 NULL），而且此后改任何一列都会顺手清掉它。守卫：
`TestSaveApplicationServiceLogSettingPassesEveryOverrideColumn`（请求体里每一列都要走到 SQL）。

**逐条日志配置只有这一处入口**（2026-09-20 定）：逻辑服务编辑弹窗只保留服务级的两个日志默认值
（开启采集、默认保留档位）与一个指向本页的说明；「模板日志」表已删除——它的能力（勾选批量、
配置差异、下发、链路诊断）都在本页，两处并存只会让人不知道该信哪一处。
保存逻辑服务时**不提交 `log_settings`**（该字段是整表替换语义，提交空集合会删掉已有覆盖值；
字段缺省 = 后端整块跳过）。

**批量：先勾选，再对多行做同一件事**（2026-09-20 加，现场反馈"日志多了还是得一条条调"）。
「日志配置」的表格有勾选列（键 = `log_definition`，与 row-key 同源），勾选后表格上方出现批量栏：
**打开采集 / 停止采集 / 设置保留档位 / 设置采集过滤 / 格式认证 / 取消选择**，栏上写明"已选 N 条"。

| 批量动作 | 提交路径 | 语义 |
|---|---|---|
| 打开采集 | `POST …/log-config/settings/batch/` | `collection_enabled: null`（回到默认"采"），不确认——它不会让任何东西停止采集 |
| 停止采集 | 同上 | `collection_enabled: false`，走 `confirmConfigChange` 确认一次（与逐条同一句后果） |
| 设置保留档位 | 同上 | `retention_tier` = 选中档位或 null（继承服务默认）；弹窗里写清"写新流、旧流不迁移" |
| 设置采集过滤 | 同上 | 只写被选方向那一列（`collection_filter_rule` / `collection_exclude_filter_rule`），另一个方向原样带回；弹窗里写清"被滤掉的不会进 ES、事后补不回" |
| 格式认证 | 逐条调 `POST …/log-config/verify/` | 后端**没有**批量认证接口：每条认证都要去主机取样例，天然串行（见 §4.8） |

几条刻意的取舍：

- **每一行提交的仍是"该行覆盖值的全集"**（复用页面里那份 `overridePayload(record, patch)`），
  所以批量改档位不会顺手把别的行的过滤/开关清掉——单条接口 2026-09-19 踩过的坑在批量上同样成立。
- **整批一次事务写完**（`UpsertServiceLogOverrides`，服务端只做一次校验读）：要么都落库、要么都不落，
  不留改了一半的配置，也不像"前端循环发 N 个请求"那样对每个 id 重算一遍模板日志与认证指纹。
  与批量删除的差别在于这里**没有可独立的部分**——整批改的是同一列的同一个值；批量删除的逐条独立
  口径见 [CONVENTIONS.md](CONVENTIONS.md) §一。
- **逐条返回 `{id, ok, message}`**（与批量删除同一响应口径）：不属于本服务当前模板的日志记
  `ok=false` 并说明原因、其余照写，不整体失败（往别的服务的行上写覆盖值必须被拦住）；前端把失败的
  id 翻成**日志名**报出来，只回 id 用户不知道是哪一条。空 `items` / 超过 `MaxBatchLogOverrides`
  在服务层报 `ErrInvalid`。
- **改档位/过滤的弹窗本身就是确认点**（弹窗里写清后果 + 影响几条），不再叠一层确认框；
  只有"停止采集"这种直接按钮走确认框——与「切回该档位」同一个节奏。
- **勾选的作用域是当前服务**：换服务时清空、配置重载后剔除已不存在的行，否则批量请求会带着
  别的服务的日志定义（被后端逐条拒绝，看起来像"批量保存坏了"）。
- **格式认证是逐条的**：共享组件 `LogFormatVerifyDialog` 接受 `targets`（单条 = 一条），逐条串行、
  显示"正在认证第 x/y 条"、失败逐条列出（含主机离线这类硬失败，单条失败不中断整批），
  没挂解析规则的日志**明确跳过并说明跳过了几条**——静默少认证几条比拒绝更糟（用户以为都验过了）。
  部分不通过不影响其余条目的认证结果：认证是逐条的业务结果，没有"整批回滚"这回事。

#### 9.5.1 两条写路径，以及为什么不合并

日志覆盖值（采集开关 / 保留档位）有两条写入通道，**语义不同、各有明确适用范围**：

| 通道 | 语义 | 允许的调用方 |
|---|---|---|
| `SaveApplicationService` 的 `log_settings`（`PATCH /assets/application-services/:id/`） | **整表替换**（提交的集合即全量），但实现是**逐行 upsert（`UpsertServiceLogOverride`）+ 只删本次未提交的行**（`DeleteServiceLogSettingsByServiceAndDefinitions`）；与服务主体同一个事务 | **只允许“完整表单”**。2026-09-20 起前端**没有调用方**：逻辑服务编辑弹窗去掉了「模板日志」表（逐条配置统一走日志中心），保存时不再提交该字段（字段缺省 = 后端整块跳过、一行都不动）。字段与语义保留，供 API 侧“随表单一次性写覆盖值”用 |
| `POST /assets/application-services/:id/log-config/settings/`（`SaveServiceLogOverride`） | **按行 upsert**：只写这一条 `(服务 × 日志定义)` 的采集开关 + 档位，不影响同服务其他行，也不碰 `format_verified_*` | 按行操作（日志中心页的内联开关/档位） |
| `POST /assets/application-services/:id/log-config/settings/batch/`（`BatchSaveServiceLogOverrides`） | **按行 upsert 的批量版**：`items[]` 里每项与单条同一结构（该行覆盖值全集），一次校验读 + **一个事务**写完；不同服务模板的行逐条 `ok=false`、其余照写 | 日志中心页的批量操作（勾选多行一起改档位/过滤/开关，见 §9.5.0） |
| `POST /assets/application-services/:id/log-collection/`（`SetServiceLogCollection`） | **单列 UPDATE**：只写服务行的 `log_collection_enabled` + `update_time`，不碰服务其他属性 | 服务级采集总开关（日志中心页顶部那个开关） |

为什么不把编辑弹窗也切到按行写（统一成一条路）：那会丢掉“服务主体 + 日志设置在同一事务里提交”的
原子性——弹窗的保存是一个表单动作，改成按行就得发 N 个请求、部分失败无从回滚。而整表替换在
“调用方提交的是完整集合”这个前提下既正确又更简单。所以两者的边界靠**接口语义**划开：
整表替换只接受完整集合，按行接口在表达上根本做不到“删除其他行”。

**整表替换不能 delete-all 再重插（2026-09-20 修）**：认证结果（`format_verified_*`）只由格式认证
流程写，`DELETE WHERE service_id` 会把用户刚在编辑弹窗里认证通过的结果一起抹掉——现场表现就是
“服务树里认证通过、保存后又变成未认证，必须去日志中心再认证一次才生效”。现在的实现：已提交的行
走 `UpsertServiceLogOverride`（不碰认证列），只有本次未提交的行才删。回归测试
`TestSaveApplicationServiceKeepsFormatVerification` / `TestSaveApplicationServiceDeletesUnsubmittedLogSettings` 钉住这一点。

代价是“往整表替换接口发部分集合会静默删覆盖值”。这条靠三处兜住：弹窗是唯一调用方且它渲染全量；
按行接口只写覆盖列；以及守卫测试 `TestUpsertServiceLogOverrideKeepsFormatVerification` 钉住
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

#### 9.5.2 非服务节点 = 层级下钻视图（2026-09-20）

**问题**：服务树有六层（全部/项目/业务系统/环境/逻辑服务/部署实例），但只有服务与实例两层的 scope
带 `applicationServiceId`（`ServiceTree` 的 `scopeByKey`）。于是选中上面四层时，「日志查询」的面板
根本不挂载、「日志配置」整块内容在 `v-else` 里——两个 tab 都只剩一句"请在左侧选择逻辑服务或部署
实例"。而用户停在这一层时想问的是"**这一片里哪些服务的日志有问题**"。

**现在这一层显示"下一层的日志视角"**：指标条 + 下一层清单，点行下钻一级，到最底层（环境）的行
就是逻辑服务、点进去就是服务级界面（服务层现有内容一字未改）。层级映射与「存储水位」tab 的
`groupDimension` 完全一致：**全部→项目、项目→业务系统、业务系统→环境、环境→逻辑服务**。

**两个 tab 在非服务节点上回答的是两件不同的事**（2026-09-20 定，此前两边共用同一张表、只换一列，
结果看起来几乎一样——上层连那一列都没有数据）：

| tab | 回答的问题 | 指标条 | 列（叶子层=环境） | 列（上层） |
|---|---|---|---|---|
| 日志查询 | 这一片**哪个能查、有没有数据** | 下辖服务 / **可查**（采集开着）/ 采集关闭 / **无写入**（附"有写入 N 个"，仅环境层可算） | 逻辑服务 / 采集 / 最近写入（0 → 红「无写入」）/ 操作=查询日志 | 名称 / 下辖服务 / 可查 / 采集关闭 / 操作=下钻 |
| 日志配置 | 这一片**哪些服务有配置欠账** | 下辖服务 / 已开采集 / 采集关闭 / 未认证日志（附需复验）/ 待下发（附分母"已纳管 台·服务"） | 逻辑服务 / 采集 / 日志定义（未认证、需复验、逐条关闭、未挂规则）/ 默认档位 / 配置态 / 操作=日志配置 | 名称 / 下辖服务 / 采集关闭 / 未认证日志 / 未挂规则 / 配置态 / 操作=下钻 |

判据是"**这一层能不能回答**"：查询 tab 里不放未认证/待下发（那是配置欠账，与"查日志"无关），
配置 tab 里不放写入量（那是数据视角）。共用的只有"下辖服务数"——两边都要知道这一层有多大。

> **上层不显示"最近写入"是刻意的**：写入量接口按 (业务系统, 环境) 聚合（见 §9.6），
> 项目/业务系统层要给出这个数就得逐服务查 ES（几十次聚合）或把接口扩成索引通配 + 四段
> `multi_terms`；换来的只是列表上一个提示列，不值。所以：上层的「无写入」指标格与相关列
> **根本不出现**（不摆"-/本层不统计"这类占位——不统计就别占版面），只在底部留一句可操作的指引
>（"要看写入量请下钻到环境层"）。**"没取到数据"与"查询结果为 0"在数据层就分开**
>（`recentDocsKnown` / `hasRecentDocs`），永远不会把"没算出来"显示成"没有写入"。

| 区块 | 口径 |
|---|---|
| 指标条 | **合计 = 清单各行之和**（同一份 `util/logLevelOverview.js` 的纯函数算出来），所以"指标条说 3、点进去是 4"不会发生 |
| 中间层的行 | 下一层节点（项目/业务系统/环境）+ 下辖服务数 / 采集关闭数 / 日志条数（含未认证、未挂规则）/ 配置态。**每一层都按 scope 收窄**：项目节点只列**这个项目**的业务系统（业务系统行上的 `project` 字段是权威归属）、业务系统节点只列该系统的环境、环境节点只列该环境的服务；根节点上方的"项目/环境"多选同样生效（与存储水位 tab 的筛选口径一致）。漏了收窄就会出现"点某个项目却列出全部业务系统"（现场反馈）——这是这一类错误里最容易犯的一个 |
| 叶子层（环境）的行 | 就是逻辑服务本身；列见上表（查询 tab 与配置 tab 不同） |
| 空范围 | 按 tab 说清"这一层为什么没有可列的内容"（日志要按服务查 / 日志配置按服务生效），不留空表格 |
| 写操作 | **层级上只读**：写仍以逻辑服务为最小单位（重下发的最小完整单位是主机，见 §8.7）。层级视图只负责"看 + 下钻" |

**数据来源**（三份名单 + 一个汇总接口，页面级加载一次、两个 tab 共用；各拉一份必然出现两个 tab
数字不一致）：

- 成员与采集状态：`GET /assets/application-services/` 的服务行（`log_collection_enabled`、
  `log_retention_tier_id` 是权威值）+ 项目/业务系统名单，前端按 scope 本地收窄（与资产服务树页
  同一套做法）。
- 日志定义分桶与配置态：**`POST /assets/application-services/log-status-summary/`**，
  body `{"service_ids":[...]}`（读接口，入参是一批 id，所以按 POST 传列表，与 `log-targets/batch-jobs`
  同形），权限 `assets:applications:view`。data =
  `{items:[{service_id, logs, verified, needs_recheck, unverified, disabled_logs, no_rule,
  pending:{hosts,managed,unmanaged,synced,drift,never}|null}], totals:{...}, pending_error}`。
  - 日志分桶在 assets 侧由 **`ListServiceTemplateLogs`** 聚合（`format_state` 的指纹比对只有这一处
    实现，见 §4.8）——层级视图与服务页的"未认证"数字必须同源。
  - 配置态由**注入的评估器**给（`assets.ServiceLogPendingEvaluator`，实现是
    `logcollect/log_service_status.go`，注入点在 `router.NewWithGateway`，与 `LogFormatVerifier`/
    `LogGlobPreviewer` 同一套路，依赖方向不变）。判据与单服务版 `EvaluateServiceLogConfigStates`
    **共用同一个 `serviceConfigStatus`**，差别只在算的方式：把入参服务涉及的主机并起来、**每台主机
    只渲染一次**，再按服务子指纹分别比对（共享主机上改 A 不会把 B 带成待下发）。
  - `pending` 为 `null` / `pending_error` 非空 = "配置态**没算出来**"（没有启用的默认集群、
    评估器未注入）。界面这一列显示"-"并给出原因——**绝不显示成"都已同步"**。
  - 规模的可见上限 `MaxLogStatusSummaryServices = 200`：查询次数随服务数线性（每服务一次
    `ListServiceTemplateLogs` + 一次承载主机查询），外加渲染输入与默认集群的 3 条。要支撑上千服务
    时的扩容路径：把"承载主机 + 已下发子指纹"与日志定义读取都换成 `IN (sqlc.slice(ids))` 的批量版，
    整批只剩 4 条查询（见该常量上的注释）。
- 「最近写入」（只在叶子层）：`GET /monitor/elasticsearch-clusters/:id/log-service-usage/`
  （按 业务系统 × 环境 聚合各服务文档数，窗口 30 天，与列标题一致）。**上层不算这一列**：那要按服务
  逐个查 ES，代价不值，列里显示"-"（没有这项数据 ≠ 0 条）。

**边界**：不做"跨服务检索"。日志检索的索引里带服务段、接口硬性要求 `application_service_id`
（见 §9.7），放开它要同时改命中上限（2000）、facet 统计与排序语义，属于另一件事。所以查询 tab 在
层级上的职责是"把人送到具体服务"，面板里也明说这一点。

### 9.6 数据流清理（Go 版，2026-09-17）

逻辑服务维度的历史数据清理，让用户自助释放存储，不用 DBA 上 ES 手删。

- **按流清理（可选 `tier`，2026-09-19）**：`POST /monitor/log-datastreams/cleanup/` 的 body 支持
  可选 `tier`（档位编码），把范围从"该服务所有档位"收窄到**某一条流**。用于回收"改了档位、
  不打算再切回"的历史流——它本来会按自己档位的保留期由 ILM 到期删除，但保留期长/数据大的时候
  用户希望立刻释放。**档位编码必须先命中档位表**才能拼进索引模式（它直接进 ES 的索引模式，
  放行任意字符串等于把"删任意索引"的口子重新开出来）；语义仍是"删这条流里的文档"而不是
  "删掉 data stream 对象"——流本身留着（变空），与 `mode=all` 的既有行为一致，切回该档位继续写它。
- **入口只有一处：数据流列表**（日志中心 → 存储水位，**每条流一行的操作列**，按服务视图与全量视图都有）。
  2026-09-19 之前这个动作只在"选中服务 + 历史档位流"上给，而「日志查询」面板另有一个「清理日志数据」
  按钮——同一个动作两处入口、两套交互。现在：**查询面板的按钮已删除**，清理统一从数据流列表进，
  弹窗换成共享组件 `frontend/src/components/LogCleanupDialog.vue`（就是原来那套：保留 N 小时 /
  N 天 / 全部清空 + 不可恢复提示，并写明清理对象），两处作用域都由它承载：
  按服务（带 `tier` 收窄到那一条流）/ 按流名（未识别流，服务端三条校验，见下）。
  不放编辑弹窗（那里是编辑态，不适合破坏性操作）。
- 请求体只接受 `{service_id, mode, amount, tier?}`：`mode=all` 清空；`mode=hours|days` 保留最近
  N 小时/天（`amount` 上限 87600 小时 / 3650 天）。**不接受客户端传索引名**，流名由后端按
  service_id 解析维度码后拼 `<prefix>-<项目>-<业务>-<环境>-<服务>-*`（档位段 `*` 通配，
  覆盖换过档位的历史流）。
- 执行方式：对匹配的数据流发 `_delete_by_query?wait_for_completion=false&conflicts=proceed&refresh=false`，
  异步后台执行（同步删大量数据会超时），**保留 data stream 本身**（不删 backing index/流），
  避免 Filebeat 重建流时的空窗与 ILM 绑定问题；按 `@timestamp` 判定（与写入时区无关，存 UTC）。
- 失败语义：没有任何匹配的数据流（ES 404 `index_not_found`）视为已清理、返回 `matched=false`；
  其余 ES 报错原样返回 400。返回 `{stream_pattern, mode, amount, task, matched}`。
- **按流名兜底**（2026-09-19）：`POST /monitor/log-datastreams/cleanup-stream/`，
  服务维度的清理要求服务存在且档位段在档位表里，未识别流（旧服务下线留下的）与"档位段不在表里"
  的历史流都清不掉。这条路径只收**一条具体的数据流名**：带通配符 / 以 `.` 开头（后备索引、系统索引）/
  前缀之外的名字一律 400，且必须真的存在于集群（否则 404）；删除语义与上面完全一致。
  返回值里带 `recognized/service/tier` 作为事实（页面对能归属到服务的流优先走服务维度那条）。
- 破坏性操作，前端二次确认（不可恢复提示 + 范围选择）。
- **范围必填、无服务端默认**：`mode` 是必填参数，`all` 也必须由用户显式选择——计划中"清理不默认全清"
  的结论与现状一致（[LOG_COLLECTION_LIFECYCLE](../plans/LOG_COLLECTION_LIFECYCLE.md) §9 第 3 条）。
- **权限现状与目标**：当前该接口只继承 `monitor:view`（`router.go:366`），意味着只读权限即可删数据。
  计划按"破坏性动作整体拆权限"修正（数据清理 / 目标删除 / 停止服务 / 配置下发），见计划文档 §7、§9 第 4 条。

---

## 9.7 日志检索：过滤条件与关键词的两种模式

检索接口 `GET /monitor/elasticsearch-clusters/:id/log-search/`（`logcollect/elasticsearch.go` 的
`buildLogQuery`）在用户条件之外**固定**加五条过滤：`term service = <选中逻辑服务 code>`、
`project`、`business_system`、`environment`（四个维度）与时间范围（上限 30 天）——所以"查不到"
的第一顺位原因是**选错了服务**（或选的是项目/业务系统/环境节点），而不是数据不存在。
用完整维度而不是只按 `service`：服务编码的唯一域是 `(业务系统, 环境)`（见 assets 000044 迁移），
允许跨业务/环境重复，只按 code 过滤会把同名服务的日志串在一起。

白名单过滤（都是精确 `term`，走字段）：`instance` / `host_ip` / `log_name` / `log_path` /
`error_fingerprint` / `log_level`（逗号分隔的 `terms`）。后四项在面板上只通过**统计面板下钻**
写入（点分面值生成过滤 chip），没有常驻输入框。

**过滤条件的生命周期：切节点即清空（2026-09-21 修复）**。`LogQueryPanel.vue` 的 `filters` 是面板内
局部状态，切换左侧树节点（scope 变化）时 `watch(props.scope)` 必须把上一个服务的过滤条件全部清掉：
关键词、`logLevels`、以及统计下钻写入的 `hostIp` / `logName` / `logPath` / `errorFingerprint`
（`instance` 按新 scope 重设为部署实例名或空）。此前只重置了 `instance`，导致在服务 A 输入关键词、
选了级别、或从统计下钻后切到服务 B，这些条件原样带过去——现象是"B 查不到日志"，因为实际上是拿 A 的
条件在 B 的范围里搜。**scope 本身才是唯一的范围来源**，任何过滤条件都不应跨节点残留。
`deep: true, immediate: true` 的 watch 覆盖挂载与切节点两条路径；守卫见 `LogQueryPanel.spec.js`
的 "切换到另一个逻辑服务时清空关键词、级别与下钻过滤"。

**统计维度**（`GET .../log-facet-stats/`，后端白名单 `logFacetAllowedFields`）：
`error_fingerprint` / `log_level` / `instance` / `host_ip` / `log_name` / `log_path`。
`log_path` 即日志详情里显示的"日志路径"（Filebeat `log.file.path`）——配置里写通配路径时，
按它分组才能区分具体是哪个文件在产日志。

> **为什么日志路径的分组/过滤用运行时字段**：历史文档里存的 `log_path` 是配置里的**路径模式**
> （带 `*`），而具体文件只在 `log.file.path` 里；该字段在索引模板 `dynamic:false` 下没有 mapping、
> **不可聚合**。所以按 `log_path` 分组或过滤时，请求里带 `runtime_mappings` 定义一个从 `_source`
> 读 `log.file.path` 的 keyword 运行时字段（`log_file_path`），用它做 `terms`/`term`；样例与检索
> 结果的 `log_path` 也替换成真实文件。好处是**旧数据无需重建索引**（2026-09-20 现场：按日志路径
> 统计，列表里还是 `/home/esb/data/logs/*/log_error.log`）。

**关键词框的关键词有两个模式**（`keyword_mode`，2026-09-19 加）：

| 模式 | 行为 | 为什么 |
|---|---|---|
| `message`（默认） | `query_string` 且 `default_field=log_message`，并把输入里的 `:` **转义** | 面板上"关键词"的语义就是"搜日志正文"；不转义的话 `field:value` 会绕过 `default_field` 去查任意字段（`query_string` 的 `default_field` 只管没有前缀的 term） |
| `lucene` | 同一套 `query_string`，但**原样下发**：`host_ip:"192.168.201.209"`、`log_level:ERROR AND timeout` 都能用 | 框里写着 Lucene 就必须真的支持 Lucene——现场就是这么踩的：按提示写了字段过滤，被转义成在正文里找字面量 `host_ip:`，0 条、且界面上没有任何提示 |

两种模式**共同**的约定（有意为之）：裸词仍落在 `log_message`（`default_field` 不变，否则 `a AND b`
的语义会随"默认字段是整个文档"漂移）、`allow_leading_wildcard=false`（`*foo` 这类前置通配在 ES 侧是纯扫描，
代价与收益不成比例）、关键词截断 500 字符、`lenient=true`（字段名写错不报错，只是查不到）。
**模式只决定关键词怎么解析，不影响上面那两条固定过滤**：Lucene 模式也仍然只在你选中的服务范围内查。

面板上的切换（`正文 / Lucene`）同时改 placeholder 与说明文字，切模式本身不触发查询；
没有关键词时**不发送** `keyword_mode`（后端默认即正文模式）。

**时间显示到毫秒**：ES 的 `@timestamp` 映射为 `date`（毫秒精度，`log_management.go`），日志查询
表格与详情用 `formatTimeWithTimezone(..., 'YYYY-MM-DD HH:mm:ss.SSS')` 显示（`LogQueryPanel.vue` 的
`formatLogTime`）——同一秒内多条记录靠它区分先后；时间范围标签与趋势图轴仍用秒级格式（选的是
分钟级范围，显示毫秒只会变吵）。毫秒占位符 `SSS` 由 `fronted/src/util/timezone.js` 支持。

---

## 9.8 强制刷新索引（`refresh_interval=10s` 的按需补偿）

索引模板把 `index.refresh_interval` 设为 **10s**（见 §4.2，`logcollect/log_management.go` 的
`buildIndexTemplateBody`），Elasticsearch 默认是 1s——这是用搜索实时性换写入吞吐的取舍。
代价是**文档已写进 ES、但最多 10s 后才可被检索**，容易被误判成"没采集到"。

接口 `POST /monitor/elasticsearch-clusters/:id/log-refresh/`（`logcollect/log_refresh.go` 的
`ElasticsearchLogRefresh`）按需对该逻辑服务**采集中的 data stream** 做一次 `_refresh`，
让已写入的文档立即进入可搜索 segment：

- 入参只有 `application_service_id`（**不接受客户端传索引名**，与清理路径同一防口子考虑）；
- 由 service_id 解析维度码（`GetApplicationServiceStreamDims`）与生效档位集合
  （`ListServiceActiveStreamTiers`）后拼精确流名 `<prefix>-<项目>-<业务系统>-<环境>-<服务>-<档位>`；
  流名不含 `log_name`，同一服务的多条日志若档位相同则共用同一条流，故按档位去重即得全部采集流；
- **只刷采集中的档位，不含历史档位流**（历史流已停止写入，刷它没有意义）；
- 没有采集中的流时返回 `{streams:[], count:0, message:...}`，不发 ES 请求；
- 集群取请求 URL 上的集群（与日志查询面板选中的是同一个），执行
  `POST /<逗号分隔的流名>/_refresh?ignore_unavailable=true&expand_wildcards=open`；
- 失败语义：ES 报错转 502；服务不存在转 404。

前端入口是日志查询面板顶部的**「刷新索引并查询」**按钮（`LogQueryPanel.vue`）：先刷新、再自动重查一次。
它是按需操作，不改变写入侧行为，也不替代 ES 自身的 10s 周期。**只能解决"已写入未刷新"**——
Filebeat 还没发出的数据（应用空闲、被采集过滤排除、主机采集未生效）刷多少次都不会出现，
那属于采集链路问题（见 §9.5）。

---

## 10. 代码组织

采集与存储两条链路的实现在 **`autoadmin/internal/logcollect/`**，不在 `internal/monitor/`。
拆分的依据是职责边界：日志采集/存储自成一套对象（Filebeat 纳管目标、ES 集群、日志定义、数据流），
与监控域的 exporter 目标、告警/通知、软件包仓库没有共享状态。

| 文件 | 职责 |
|---|---|
| `log_config_render.go` | 片段渲染与指纹（**纯函数、不访问数据库**，与 SQL 解耦便于单测） |
| `log_target_actions.go` | 纳管目标的安装/卸载、启停、配置下发、批量操作 |
| `log_service_chain.go` | 按服务的采集链路诊断（"为什么没日志"，见 §9.5；判定复用体检与下发状态） |
| `log_collection_filter.go` | 采集过滤的两层三态解析与降级（见 §6.1；渲染侧只认最终生效的两条正则） |
| `log_config_consistency.go` | 保存逻辑服务时的配置自洽性校验（硬问题拦保存、软告警放行；见 §8.4） |
| `log_health.go` | 链路体检六层对账 |
| `log_management.go` | 索引模板、ILM 策略、bootstrap |
| `log_datastream_cleanup.go` | 数据流按服务/日志文件/时间窗清理 |
| `datastream_status.go` | 存储水位：流级运行态与维度树 |
| `regex_pattern.go` | 规则正则的 RE2 判定与"照着改"提示（保存校验、认证、渲染三处共用，见 §2） |
| `pipeline_compat.go` | 处理器参数的引擎方言别名与"照着改"提示（见 §5.2.1） |
| `elasticsearch.go` | Elasticsearch 客户端、日志检索、聚合 |
| `log_refresh.go` | 按需强制刷新逻辑服务采集中的 data stream（见 §9.8） |
| `elasticsearch_config.go` / `elasticsearch_pipeline.go` | 集群配置 CRUD、pipeline 模拟与 `missing_fields` / `schema_violations` 判定 |
| `sample_event.go` | 样例事件的 Filebeat 真实载荷（`filebeatEventFields`）：试算、格式认证、链路试跑三处共用，见 §5.2 |
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
