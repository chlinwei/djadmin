# 前端开发说明（fronted）

> 功能架构从 [文档中心](../docs/README.md) 查阅；本文只维护前端运行、公共交互与质量检查。

登录系统后可通过顶级菜单「文档中心」在线阅读：左侧按业务菜单组织模块，右侧展示重点、
操作流程和可视化 Mermaid 流程图，并提供本篇章节导航。实现文件放在开发参考中，
计划、历史与未上线方案不随阅读器发布。
文档随前端构建发布，不需要后端文档接口或数据库迁移；更新生产文档需重新构建发布前端。
范围与安全边界见 [公共文档中心](../docs/architecture/SYSTEM_MANAGEMENT_AUDIT.md#公共文档中心)。

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

- 规范见本文的 [列表排序规范](#list-sorting)。

3. 时间范围选择组件统一规范

- 统一工具文件：`fronted/src/util/timezoneRange.js`
- 适用范围：所有使用 `a-range-picker + show-time` 的查询页面（包含审计页、监控历史告警、运行记录中心）。
- 统一要求：
	- 默认时间范围必须为用户时区当天 `00:00:00` 到 `23:59:59`。
	- 快捷时间（过去 5 分钟/1 小时/1 天等）必须基于用户时区“当前时刻”计算。
	- 查询参数提交给后端前，必须把“用户时区墙上时间”转换为 UTC ISO 字符串。
	- 页面内禁止重复实现同类转换逻辑，统一复用公共工具。

推荐接入方式：

```js
import { computed, ref } from 'vue'
import store from '@/store'
import {
	buildUserTimezoneShowTime,
	buildUserTimezoneRangePresets,
	toUtcQueryISOStringByUserTimezone,
} from '@/util/timezoneRange'

const userTimezone = computed(() => store.state.user?.timezone || 'Asia/Shanghai')
const timeRangePresets = ref([])
const timeRangeShowTime = buildUserTimezoneShowTime(userTimezone.value)

function onTimeRangeOpenChange(open) {
	if (open) {
		timeRangePresets.value = buildUserTimezoneRangePresets(userTimezone.value)
	}
}

function buildQueryParams(startTime, endTime) {
	return {
		start_time: toUtcQueryISOStringByUserTimezone(startTime, userTimezone.value),
		end_time: toUtcQueryISOStringByUserTimezone(endTime, userTimezone.value),
	}
}
```

模板绑定示例：

```vue
<a-range-picker
	v-model:value="filters.timeRange"
	:show-time="timeRangeShowTime"
	:presets="timeRangePresets"
	:getPopupContainer="getPopupContainer"
	@openChange="onTimeRangeOpenChange"
	@change="handleTimeRangeChange"
/>
```

当前已接入页面（基线）：

- `fronted/src/views/audit/login/index.vue`
- `fronted/src/views/audit/operationLog/index.vue`
- `fronted/src/views/audit/webssh/index.vue`
- `fronted/src/views/monitor/alerts/index.vue`
- `fronted/src/views/automation/logs/center/controller.js`（运行记录中心）

4. 自动刷新 + keep-alive 生命周期统一规范

- 统一工具文件：`fronted/src/util/keepAliveRefresh.js`
- 适用范围：所有包含定时刷新（`setInterval`）且页面可能被 keep-alive 缓存的页面。
- 统一要求：
	- 页面切换到其他菜单（deactivated）时必须暂停轮询，禁止后台持续请求。
	- 页面返回（activated）时恢复轮询。
	- 页面销毁（beforeUnmount）时仍需保留兜底清理，防止定时器泄漏。
	- 禁止在页面内重复手写 activated/deactivated 轮询样板逻辑，统一复用公共工具。

推荐接入方式：

```js
import { onBeforeUnmount } from 'vue'
import { useKeepAliveRefreshLifecycle } from '@/util/keepAliveRefresh'

function startPolling() {
  // 启动轮询
}

function stopPolling() {
  // 停止轮询
}

useKeepAliveRefreshLifecycle(startPolling, stopPolling)

onBeforeUnmount(() => {
  stopPolling()
})
```

当前已接入页面（基线）：

- `fronted/src/views/assets/host/index.vue`（主机管理）
- `fronted/src/views/monitor/index.vue`（监控中心）
- `fronted/src/views/monitor/alerts/index.vue`（告警）
- `fronted/src/views/automation/logs/center/controller.js`（运行记录中心）

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

### 1) WebSSH 页面（`/views/assets/host/webssh/index.vue`）

- 左侧文件区支持目录浏览、过滤、右键菜单、拖拽上传。
- 文件区返回按钮为 **↑**，语义为“返回上一次访问目录”（类似 `cd -`）。
- 右键菜单支持“复制目录路径”（文件/目录均复制父目录路径）。
- 右键菜单打开时，当前行会高亮，便于确认操作对象。

### 2) 自动化任务运行记录（`/views/sys/automation/logs.vue`）

- `pending` 状态任务不显示“下载日志”按钮。
- 仅在任务有可用执行结果时显示下载入口。

### 3) Workflow 前端文档

- 详细说明见 [Workflow 前端说明](WORKFLOW_FRONTEND_GUIDE.md)。
- 包含页面职责、数据结构、状态映射、交互约束、后端契约与联调建议。

## 相关联后端依赖

- 自动化运行方式、日志与取消语义见 [自动化作业](../docs/architecture/AUTOMATION_JOB_EXECUTION.md)，不依赖已废弃的 Celery。
- WebSSH 与文件传输复用 api 持有的 agent gRPC 会话，见 [Agent 架构](../docs/architecture/DJ_AGENT_ARCHITECTURE.md)。


<a id="list-sorting"></a>

## 列表排序规范

本规范用于统一前端列表页是否开启列排序，避免“每列都可排序”导致的体验噪音与后端成本膨胀。

### 目标

- 只给高价值字段开启排序。
- 排序行为与后端 `ordering` 字段一致，避免前后端语义不一致。
- 默认采用“最小可用排序集”，按需渐进扩展。

### 字段类型与排序建议

#### 必加排序（默认开启）

- 时间类字段：`create_time`、`update_time`、`start_time`、`end_time`、`run_time`
- 状态类字段：`status`、`enabled`
- 标识类字段：`id`、`job_id`、`run_id`
- 数值类核心指标：`duration_seconds`、`count`、`total`

#### 选加排序（业务确认后开启）

- 名称类字段：`name`、`task_name`、`workflow_name`
- 优先级/权重类字段：`priority`、`weight`
- 版本号/序号类字段：`version`、`order_num`

说明：名称类字段仅在“列表规模较大且用户确有按字母定位需求”时开启。

#### 不建议排序（默认关闭）

- 操作列：`action`
- 复杂拼接展示列（如多标签聚合文本）
- JSON 摘要列（如环境变量片段）
- 跳转链接列（仅用于导航，不用于比较）

### 前端实现要求

- 仅在列定义中显式声明可排序列：`sorter: true`
- 表格 `@change` 事件中统一处理排序参数并转换为后端 `ordering`
- 未声明排序的列，禁止展示排序 UI（避免误导）

### 后端契约要求

- API 必须声明可排序字段白名单（`ordering_fields`）
- 前端只能传白名单内字段，防止无效排序请求
- 推荐默认排序：`-id` 或 `-create_time`

### 页面最小可用排序集（当前项目建议）

- 自动化任务列表：`name`、`enabled`、`update_time`
- Workflow 列表：`name`、`enabled`、`update_time`
- 运行记录中心（任务）：`id`、`status`、`start_time`、`duration_seconds`
- 运行记录中心（Workflow）：`id`、`status`、`start_time`、`duration_seconds`

### 验收清单

- 排序字段是否都在后端白名单中
- 升序/降序是否都正确
- 翻页后排序状态是否保持
- 与筛选联用时结果是否正确
