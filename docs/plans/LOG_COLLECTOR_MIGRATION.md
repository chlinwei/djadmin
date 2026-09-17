# 日志采集迁移方案（Fluent Bit → Filebeat，OpenSearch → Elasticsearch）

> 状态：进行中（Phase 1 已落地）。起因：Fluent Bit 离线安装要处理 rpm 依赖冲突（libyaml/libpq）、
> 每个软件包维护 ansible playbook，安装链路太重；需要一套"单二进制、装起来不折腾"的采集器；
> 同时搜索后端从 OpenSearch 换成 Elasticsearch。
> 现有最终逻辑（Fluent Bit + OpenSearch 版）见 [LOG_COLLECTION_ARCHITECTURE.md](../architecture/LOG_COLLECTION_ARCHITECTURE.md)。

## 一、为什么换成 Filebeat

| 维度 | Fluent Bit（现状） | Filebeat |
|---|---|---|
| 安装 | rpm/deb 需自己凑依赖，易冲突；tar.gz 官方不出 Linux 二进制 | 单静态 Go 二进制，官方 tar.gz（Linux x86_64/arm64）与 rpm/deb 均自包含，无系统库依赖 |
| 配置 | 主配置 + inputs.d/outputs.d/parsers 多文件，多行要专门 parsers 文件 | 一份 `filebeat.yml`，inputs 可拆 `inputs.d/*.yml` 配 `reload.enabled` 热加载 |
| 多行 | `[MULTILINE_PARSER]` 强校验，位置受限 | filestream `parsers: multiline` 现成 |
| 每流索引 | OUTPUT 的 Index | filestream input 支持 `index` 覆盖 / `indices` 条件 |
| OpenSearch | es output | `output.elasticsearch` 与 OpenSearch 兼容（IK/ingest pipeline 同样可用） |
| 卸载/升级 | playbook 各自维护 | 覆盖二进制 + 重启，无依赖链 |

## 二、候选方案对比

1. **Filebeat（推荐）**：安装即痛点，官方单二进制；多行、每输入索引、OpenSearch ingest pipeline 全部现成；后端只需把"片段渲染"换成"filebeat.yml 渲染"，数据流索引/ISM 不动。
2. **dj-agent 自采集**：彻底去掉第三方采集器，agent 内置 tailer + OpenSearch bulk。零安装，但 offset/多行/背压/鉴权都要自研，可靠性与工作量都大，作为长期演进方向。
3. **Vector / OTel Collector**：同为单二进制、YAML 配置；Vector 的 VRL 处理更强，OTel 生态更标准。工作量与 Filebeat 相近，选型偏个人偏好；OTel 的 filelog 多行与 OpenSearch 输出链相对绕。

结论：先按 **Filebeat** 落地，agent 自采集作为后续可选优化。

## 三、Filebeat 方案设计

### 3.1 主机侧布局

```
/opt/filebeat/filebeat                 单二进制
/etc/filebeat/filebeat.yml             主配置（output + 全局）
/etc/filebeat/inputs.d/*.yml           按 服务×日志定义 拆的 inputs（reload.enabled 热加载）
/var/lib/filebeat/                      registry（offset）与 data，必须持久化
```

### 3.2 渲染改造（`internal/monitor/log_config_render.go`）

- 输入/聚合维度不变（服务×日志定义×实例、路径宏、档位、处理规则、多行）。
- 输出从 `renderedHostLogConfig.fragments` 改为一组 Filebeat 文件：
  - 每个 `服务×日志定义` 生成一个 `inputs.d/<app>__<svc>__<log>.yml`，一个 filestream input 含各实例 `paths`；
  - `fields`（project/business_system/environment/service/instance/application/log_name/host_ip）+ `fields_under_root: true`；
  - `index` = `LogDataStreamName(...)`（沿用现有数据流命名与保留档位）；
  - `multiline`（处理规则开启时）+ `pipeline`（处理规则 name）；
  - `output.elasticsearch`（hosts/认证/TLS）写在 `filebeat.yml` 主配置。
- 指纹（`config_fingerprint`）沿用：文件集合内容的 sha256。
- 渲染仍是纯函数，便于单测；新增 `renderHostLogConfigFilebeat`，旧 Fluent Bit renderer 可保留一个版本以便回滚/灰度。

### 3.3 agent 改造（`dj_agent/internal/executor/builtin_actions.go`）

- 新增 `apply_filebeat_config`：写 `inputs.d/*.yml`（全量托管、删残留）、写 `filebeat.yml`；
  先 `filebeat test config` 校验，再 reload（`reload.enabled` 时无需重启）或 `systemctl restart filebeat`。
- 新增/替换 `configure_filebeat_output`：写 OpenSearch 地址与账号（env 文件 + drop-in，参照现有 fluent-bit 做法）。
- 运行态探测：`systemctl is-active filebeat` 退出码语义与现有 fluent-bit 一致。
- 保留 Fluent Bit 动作一个版本，按主机 `collector_type` 灰度切换。

### 3.4 安装/下发

- 软件仓库新增 `package_type=filebeat`：单二进制 tar.gz（`filebeat-<ver>-linux-x86_64.tar.gz`）或官方 rpm/deb。
- 安装 playbook 极简：解压到 `/opt/filebeat`、写 systemd unit、写初始 `filebeat.yml`、启动；无依赖链。
- 卸载：停服、删 `/opt/filebeat` 与 unit，保留 registry 可选。
- 安装历史/失败文案沿用 `monitor_target_install_history` 与 `ansibleResultMessage`（stderr 优先）。

### 3.5 不改动

- 索引/数据流命名、保留档位、ISM/rollover。
- OpenSearch ingest pipeline（处理规则）与解析调试链路。
- 服务树配置 UI、指纹跳过、日志查询/存储水位等读取侧。

## 四、实施步骤（建议顺序）

1. 采集器抽象：抽出 `collector` 概念（字段 + 渲染接口），Fluent Bit 为现有实现，新增 Filebeat 实现。
2. Filebeat renderer + 单测（对标现有 render 单测的覆盖）。
3. agent `apply_filebeat_config` / `configure_filebeat_output` + 部署脚本；本机验证 `filebeat test config` 与 reload。
4. 软件仓库 `package_type=filebeat` 与极简 playbook；安装/卸载跑通一台。
5. 前端：日志采集配置页增加采集器类型/相关字段（最小化，先不暴露复杂项）。
6. 灰度：按主机切换 collector，核对索引/字段/多行/offset 延续性；稳定后默认 Filebeat 并冻结 Fluent Bit 路径。
7. 清理：确认无 Fluent Bit 主机后，删除其渲染与 agent 动作、文档归档。

## 五、风险

- **offset 无法从 Fluent Bit 平滑迁移**：切换采集器会从文件末尾重读或重复/漏读，需选定切换点（停采→换→续采）并接受一段重读。
- **多行/解析语义差异**：Fluent Bit multiline 与 Filebeat multiline 正则语义（negate/match/after）需逐规则比对，处理规则沿用 OpenSearch pipeline 的部分不受影响。
- **OpenSearch 兼容**：`output.elasticsearch` 对 OpenSearch 基本可用，但版本/安全插件（如需要 `compatibility` 模式）需实测。
- **灰度成本**：两套渲染与 agent 动作并存一段时间，需要 `collector_type` 明确到主机级。

## 六、OpenSearch → Elasticsearch

### 6.1 差异盘点（按现网调用点）

现网只有 3 处 OpenSearch 专有 API，其余全部与 ES 兼容：

| 现网 OpenSearch | 位置 | Elasticsearch 对应 |
|---|---|---|
| `/_plugins/_ism/policies/<name>` GET/PUT | `log_management.go:168/169/175`、`log_health.go:246` | `_ilm/policy/<name>` |
| `/_plugins/_ism/explain/<prefix>-*` | `datastream_status.go:183` | `_ilm/explain`（或直接查 `_cat/indices`） |
| mapping `flat_object` | `log_management.go:74` | `flattened` |

兼容、无需改协议：`/`、`/_cluster/health`、`/_search`、`_index_template`（composable）、`_ingest/pipeline`、`_cat/indices`、`_cat/allocation`、`_ingest/pipeline/_simulate`。

### 6.2 要改的语义

- **ISM → ILM**：`buildISMPolicyBody`（`log_management.go:90-118`）改写成 ILM `policy.phases.hot.actions.rollover` /
  `delete`；`ism_template`（113-116）在 ILM 无对应，改为在 `_index_template` 的
  `template.settings.index.lifecycle.name` 绑定策略；rollover 条件 `min_primary_shard_size` → `max_primary_shard_size`/
  `max_age`（ILM 的 rollover 语义是"达到条件即滚动"）。
- **策略签名/健康检查**：`ismPolicySignature`（`log_health.go:194-233`）、`checkLogISMPolicies`（236-268）改为读
  `_ilm/policy`，比较 `version`/`modified_date` 或策略体。
- **错误前缀**：`isOpenSearchNotFound` / 错误文案里的 `opensearch` 前缀统一改 `elasticsearch`。

### 6.3 命名与 DB

- 表 `monitor_opensearch_cluster` → `monitor_elasticsearch_cluster`（两方言 schema + 迁移 + sqlc 查询名
  `*OpenSearch*` → `*Elasticsearch*` + `make derive/generate/facade`）。
- Go 符号：`openSearchCluster`/`openSearchRequest*`/`OpenSearch*` handler 全部更名；`handler.secrets` 加解密不变。
- API 路由：`/monitor/opensearch-clusters/*` → `/monitor/elasticsearch-clusters/*`（前端 `monitor.js` 同步）。
- 前端文案：`log-storage`、`log-storage-overview`、`log-parsers`、`LogQueryPanel` 里的 "OpenSearch" 字样与
  `ISM`/`Ingest` 提示。
- **无 seed 数据**：默认值都硬编码在代码里（`index_prefix "autoadmin"`、档位 `std`、rollover 默认值），无迁移数据要改。

## 七、实施状态

- [x] **Phase 1（已落地）**：Filebeat 配置渲染 `renderHostLogConfigFilebeat`（`log_config_render.go`）+
  单测，与 Fluent Bit 渲染共用维度查询，产物为 `/etc/filebeat/inputs.d/*.yml`（每实例一个 filestream input，
  含 dimension fields / index 数据流名 / pipeline / multiline）。
- [x] **Phase 2a（已落地）**：ES 8 兼容改造，已用真实集群 `https://192.168.201.123:9200`（ES 8.13，default）验证：
  - `flat_object` → `flattened`；
  - 保留策略由 ISM 改为 ILM：`PUT /_ilm/policy/<name>`（`phases.hot.actions.rollover.max_primary_shard_size` + `max_age`、`phases.delete.min_age`）；
  - 新增每档位自包含索引模板 `PUT /_index_template/<prefix>-<tier>-template`（`index_patterns: <prefix>-*-<tier>`、`priority 200`、`data_stream:{}`、完整 settings/mappings + `index.lifecycle.name`）——实测 ES 8.13 高优先级模板不与基础模板合并 mappings，必须自包含；
  - 基础模板 `<prefix>-template` 不设 priority（默认 0），但 bootstrap 在写入前用 `_cat/templates` 预检：存在同 priority 且 pattern 重叠的模板（如 `test`，patterns=`logs*`（历史遗留），priority 0）时**直接报错并列出冲突模板**，不自动调 priority 绕过；
  - 健康检查读 `GET /_ilm/policy/<name>` 并按 ILM 归一签名比对（忽略 `hot.min_age=0ms`、`delete_searchable_snapshot` 等 ES 默认值）；
  - 存储水位 explain 由 `_plugins/_ism/explain` 改为 `GET /<prefix>-*/_ilm/explain`（取 `indices.<idx>.phase`）；
  - HTTP 层错误前缀 `opensearch` → `elasticsearch`，连接测试的 `distribution` 回落 `build_flavor`。
- [x] **Phase 2b（已落地）**：命名与 DB 全量更名：
  - 迁移 `db/migrations/{mysql,postgres}/000025_elasticsearch_cluster_rename.{up,down}.sql` 重命名表（PG 同步重命名唯一约束）；
  - `db/schema/*` 表名、`db/queries/mysql/monitor.sql` 查询名/表名，经 `make derive/generate/facade` 同步生成物；
  - Go 符号/文件（`elasticsearch*.go`、`Elasticsearch*` 函数与 handler）、路由 `/monitor/elasticsearch-clusters/*`；
  - 前端 `api/monitor.js` 与各视图函数名/文案、`ism_state`→`ilm_state`、`ISM`→`ILM`；
  - 保留：agent 侧的 `configure_fluent_bit_opensearch` 动作名（Fluent Bit 弃用路径，随 Phase 6 一并清理）。
- [x] **Phase 3（已落地）**：agent 新增 `configure_filebeat_output`（写 `filebeat.yml` 的
  output.elasticsearch + `filebeat.config.inputs` 托管段，`filebeat test config` 后 restart）与
  `apply_filebeat_config`（写 `inputs.d/*.yml`、清理残留、test config 后 restart）；
  删除 `reload_fluent_bit`/`configure_fluent_bit_opensearch`/`apply_fluent_bit_config`。
- [x] **Phase 4（已落地）**：软件仓库只允许 `package_type=exporter|filebeat`；Filebeat 只认
  `package_format=tar.gz`、`platform_family=any`、`platform_major=''`，安装包按 CPU 架构
  （amd64/arm64，自动补采）匹配；存储目录 `monitor_packages/filebeat/...`。
  迁移 `000026_drop_fluent_bit_packages` 删除历史 Fluent Bit 软件包记录。
- [x] **Phase 5（已落地）**：后端/前端 Fluent Bit、OpenSearch 文案与符号全部改为 Filebeat、
  Elasticsearch（`fluent_bit`→`filebeat`、`fluent_bit_managed`→`filebeat_managed`、
  渲染产物 `/etc/filebeat/inputs.d/*.yml`）。
- [ ] **Phase 6**：真机灰度验证（安装 tar.gz、下发 inputs、确认已写入 ES 数据流），
  稳定后清理归档文档中的历史 Fluent Bit/OpenSearch 描述。
