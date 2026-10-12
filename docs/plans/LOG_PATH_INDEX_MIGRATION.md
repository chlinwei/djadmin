# 真实日志路径索引迁移方案

## 目标与执行边界

查询、筛选候选和路径统计统一直接使用 `log_path` keyword 索引。真实值来自 Filebeat
`log.file.path`，不再在每次查询时用运行时字段扫描 `_source`。复用已有字段，不增加第二套路径索引。

这是待确认的操作方案，**不是自动迁移**。未确认集群、数据流、时间范围、快照与维护窗口前，
不得执行下面的写入请求。示例使用 Elasticsearch Console 语法，所有尖括号占位符必须替换；
认证只使用现有受控凭据，不写入文档或脚本。

本迁移不改 MySQL schema，不改变日志正文、时间戳、服务维度或保留档位。
路径缺失/非字符串/空串的文档移除 `log_path`，而不是保留配置模式作为真实路径。
ES 的 final pipeline 仍会执行既有必备字段检查；必须核验其模式和附加标记行为。

## 1. 只读预检与确认

1. 明确目标集群及具体服务数据流，包含历史档位。由服务的四元组解析数据流后，记录
   **确切数据流名及 backing index 列表**；不得直接对全局 `autoadmin-*` 提交回填。
2. 核验目标 backing indices 的 `log_path` 都是可聚合的 `keyword`：

   ```http
   GET <已确认的索引列表>/_field_caps?fields=log_path
   GET <已确认的索引列表>/_settings?filter_path=*.settings.index.final_pipeline,*.settings.index.blocks
   GET _ingest/pipeline/<前缀>-mapping-guard
   ```

   类型冲突、只读/关闭索引、无法解析的 final pipeline 都是阻断项，不忽略、不自动改设置。
   **若必备字段 guard 为 `drop` 模式，停止迁移**：update-by-query 会执行 final pipeline，
   可能丢弃不符合当前规则的历史文档。须另行确认无损策略，不能擅自切换 guard 模式。
3. 在固定时间段抽样，比较 `_source.log_path` 与 `_source.log.file.path`：

   ```http
   POST <已确认的数据流列表>/_search
   {
     "size": 100,
     "sort": [{"@timestamp": "desc"}],
     "_source": ["@timestamp", "log_path", "log.file.path"],
     "query": {"range": {"@timestamp": {
       "gte": "<确认的 UTC 开始时间>",
       "lte": "<确认的 UTC 结束时间>"
     }}}
   }
   ```

4. 记录文档数、search 队列、堆使用率、磁盘余量和索引大小。确认现有受控快照仓库，
   对目标 backing indices 建立可恢复快照并确认成功；未经确认不得建立外部备份目的地。
5. 首轮仅选一个已确认服务的固定 12 小时段；通过后按服务和自然日分批处理全部需保留的历史日志。
   在同一索引列表和时间范围内比较前后数据，避免把 ILM 到期删除或持续采集误判为迁移丢数。

## 2. 新写入口径与维护窗口

1. 部署新版后端时关闭日志查询入口/进入维护窗口，避免历史回填未完成时暴露错误的路径候选。
   保持采集运行；新数据须先进入归一化后的 pipeline，避免回填期间继续写入路径模式。
2. 在日志存储管理保存目标集群现有配置，触发仓库既有存储同步流程；这会同步该集群的
   模板、档位策略和平台 final pipeline，**影响该前缀下的新写入**，须纳入执行确认范围。
   等待 `storage_sync_state` 成功并读取 ES pipeline 核验，不以“保存请求成功”代替同步完成。
3. 确认 `log_path` 保持 keyword，final pipeline 包含最后的 `canonical_log_path` 处理器：
   以 `log.file.path` 覆盖 `log_path`；没有有效真实路径则移除这个可选字段。
   模板不改变已建索引的 mapping；当前目标字段已经是 keyword，不需重建索引或新增 mapping。
4. 对新写入的样例验证真实路径、标准必备字段与正文均正确。模板/pipeline 同步失败则停止，
   不启动历史回填。

## 3. 限速、异步历史回填（写入，执行前需再次确认）

只使用第 1 步确认的确切数据流和固定 UTC 时间段。下面请求是有写入副作用的，
**本方案准备阶段不执行**。

```http
POST <已确认的数据流列表>/_update_by_query?wait_for_completion=false&requests_per_second=100&scroll_size=100&slices=1&conflicts=proceed&refresh=false
{
  "query": {
    "range": {
      "@timestamp": {
        "gte": "<确认的 UTC 开始时间>",
        "lte": "<确认的 UTC 结束时间>"
      }
    }
  },
  "script": {
    "lang": "painless",
    "source": "def path = null; if (ctx._source.log instanceof Map && ctx._source.log.file instanceof Map) { path = ctx._source.log.file.path; } if (path instanceof String && path.length() > 0) { if (path.equals(ctx._source.log_path)) { ctx.op = 'noop'; } else { ctx._source.log_path = path; } } else if (ctx._source.containsKey('log_path')) { ctx._source.remove('log_path'); } else { ctx.op = 'noop'; }"
  }
}
```

- ES 按 `target_time = batch_size / requests_per_second` 节流。`scroll_size=100`、单 slice、
  `requests_per_second=100` 时，每批目标周期为 1 秒，初始处理速率约 100 文档/秒；
  实际慢于目标时不会额外加速。按负载调整，不能一开始无限速、多 slice 并发。
- 路径已正确的文档为 noop，重复提交幂等；只修改路径字段的迁移脚本不修改正文、时间和维度。
  final pipeline 的既有 app_fields 补齐/违规标记可能同时生效，须在试迁移时验收。
- `conflicts=proceed` 只跳过并发版本冲突，不代表全部成功；记录冲突数并重新处理固定范围，
  直到冲突清零或逐项确认。不得忽略 `failures`、任务 `error` 或异常 `deleted`。
- 保存返回的 task ID；不要重复提交相同范围制造并发任务。回填创建的是异步服务端任务，
  关闭 Console 不会自动终止它。

## 4. 监控、暂停与验收

```http
GET _tasks/<task_id>
GET _nodes/stats/jvm,thread_pool,indices?filter_path=nodes.*.jvm.mem.heap_used_percent,nodes.*.thread_pool.search,nodes.*.indices.search

POST _update_by_query/<task_id>/_rethrottle?requests_per_second=20
POST _tasks/<task_id>/_cancel
```

负载异常、搜索队列积压、失败或删除数非零时，降低速率或取消这个**明确 task ID**。
取消不是回滚，已经成功更新的文档仍保留新路径。任务完成后确认 `failures=[]`、
`deleted=0`，记录 `updated/noops/version_conflicts`；仅对已确认索引显式 refresh 后验收：

1. 抽样中每条有真实路径的文档满足 `log_path == log.file.path`；没有有效真实路径则没有 `log_path`。
   如需全量一致性核验，采用受控分页/离线比对，不能把抽样成功等同于全部历史数据已完成。
2. 在同一服务范围、同一 12 小时段执行日志明细、路径候选、路径精确过滤和完整路径统计，
   确认全部结果与实际文件一致，`timed_out=false`、无失败分片、客户端未超时。
3. 比较固定历史范围的文档数、正文与标准字段，排除并发采集、ILM 删除和版本冲突后不得丢失数据。
4. 每批验收后再处理下一批。全部需保留范围完成、检查新写入持续正确后才开放新版查询。

## 5. 回退

停止后续回填并记录任务结果；必要时回退 API 查询版本，但不恢复运行时脚本作为新版兼容分支。
数据回退需要已确认快照和维护窗口：先恢复到隔离索引进行比对，再制定确切恢复范围，
不得直接覆盖仍在接收新日志的数据流，避免丢失快照之后的新写入。
旧版查询读取 `log.file.path`，规范化后的顶层字段不会改变这个原始来源。
