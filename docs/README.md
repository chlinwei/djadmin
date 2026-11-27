# 文档中心

> **按系统菜单找模块，先看重点和流程图，再查操作与排障。这里描述当前实现，不展示旧计划和历史流水账。**

## 先看全局流程

```mermaid
flowchart LR
    A["登录认证"] --> B["加载菜单与权限"]
    B --> C["资产管理：主机、应用、服务树"]
    C --> D["自动化运维与巡检"]
    C --> E["监控告警与通知"]
    C --> F["日志采集与查询"]
    C --> G["安全基线扫描"]
    B --> H["系统管理：用户、授权、参数与清理"]
```

## 按菜单阅读

| 系统入口 | 模块手册 | 最需要关注 |
|---|---|---|
| 公共基础 | [登录认证与权限](architecture/modules/ACCESS.md) | 登录与鉴权流程、权限快照、失效边界 |
| 首页 | [首页](architecture/modules/HOME.md) | 运维概览、近期动态、快捷入口及统计口径 |
| 资产管理 | [资产管理](architecture/modules/ASSETS.md) | 主机/Agent、项目、环境、凭据、应用、服务树与删除 |
| 自动化运维 | [模板、Inventory、任务与运行记录](architecture/modules/AUTOMATION.md) | 提交与成功的区别、执行位置、取消边界 |
| 自动化运维 → 巡检中心 | [巡检中心](architecture/modules/INSPECTION.md) | 采集 → OPA → 断言 → 报告，失败与错误的区别 |
| 监控中心 → 监控告警 / 通知管理 | [监控告警与通知](architecture/modules/MONITORING.md) | 指标、告警摄取和通知分发分别排查 |
| 监控中心 → 日志管理 | [日志管理](architecture/modules/LOGGING.md) | 配置 → Filebeat → ES，停止、换档与删除的区别 |
| 安全中心 → 基线扫描 | [基线扫描](architecture/modules/SECURITY.md) | OS 合规检查，不是漏洞扫描或自动修复 |
| 安全中心 → 安全问题 | [安全问题台账](architecture/SECURITY_ISSUES.md) | 跨扫描去重、按风险聚合、复查关闭与复发历史 |
| 系统管理 | [系统管理](architecture/modules/SYSTEM.md) | 用户/组/角色、菜单、参数、调度与审计 |
| 右上角用户菜单 | [个人中心](architecture/modules/PROFILE.md) | 密码、时区与通知收件绑定 |
| 文档中心 | [阅读与维护说明](architecture/modules/DOCUMENTATION.md) | 目录、章节、流程图与发布边界 |

## 阅读方法

```mermaid
flowchart LR
    A["按菜单选择手册"] --> B["阅读重点与操作"]
    B --> C["查看流程和失败分支"]
    C --> D["需要时查阅技术细节"]
```

- [逻辑服务删除全流程](architecture/modules/ASSETS.md#service-deletion)：查看删除、重试、重建和影响范围。
- 「提交」不等于「执行成功」；菜单能否操作以实际角色权限为准。

## 开发参考

模块手册是日常第一入口；底层数据结构和执行约束按需查阅：

- [项目与进程边界](overview/PROJECT_CONTEXT.md)、[开发与 API 约定](architecture/CONVENTIONS.md)、[SQL 设计](architecture/SQL_DESIGN.md)。
- 构建与运行：[后端](../autoadmin/README.md)、[前端](../fronted/README.md)、[Agent](../dj_agent/README.md)。
- 部署：[后端](../autoadmin/deploy/README.md)、[Agent](../dj_agent/deploy/README.md)。

**维护要求：** 功能变更必须同步模块说明和流程图；不将未上线设计、旧实现说明或临时修复记录混入模块手册。
仓库保留的计划与历史仅用于追溯，不随系统阅读目录发布。
