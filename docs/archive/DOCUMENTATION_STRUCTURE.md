# 📚 djadmin 文档组织结构

## 目录概览

```
djadmin/
├── README.md                              # 【根】项目总览、快速开始
├── PROJECT_CONTEXT.md                     # 【根】项目架构、API接口全览
├── SCHEDULER_README.md                    # 【根】任务调度系统文档
├── SHELLSCRIPT_IMPLEMENTATION.md          # 【根】ShellScript功能实现
├── DJ_AGENT_ARCHITECTURE.md               # 【根】dj-agent架构设计
├── .gitignore                             # 忽略自动生成的报告
│
├── .github/                               # 🔧 开发规范和工具配置
│   ├── copilot-instructions.md            # Copilot AI助手指令
│   ├── API_RULES.md                       # ⭐ API规范规则（核心）
│   ├── agents/                            # 自定义Copilot代理
│   │   ├── frontend-specialist.agent.md   # 前端审查代理
│   │   └── backend-specialist.agent.md    # 后端审查代理
│   ├── instructions/                      # 文件级规范（auto-apply）
│   │   ├── frontend-view-standards.instructions.md
│   │   └── backend-api-standards.instructions.md
│   └── docs/                              # 📦 历史和归档文档
│       └── API_COMPLIANCE_REPORT.md       # (历史) API合规性检查报告
│
├── backend/djadmin/                       # 🐍 后端代码
│   ├── assets/AGENT_JOB_API.md            # Agent作业API文档
│   ├── user/API_TOKEN_API.md              # Token认证API文档
│   ├── automation/
│   │   ├── MODEL_RELATIONSHIP.md          # 数据模型关系图
│   │   └── WORKFLOW_RUNTIME_LOGIC.md      # Workflow运行逻辑
│   ├── sys_config/
│   │   └── SYS_CONFIG_CLEANUP_GUIDE.md    # 系统配置清理指南
│   └── dj_agent/                          # Go Agent组件
│
├── fronted/                               # 🎨 前端代码
│   ├── README.md                          # 前端项目概览
│   ├── WORKFLOW_FRONTEND_GUIDE.md         # 工作流前端实现指南
│   ├── LIST_SORTING_GUIDELINES.md         # 列表排序和排序规范
│   └── docs/                              # 📑 前端功能文档
│       └── (待添加特定功能文档)
│
└── dj_agent/                              # 🚀 Go Agent项目
    ├── README.md                          # dj-agent概览
    ├── DEVELOPMENT_PLAN.md                # 开发计划
    └── TASK_RESULT_SCHEMA_V1.md          # 任务结果数据结构
```

---

## 📖 文档分类说明

### 🎯 **根目录文档**（项目级全局视图）

| 文档 | 用途 | 更新频率 |
|------|------|---------|
| **README.md** | 项目入门、环境配置、快速开始 | 中等 |
| **PROJECT_CONTEXT.md** | 项目架构、数据模型、API全览（超详细） | 低（核心参考） |
| **SCHEDULER_README.md** | Celery + Beat 调度系统完整说明 | 低 |
| **SHELLSCRIPT_IMPLEMENTATION.md** | ShellScript功能实现详情 | 低（功能特性） |
| **DJ_AGENT_ARCHITECTURE.md** | dj-agent（Go Agent）架构设计 | 低（架构文档） |

**访问策略**: 新入职/理解全貌时优先阅读这些文档

---

### 🔧 **.github/ - 开发规范和工具**

#### 直接使用（核心文件）
| 文件 | 用途 |
|------|------|
| **API_RULES.md** | ⭐ **必读** - 所有API必须遵循的规范 |
| **copilot-instructions.md** | Copilot的项目级指令和最佳实践 |

#### agents/ - 自定义Copilot代理
| 代理 | 激活条件 |
|------|---------|
| **frontend-specialist.agent.md** | 审查Vue 3组件，检查Ant Design规范 |
| **backend-specialist.agent.md** | 审查Django API，检查响应格式 |

#### instructions/ - 文件级自动应用规范
这些规则会在您编辑对应文件类型时自动加载：

| 规范文件 | 应用范围 | 内容 |
|---------|---------|------|
| **frontend-view-standards** | `fronted/src/views/**/*.vue` | 工具提示、按钮样式、表格布局、删除交互、时区处理 |
| **backend-api-standards** | `backend/djadmin/*/views.py` | API响应格式、分页、类型提示、迁移影响、测试结构 |

#### docs/ - 历史/归档文档
| 文档 | 说明 |
|------|------|
| **API_COMPLIANCE_REPORT.md** | 2026-06-30 的API合规性检查历史报告 |

---

### 📚 **模块文档**（在源码目录中）

#### 后端模块
| 文件位置 | 内容 | 用途 |
|---------|------|------|
| `assets/AGENT_JOB_API.md` | Agent作业执行API详细说明 | Agent集成参考 |
| `user/API_TOKEN_API.md` | Token认证流程和接口 | 认证集成参考 |
| `automation/MODEL_RELATIONSHIP.md` | Automation模块的数据模型关系 | 了解数据结构 |
| `automation/WORKFLOW_RUNTIME_LOGIC.md` | Workflow执行引擎原理 | 工作流开发 |
| `sys_config/SYS_CONFIG_CLEANUP_GUIDE.md` | 系统配置清理和重置指南 | 部署运维 |

#### 前端模块
| 文件位置 | 内容 | 用途 |
|---------|------|------|
| `fronted/README.md` | 前端项目结构和开发指南 | 前端开发入门 |
| `fronted/WORKFLOW_FRONTEND_GUIDE.md` | Workflow前端实现细节 | Workflow UI开发 |
| `fronted/LIST_SORTING_GUIDELINES.md` | 列表排序和字段顺序规范 | 列表页面开发 |

#### dj-agent 项目
| 文件位置 | 内容 | 用途 |
|---------|------|------|
| `dj_agent/README.md` | Go Agent项目概览 | dj-agent入门 |
| `dj_agent/DEVELOPMENT_PLAN.md` | Agent开发路线图 | 功能规划参考 |
| `dj_agent/TASK_RESULT_SCHEMA_V1.md` | 任务结果数据结构定义 | Agent集成参考 |

---

## 🎯 快速导航

### 我想了解...请阅读:

| 需求 | 推荐阅读 | 优先级 |
|------|--------|------|
| 项目架构全貌 | `PROJECT_CONTEXT.md` | ⭐⭐⭐ |
| API规范和规则 | `.github/API_RULES.md` | ⭐⭐⭐ |
| 如何快速启动 | `README.md` + 各模块 README | ⭐⭐ |
| 后端API如何实现 | `.github/instructions/backend-api-standards.instructions.md` + 对应views.py | ⭐⭐ |
| 前端组件规范 | `.github/instructions/frontend-view-standards.instructions.md` + 对应vue | ⭐⭐ |
| 任务调度系统 | `SCHEDULER_README.md` | ⭐ |
| ShellScript功能 | `SHELLSCRIPT_IMPLEMENTATION.md` | ⭐ |
| Agent集成 | `DJ_AGENT_ARCHITECTURE.md` + `assets/AGENT_JOB_API.md` | ⭐ |

---

## 📝 文档维护规则

### ✅ 应该做的
- ✅ **模块特性文档** - 放在源码目录中（如 `automation/WORKFLOW_RUNTIME_LOGIC.md`）
- ✅ **项目全局文档** - 放在根目录（如 `PROJECT_CONTEXT.md`）
- ✅ **开发规范** - 放在 `.github/` 中（如 `.github/API_RULES.md`）
- ✅ **历史报告** - 放在 `.github/docs/` 中（不需要频繁查看）
- ✅ **自动生成的报告** - 添加到 `.gitignore`

### ❌ 不应该做的
- ❌ 同一功能的多份文档副本（导致维护困难）
- ❌ 过时/日期戳的测试报告提交到Git
- ❌ 在根目录堆积太多细节文档（保持简洁）
- ❌ 不相关的文档混在源码目录中

---

## 🚀 后续优化建议

1. **补充缺失的README**
   - [ ] `dj_agent/README.md` - Go Agent项目概览

2. **考虑添加的文档**
   - [ ] `fronted/docs/API_INTEGRATION_GUIDE.md` - 前端与后端API集成指南
   - [ ] `fronted/docs/COMPONENT_LIBRARY.md` - 可复用组件库文档
   - [ ] `.github/DEPLOYMENT_GUIDE.md` - 部署流程文档

3. **定期维护**
   - 每次大功能完成后，更新相关模块的文档
   - 每个季度检查一次过时文档并归档
   - 保持 `PROJECT_CONTEXT.md` 与代码同步

---

**最后更新**: 2026-08-17  
**文档版本**: v2.0 (整理后)
