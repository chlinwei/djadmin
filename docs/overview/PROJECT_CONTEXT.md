# 项目上下文 — djadmin 运维管理平台

> 本文是全局上下文的唯一入口；各功能的最终逻辑见 `docs/architecture/` 对应文档。
> 旧版（Django 时代）的上下文文档归档于 `docs/archive/PROJECT_CONTEXT_DJANGO.md`，仅作追溯。

## 项目概述

面向 IT 运维团队的管理平台：主机资产管理、凭证、服务树/应用目录、自动化编排（Ansible）、巡检、监控告警、日志采集、定时任务与权限审计。

## 技术栈与进程模型

| 组件 | 技术 | 说明 |
|---|---|---|
| 后端 | **Go（autoadmin/）** | 唯一后端实现。Gin + MySQL，单进程承载 API、定时调度（`internal/scheduler`，进程内 dispatcher）与巡检触发 |
| 执行代理 | **Go（dj_agent/）** | 目标主机上的常驻 agent，gRPC 双向流；承载自动化执行、巡检检查、指标/日志采集、WebSSH、文件传输 |
| 前端 | Vue 3 + Vite + Ant Design Vue（fronted/） | 管理控制台 |
| 认证 | JWT | |
| 历史栈 | Django + Celery | **已废弃并移出版本库**（历史见 git，归档见 `docs/archive/`），禁止作为实现依据（见 AGENTS.md） |

## 后端模块地图（autoadmin/internal）

| 模块 | 职责 | 架构文档 |
|---|---|---|
| api/router | 全部路由注册 | — |
| assets | 主机、凭证、服务树（项目/业务系统/环境/逻辑服务）、应用目录与部署 | ASSET_CATALOG.md |
| automation | Playbook / Inventory / 任务 / 作业执行 | AUTOMATION_INVENTORY.md、[AUTOMATION_JOB_EXECUTION.md](AUTOMATION_JOB_EXECUTION.md)（作业执行/超时/失联对账/日志边界） |
| inspection | 巡检组（分类、挂载点模型）/ 巡检任务 / 执行快照 | INSPECTION_ARCHITECTURE.md |
| monitor | 监控目标、Prometheus 集成与代理、告警（规则/路由/媒介）、主机列表 | MONITOR_PROMETHEUS_PROXY.md、ALERT_HISTORY_ARCHITECTURE.md、ALERT_NOTIFICATION_DISPATCH.md、ALERT_NOTIFICATION_CHAIN.md、ops/ALERT_MEDIA_SETUP_GUIDE.md |
| baseline | 基线（组/分类/检查项）与扫描 | BASELINE_ARCHITECTURE.md |
| scheduler | 进程内定时调度 | ops 说明以 INSPECTION_ARCHITECTURE.md 调度章节为准 |
| rbac / identity / audit | 菜单角色权限、用户、操作审计 | SYSTEM_MANAGEMENT_AUDIT.md |
| k8s | Kubernetes 集成 | K8S_INTEGRATION_ARCHITECTURE.md |
| logcollect | 日志采集（Filebeat 纳管/渲染/下发/体检/清理）与日志存储（ES 集群/索引模板/ILM/pipeline/检索） | LOG_COLLECTION_ARCHITECTURE.md |
| shared | binding / opapolicy / pagination / logstream / filebeat 等公共件 | — |

## 前端结构（fronted/src）

- `views/` 按域组织：sys / assets / automation / inspection / monitor / security / audit / userCenter
- `api/` 每域一个 API 模块；`util/` 公共工具（timezone、tableStyle、popupContainer 等）
- 表格/分页等 UI 约定见 `docs/architecture/CONVENTIONS.md`

## Agent 侧（dj_agent）

执行引擎架构见 `docs/architecture/DJ_AGENT_ARCHITECTURE.md` 与 `dj_agent/README.md`。

## 关键约定与规则

- API 设计（批量删除等）：`docs/architecture/CONVENTIONS.md`
- 响应格式：`.github/API_RULES.md`
- 协作与文档同步规则：`AGENTS.md`

## 运维

- 告警媒介配置：`docs/ops/ALERT_MEDIA_SETUP_GUIDE.md`
- 历史调度（Celery）文档已归档：`docs/archive/SCHEDULER_CELERY_README.md`
