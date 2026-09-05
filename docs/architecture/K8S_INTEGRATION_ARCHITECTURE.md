# K8s 对接架构方案文档

本文档描述 autoadmin 对接 Kubernetes 的整体方案，覆盖资产纳管、告警统一、日志统一和高频操作四部分。当前为**方案设计阶段，尚未实现**，实现后本文档需同步更新为最终逻辑。

---

## 1. 定位与边界

autoadmin 对 K8s 的定位是**统一运维平台的资源域**，不是 K8s 管理平台。

### 1.1 做什么

- 集群/节点/工作负载/Pod 的资产纳管与状态展示
- K8s 告警汇入平台统一告警链路（AlertRoute / AlertMedia）
- 容器日志汇入平台统一日志链路（Fluent Bit + OpenSearch）
- 少量高频操作（重启 Deployment、扩缩容、删 Pod），走平台权限、审批、审计体系

### 1.2 不做什么

以下属于 Rancher / 云控制台的能力边界，本平台不做：

- 集群创建、升级、节点池管理
- Helm 应用商店、应用一键部署
- YAML 编辑器、CRD 全资源管理
- HPA/VPA 配置管理
- 网络/存储/PSP 精细治理

### 1.3 功能取舍判断标准

每个候选功能问一句：**"这件事离开 Rancher 能不能形成闭环？"**

- 能（如带审批的重启、统一告警路由、资产关联业务系统）→ 属于本平台
- 不能（如 Helm 部署、建集群）→ 留给 Rancher / 云控制台

云上 K8s（ACK/EKS/TKE）优先使用云控制台，本方案主要覆盖**自建集群**。

---

## 2. 总体架构

每个集群部署三件套，全部复用平台现有体系：

```
                        ┌────────────────────── autoadmin ──────────────────────┐
                        │  CMDB / 调度器(告警评估) / AlertRoute-Media / 日志洞察   │
                        └────────────────────────────────────────────────────────┘
   控制面 (gRPC, 复用现有通道)      数据面(日志)             指标面
┌──────────────────────┐   ┌──────────────────┐   ┌──────────────────────────┐
│ dj-agent K8s 模式     │   │ Fluent Bit       │   │ Prometheus (集群内)       │
│ Deployment, 单副本    │   │ DaemonSet        │   │  ├ http_sd  (主机, 现有)  │
│ ├ watch 资产/事件上报  │   │ tail 容器日志     │   │  └ kubernetes_sd (K8s)   │
│ └ 写 Fluent Bit 配置   │   │  → OpenSearch    │   └──────────────────────────┘
│   (ConfigMap)         │   └──────────────────┘
└──────────────────────┘
```

### 2.1 核心设计决策

| 决策 | 结论 | 理由 |
|---|---|---|
| Agent 职责 | 只做控制面：配置下发、资产/事件上报、操作执行。**不采集日志、不做指标抓取** | 与虚拟机侧"Agent 管配置、Fluent Bit 管数据"分工对称；避免每节点双采集器 |
| 告警大脑 | autoadmin 调度器，**不引入 Alertmanager** | 平台现有模式即调度器轮询评估 → AlertRoute → AlertMedia；规则、静默、收敛、通知只有一个大脑 |
| Prometheus 角色 | 只做采集与存储（指标数据库） | kubernetes_sd 负责目标发现；`/api/v1/query` 供调度器评估告警、`/query_range` 供前端画图 |
| 资产全集来源 | dj-agent 上报为准 | Prometheus targets 只是采集视角（随注解增减），不能反推资产清单 |
| 多集群模型 | 每集群一套三件套，autoadmin 侧多数据源配置 | 天然分片，无跨集群状态 |
| 日志采集器 | 沿用 Fluent Bit（集群内 DaemonSet 形态） | 与虚拟机侧同款，索引/字段/解析规则全复用 |

---

## 3. 资产纳管

### 3.1 资产模型

- 新增**集群**资源类型，挂入现有 CMDB 体系，与主机平级：
  - 名称、环境（prod/test）、云厂商或自建、K8s 版本、接入状态
- 集群详情页：节点、工作负载（Deployment/StatefulSet/DaemonSet）、Pod、事件
- Node 与主机资产打通：一台机器既可以是 Agent 管的虚拟机，也是某集群的 node

### 3.2 业务标签契约

K8s 资源与 CMDB 业务系统的映射靠 **label 约定**：

```yaml
metadata:
  labels:
    business_system: tib      # 必须与 CMDB 业务系统 code 一致
    environment: prod
```

- namespace 与工作负载均要求打标；autoadmin 定期比对 label 与 CMDB 映射，不一致产生告警
- 该 label 同时是告警路由（见 §4）和日志索引归属（见 §5）的依据，是全链路统一的关键契约

### 3.3 数据通道

dj-agent（K8s 模式，Deployment 形态驻集群内，in-cluster ServiceAccount）：

- watch Node / Deployment / Pod / Events，状态变化沿现有 gRPC 通道上报
- ServiceAccount 权限从只读起步（get/list/watch），操作权限按需逐项放开

前端结构：

```
资产管理
 ├─ 主机（现有）
 └─ 集群（新增）
     └─ 集群详情
         ├─ 节点 / 工作负载 / Pod   ← Agent 上报 + 指标图（Prometheus）
         ├─ 采集目标               ← Prometheus /api/v1/targets
         └─ 事件                   ← K8s events，与告警页打通
```

注意语义区分：**采集目标 tab 是 Prometheus 的抓取视角**，与资产清单是两个维度；未打 scrape 注解的 Pod 不在采集目标中，但资产清单必须有。

---

## 4. 告警统一

### 4.1 两类告警、两条通道

| 类型 | 示例 | 来源 | 评估方 |
|---|---|---|---|
| 事件类 | CrashLoopBackOff、Node NotReady、调度失败、ImagePullBackOff | dj-agent watch events | Agent 侧直接产生，gRPC 上报 |
| 指标类 | CPU/内存/网络、副本数不足、restart 突增 | Prometheus（kubernetes_sd 发现目标） | autoadmin 调度器查 `/api/v1/query` |

### 4.2 调度器扩展

现有调度器（见 docs/ops/SCHEDULER_README.md）增加一类 **Prometheus 数据源**告警规则：

- 告警规则同一张表，增加 `datasource` 字段（agent / prometheus）
- Prometheus 数据源规则评估时调 `GET /api/v1/query`，多集群 = 多 Prometheus 地址，查询带 `cluster` 标签
- 阈值、持续时间、恢复通知等评估语义与主机告警完全复用

### 4.3 收敛与路由

- Alert 模型增加通用**关联资产**字段：主机告警关联主机实例，K8s 告警关联集群+namespace+workload
- 收敛按 **workload 维度**（Deployment/StatefulSet）聚合，不按 Pod 维度，避免 Pod 动态生灭造成告警风暴
- 通知路由复用 AlertRoute / AlertMedia，按 `business_system` / `environment` 标签匹配，K8s 与主机告警同一条通知链路

---

## 5. 日志统一

完全复用现有 Fluent Bit + OpenSearch 链路（见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md），仅采集器位置从"主机上"变为"集群里"。

### 5.1 数据流

```
虚拟机（现有）：
  主机上的 Fluent Bit ──tail 文件──▶ OpenSearch ──▶ 日志洞察页

K8s（新增）：
  Fluent Bit DaemonSet（每节点一个）
    └─ tail /var/log/containers/*.log（kubelet 已落盘的容器 stdout/stderr）
         └─ 同一个 OpenSearch ──▶ 同一个日志洞察页
```

dj-agent 不采集日志，只负责**控制面**：接收 djadmin 下发的采集规则 → 渲染成 Fluent Bit 配置写入 **ConfigMap** → DaemonSet 挂载自动热加载（对应虚拟机侧"写 inputs.d + 热重载"模式）。

### 5.2 索引与字段对齐

- 索引命名套用现有规则：`logs-<环境>-<业务系统>-<档位>`（hot/std/cold）
- namespace → 业务系统归属：优先用 namespace 命名约定（如 `tib-order` 正则提取），映射关系在平台维护
- K8s 元数据映射进现有 `labels_` 前缀字段体系：

| K8s 元数据 | 平台字段 | 说明 |
|---|---|---|
| namespace | `labels_namespace` | |
| Pod 名 | `labels_pod` | 保留原始名 |
| Pod 名去 hash | `labels_service` | 关键字段，使错误分布/突增识别按"服务"聚合的逻辑直接可用 |
| node 名 | `labels_host` | 与主机资产可关联 |

### 5.3 前端

检索页主机日志与容器日志共用同一入口，筛选条件增加集群/namespace；Pod 详情页提供日志跳转（URL 带 `labels_pod` 过滤）。错误分布、新增/突增错误识别逻辑零新增。

---

## 6. 高频操作

最小操作集：**重启 Deployment、扩缩容、删除 Pod**。

- 下发路径：djadmin → 现有 gRPC 通道 → 集群内 dj-agent → 翻译为 K8s API 调用
- 权限、审批、二次确认、审计全部走平台现有体系（这是相对 Rancher 的差异化价值）
- Agent 的 ServiceAccount RBAC 从只读起步，操作类权限按功能上线逐项放开

---

## 7. Prometheus 接入配置要点

### 7.1 双服务发现并存

```yaml
scrape_configs:
  # 主机目标 —— 现有 http_sd
  - job_name: hosts
    http_sd_configs:
      - url: http://autoadmin:8000/api/monitor/sd/
        refresh_interval: 1m

  # K8s 应用 Pod —— 注解声明式发现
  - job_name: k8s-pods
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: "true"
      # 端口/路径改写略
      - source_labels: [__meta_kubernetes_namespace]
        target_label: namespace
      - source_labels: [__meta_kubernetes_pod_label_business_system]
        target_label: business_system
      - source_labels: [__meta_kubernetes_pod_label_environment]
        target_label: environment
      - target_label: cluster
        replacement: <cluster-name>

  # 节点
  - job_name: k8s-nodes
    kubernetes_sd_configs:
      - role: node
```

### 7.2 部署与 RBAC

- Prometheus 部署在**集群内**（kubernetes_sd 依赖 in-cluster ServiceAccount），配套部署 kube-state-metrics、node-exporter
- ServiceAccount 只授予 get/list/watch（pods/services/endpoints/nodes/ingresses）

### 7.3 接入约定

业务 Pod 打注解即接入采集：`prometheus.io/scrape: "true"` + 端口/路径；`business_system` label 取值必须与 CMDB 业务系统 code 一致（见 §3.2）。

---

## 8. 实施顺序

| 阶段 | 内容 | 量级 |
|---|---|---|
| 1 | dj-agent K8s 模式（资产/状态上报）+ 集群资产模型 + 前端集群页（只读） | 1-2 周 |
| 2 | 告警收口：events 告警 + 调度器 Prometheus 数据源规则 | ~1 周 |
| 3 | 日志接入：Fluent Bit DaemonSet + namespace 映射 + 字段对齐 + ConfigMap 下发 | ~1 周 |
| 4 | 高频操作：重启/扩缩容/删 Pod + 审批审计 | ~1 周 |

前置条件：阶段 1 的 Agent K8s 模式与业务标签契约（§3.2）是后续所有阶段的地基。

---

## 9. 组件实现职责（待实现后补充最终逻辑）

| 组件 | 职责 |
|---|---|
| autoadmin 平台侧 | 集群资产模型与 CMDB 融合、调度器 Prometheus 数据源、告警模型扩展、namespace 映射、前端 API |
| dj-agent (Go) | K8s 模式运行形态（Deployment/单副本）、资产与事件 watch 上报、ConfigMap 配置下发、操作类 API 翻译 |

语义对齐点：gRPC 通道与命令模型复用现有协议；label 契约（§3.2）是平台、Agent 及 Prometheus 配置的共同依赖。

> 差异点待实现时补充：in-cluster 部署下 Agent 在线状态语义（gRPC 断连 vs 集群存活）与现有 `Host.agent_online` 机制的对应关系。
