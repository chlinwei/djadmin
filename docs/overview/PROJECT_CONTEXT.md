# 项目上下文 — djadmin 运维管理平台

> 本文介绍项目和进程边界；完整业务域导航见 [文档中心](../README.md)。
> 各功能最终逻辑只维护在对应主文档，不在后端模块目录重复。
> 旧版（Django 时代）的上下文文档归档于 `docs/archive/PROJECT_CONTEXT_DJANGO.md`，仅作追溯。

## 项目概述

面向 IT 运维团队的管理平台：主机资产管理、凭证、服务树/应用目录、自动化编排（Ansible）、巡检、监控告警、日志采集、定时任务与权限审计。

## 技术栈与进程模型

| 组件 | 技术 | 说明 |
|---|---|---|
| 后端 | **Go（autoadmin/）** | 唯一后端实现。Gin + MySQL/PostgreSQL；一个二进制分 api / scheduler / worker / migrate 角色，scheduler 数据库持久化调度并直接执行清理，巡检触发在 api |
| 执行代理 | **Go（dj_agent/）** | 目标主机上的常驻 agent，gRPC 双向流；承载自动化执行、巡检检查、指标/日志采集、WebSSH、文件传输 |
| 前端 | Vue 3 + Vite + Ant Design Vue（fronted/） | 管理控制台 |
| 认证 | JWT | |
| 历史栈 | Django + Celery | **已废弃并移出版本库**（历史见 git，归档见 `docs/archive/`），禁止作为实现依据（见 AGENTS.md） |

## 后端模块地图（autoadmin/internal）

| 模块 | 职责 | 架构文档 |
|---|---|---|
| api/router | 全部路由注册 | — |
| assets | 主机、凭证、服务树（项目/业务系统/环境/逻辑服务）、应用目录与部署 | [资产目录](../architecture/ASSET_CATALOG.md) |
| automation | Playbook / Inventory / 任务 / 作业执行 | [自动化作业](../architecture/AUTOMATION_JOB_EXECUTION.md)（含 Inventory、超时、失联对账与日志） |
| inspection | 巡检组（分类、挂载点模型）/ 巡检任务 / 执行快照 | [巡检](../architecture/INSPECTION_ARCHITECTURE.md) |
| monitor | 监控目标、Prometheus、主机列表、告警规则与通知 | [监控](../architecture/MONITOR_PROMETHEUS_PROXY.md)、[告警](../architecture/ALERT_HISTORY_ARCHITECTURE.md) |
| baseline | 基线（组/分类/检查项）与扫描 | [基线](../architecture/BASELINE_ARCHITECTURE.md) |
| scheduler | 数据库持久化计划与执行记录，scheduler 有界执行池直接清理，无需 RabbitMQ | [定时清理](../architecture/SCHEDULER_ARCHITECTURE.md)；巡检自身调度见巡检主文档 |
| rbac / identity / audit | 菜单、角色权限、用户、头像、系统参数与审计 | [系统管理](../architecture/SYSTEM_MANAGEMENT_AUDIT.md) |
| k8s | Kubernetes 集成 | [Kubernetes](../architecture/K8S_INTEGRATION_ARCHITECTURE.md) |
| logcollect | Filebeat 纳管/渲染/下发/体检/清理；ES 模板/ILM/pipeline/检索 | [日志采集](../architecture/LOG_COLLECTION_ARCHITECTURE.md) |
| shared | binding / opapolicy / pagination / logstream / filebeat 等公共件 | — |

## 前端结构（fronted/src）

- `views/` 按域组织：sys / assets / automation / inspection / monitor / security / audit / userCenter
- `api/` 每域一个 API 模块；`util/` 公共工具（timezone、tableStyle、popupContainer 等）
- 表格/分页等 UI 约定见 `docs/architecture/CONVENTIONS.md`

## Agent 侧（dj_agent）

执行引擎、通信和安装更新见 [Agent 架构](../architecture/DJ_AGENT_ARCHITECTURE.md)，
构建与运行见 [Agent README](../../dj_agent/README.md)。

## 后端边界与执行模型

- 一个二进制按 `api / scheduler / worker / migrate` 角色独立部署；API 持有实时 agent gRPC 会话，
  会话不在普通数据库或 RabbitMQ 中共享。
- 通常按 `handler → service → repository` 分层；HTTP 解码/响应、业务校验/事务、sqlc 查询各司其职。
  基础设施包不反向导入 HTTP handler。
- 定时清理由数据库调度并直接执行，清理和成功结果同事务；日志采集批量动作仍由 api 消费 RabbitMQ，
  解析失败或不支持的消息进入死信。各自的重试与恢复边界以对应主文档为准。
- 时间戳统一存储和交换 UTC；前端使用用户时区显示。凭证、私钥等敏感值不得写入消息或日志。
- Ansible 是控制端外部进程，默认构建内嵌 CPython + ansible-core；运行依赖与安装更新细节见 Agent 主文档。

## 关键约定与规则

- API 设计（批量删除等）：[开发与设计约定](../architecture/CONVENTIONS.md)。
- 响应格式：[API 规则](../../.github/API_RULES.md)。
- 协作与文档同步规则：[AGENTS.md](../../AGENTS.md)。

## 运维

- 告警媒介配置：[操作手册](../ops/ALERT_MEDIA_SETUP_GUIDE.md)。
- 历史调度（Celery）：[归档说明](../archive/SCHEDULER_CELERY_README.md)，不是当前运行方式。
