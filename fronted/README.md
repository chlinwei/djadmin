# 前端开发说明（fronted）

## 技术栈

- Vue 3 + Vite
- Ant Design Vue
- xterm.js（WebSSH 终端）

## 安装与运行

```bash
cd fronted
npm install
npm run dev
```

## 构建

```bash
npm run build
```

## 质量守卫

1. 一键质量守卫（唯一入口）

```bash
npm run check:ui-rules
```

- 说明：按顺序执行“时间显示一致性检查 + 列表排序一致性检查 + 删除交互一致性检查”，任一步失败即退出。

2. 列表排序功能规范

- 规范文档：`fronted/LIST_SORTING_GUIDELINES.md`

## 前端测试

```bash
# 交互模式
npm run test

# 一次性执行
npm run test:run

# 生成覆盖率（HTML + 控制台摘要）
npm run coverage

# 生成类似后端的 Markdown 报告（会先执行测试）
npm run test:report
```

默认输出：项目根目录 `FRONTEND_TEST_REPORT.md`。

## 关键页面说明

### 1) WebSSH 页面（`/views/assets/host/webssh.vue`）

- 左侧文件区支持目录浏览、过滤、右键菜单、拖拽上传。
- 文件区返回按钮为 **↑**，语义为“返回上一次访问目录”（类似 `cd -`）。
- 右键菜单支持“复制目录路径”（文件/目录均复制父目录路径）。
- 右键菜单打开时，当前行会高亮，便于确认操作对象。

### 2) 自动化任务运行记录（`/views/sys/automation/logs.vue`）

- `pending` 状态任务不显示“下载日志”按钮。
- 仅在任务有可用执行结果时显示下载入口。

### 3) Workflow 前端文档

- 详细说明见 `fronted/WORKFLOW_FRONTEND_GUIDE.md`。
- 包含页面职责、数据结构、状态映射、交互约束、后端契约与联调建议。

## 相关联后端依赖

- 自动化执行依赖 Celery Worker 消费任务。
- WebSSH 文件传输默认走 ticket + transfer-service 链路。
