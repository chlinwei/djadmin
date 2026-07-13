# Copilot Instructions for djadmin

## Project Context & Rules

- **Primary Context**: Use [PROJECT_CONTEXT.md](../PROJECT_CONTEXT.md) as the primary context source for this repository.
- **API Standards**: Follow [API_RULES.md](API_RULES.md) for all API response formats, status codes, and implementation requirements.
- **Compliance Status**: See [API_COMPLIANCE_REPORT.md](API_COMPLIANCE_REPORT.md) for current API return format compliance check (55% compliant).
- **User Request First**: If the user request conflicts with these files, follow the user request first.

## Tech Stack

- Backend: Django + DRF (Python)
- Database: MySQL
- Frontend: Vue 3 + Vite
- Frontend UI: Ant Design Vue (primary)
- Scheduler: Celery + RabbitMQ (worker + beat separate processes required)

## Repository Layout

- Backend root: backend/djadmin
- Frontend root: fronted
- Main backend apps: user, role, menu, assets, scheduler, sys_config

## Working Rules

- **默认禁止兼容层**: 未经用户明确要求，禁止新增 legacy/fallback/shim/向后兼容分支；优先统一到当前唯一实现路径。
- **兼容代码准入**: 只有在用户明确要求“兼容旧数据/旧接口/灰度迁移”时，才允许引入兼容代码，并且必须标注移除条件与目标版本。

- **Pylance 类型优先**: 修改后端代码时，首先使用 `get_errors` 检查并解决 Pylance 报告的类型问题，再提交最终方案。区分真正的类型错误（需修复代码）和库的误报（加 `# type: ignore`）。
- Do not break existing API contracts unless user explicitly asks for API changes.
- Prefer minimal, targeted edits and keep current code style.
- **注释规范**: 对非直观逻辑（复杂分支、关键边界条件、协议/时区/权限等）必须补充简洁注释；禁止无信息量注释（如“给变量赋值”）。
- **全局影响/复杂改动注释强制规则**: 任何会影响全局行为的代码（如全局配置、路由守卫、全局拦截器、共享状态、权限/鉴权链路）以及复杂实现（跨模块联动、异步并发、回退/兜底策略）必须补充“为什么这样做”的简洁注释；PR 或回复中需说明影响范围与回归验证点。
- For frontend UI work, use Ant Design Vue components and patterns by default.
- Do not introduce new Element Plus-based UI changes unless user explicitly requests it.
- Keep button icons globally consistent across all pages: use the same icon mapping/style for the same action (add, edit, delete, refresh, save, details) and avoid mixing multiple icon styles for identical actions.
- **前端按钮样式强制流程**（必须执行，禁止跳步）:
  - 先锁定唯一基准页面；未得到用户确认前，不得直接修改按钮样式。
  - 默认基准为 `fronted/src/views/assets/credential/index.vue` 的新增按钮样式（大号默认按钮 + plus-circle 图标 + 文本）。如用户明确指定其他基准，以用户指定为准。
  - 先提交“差异清单”（页面范围 + 按钮类型：新增/操作列/弹窗 + 计划改法），用户确认后再实施代码修改。
  - 同类按钮需一次性统一完成，禁止只改部分页面或只改一种按钮类型导致风格再次分叉。
  - 修改完成后，必须提供逐页对照结果（改动文件 + 统一项清单 + 验证结果）。
- **前端表格操作列强制规则**（必须执行）:
  - 所有列表页存在“操作”列时，必须设置 `fixed: 'right'`，并为操作列配置明确 `width`。
  - 同一表格必须开启横向滚动（`a-table` 配置 `:scroll="{ x: ... }"`），禁止仅固定操作列而不配置横向滚动，避免列挤压。
  - 优先对齐 `fronted/src/views/sys/user/index.vue` 的可用体验（操作列固定在右侧，主内容可横向滚动）。
  - 新增或改造列表页时，若未满足上述两项，视为不合格实现，必须在本次改动内补齐。
- **前端操作按钮 Tooltip 强制规则**（必须执行）:
  - 所有列表页“操作”列中的可点击按钮（含图标按钮、文本按钮、危险按钮）必须统一使用 `a-tooltip` 包裹，禁止同页出现“部分有 tooltip、部分无 tooltip”。
  - Tooltip 文案必须使用统一动作词：`编辑`、`运行`、`删除`、`历史记录`、`详细日志`、`统一日志`、`下载日志`、`取消`、`查看状态图`。
  - `a-popconfirm` 场景（如删除/取消）也必须保持有 tooltip：在按钮层提供 `a-tooltip`，点击后再进入 `a-popconfirm` 二次确认。
  - 禁止依赖浏览器原生 `title` 作为统一提示方案；操作提示以页面显式 `a-tooltip` 为准。
- **前端 Tooltip 设计理念**（统一体验基线）:
  - Tooltip 的目标是“降低理解成本”，不是制造视觉干扰：同类动作在全站必须同文案、同交互、同出现时机。
  - Tooltip 不能影响主操作命中：提示层不得遮挡按钮点击区域，禁止出现“看得到按钮但点不到”的交互。
  - 操作按钮 Tooltip 默认在按钮上方展示（`placement: 'top'`），优先保证不挡按钮、不挡主要操作路径。
  - 统一优先于局部最优：禁止单页自定义一套 tooltip 规则，新增页面必须复用既有动作词和交互模式。
  - 规则必须可回归验证：涉及 tooltip 的改动应至少覆盖“可见性 + 可点击 + 位置”三类验证点，避免仅靠肉眼判断。
- **前端删除交互强制规则（必须执行）**:
  - 所有“删除”动作（单删/批删）必须复用统一公共删除确认代码：`fronted/src/util/deleteConfirm.js` 的 `openDeleteConfirm`，禁止新增 `a-popconfirm` 式删除确认分支。
  - 删除按钮必须使用统一样式类 `delBtn`，确保点击与 loading 状态宽度不抖动；禁止出现“有的删除按钮会变宽、有的不会”的不一致。
  - 删除确认弹窗必须居中显示，并展示待删除清单（至少包含实体名称或 ID），禁止只给“您确定要删除吗”而不展示影响对象。
  - 主机分组删除等已有“树形影响清单”特化弹窗属于可保留特例；其余删除交互默认统一到公共删除确认代码。
- **前端下拉弹层强制规则（必须执行）**:
  - 所有 `a-select` 必须显式配置 `:getPopupContainer`，禁止依赖默认挂载行为。
  - `:getPopupContainer` 必须复用 `fronted/src/util/popupContainer.js` 的 `resolvePopupContainerByContext`，禁止每个页面重复造轮子。
  - 页面内统一定义 `const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)`，并绑定到该页面全部 `a-select`。
  - 禁止在 `App.vue` 做全局 `getPopupContainer` 强绑定，防止跨页面场景冲突导致“有时可选、有时不可选”。
  - 新增/改造包含 `a-select` 的页面时，必须同步检查是否遗漏 `:getPopupContainer`。
- **Inventory 字段规则（前端表单）**:
  - 默认按业务页面语义实现，**不得擅自将 Inventory 改为必填**。
  - 仅当用户明确要求“该页面 Inventory 必填”时，才允许添加 `required`、必填文案和提交拦截。
- **时间显示强制规则（每次写前端时间显示代码都必须遵守，禁止跳过）**:
  - ❌ 禁止：`new Date(x).toLocaleString('zh-CN')`、`toLocaleString()`、任何硬编码 locale 或 timezone
  - ✅ 必须：从用户 store 取时区，配合 `formatTimeWithTimezone` 显示
  - 时区工具：`import { formatTimeWithTimezone } from '@/util/timezone'`
  - 用户时区：从 pinia store 中读取（先用 `grep_search` 查找实际字段名，不要猜测）
  - 后端时间戳以 UTC 存储返回，前端负责转换为用户当前时区显示
  - **实际用法（已验证）**：`import store from '@/store'`，时区取 `store.state.user?.timezone || 'Asia/Shanghai'`，显示用 `formatTimeWithTimezone(value, tz)`
- For backend changes, include migration impact notes when models are changed.
- For scheduler-related issues, always verify whether Celery worker and beat processes are running.
- For bug fixes, provide quick verification steps (commands + expected result).

## Common Run Commands

### Backend

**First, activate virtual environment (Windows PowerShell):**

```powershell
(Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned) ; (& c:\workspace\python\djadmin\backend\.venv\Scripts\Activate.ps1)
cd backend/djadmin
python manage.py runserver 0.0.0.0:8000
```

**Or use these commands:**

- `cd backend/djadmin`
- Activate venv: `(Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned) ; (& .\.venv\Scripts\Activate.ps1)`
- `pip install -r requirements.txt` (if needed)
- `python manage.py runserver`

### Frontend

- cd fronted
- npm install
- npm run dev
- npm run build

### Scheduler

- Recommended one-command startup: `cd backend/djadmin && python manage.py runscheduler` (starts worker + beat)
- Split mode (for debugging):
  - Worker: `cd backend/djadmin && python manage.py runceleryworker --loglevel=info --concurrency=2`
  - Beat: `cd backend/djadmin && python manage.py runcelerybeat --loglevel=info`

### Tests & Test Report

- Run backend tests (use test settings + keepdb):
  - `cd backend/djadmin`
  - `python manage.py test user role assets --settings=djadmin.test_settings --keepdb`
- Generate the Markdown test report:
  - `cd backend/djadmin`
  - `python generate_test_report.py`
  - Optional flags: `--apps user role assets` (which apps to test), `--output ../../TEST_REPORT.md` (report path).
  - Output is written to `TEST_REPORT.md` at the repo root by default.
  - The report includes a summary, a per-module summary, and a per-case detail table (module, test class, case name, description, duration, result).
- Test conventions:
  - API format compliance is verified inside business test cases via `BaseTestCase.assertResponseOK` (checks `{code, msg, data}` structure + `code==200`). Do NOT add a separate format-only test file; assert format alongside business assertions.
  - Use `assertSuccess` when the case also requires `msg == 'success'` (pure CRUD success paths).
  - SSH collection tests against fake IPs print connection-timeout tracebacks — these are expected, not failures.

## Default Credentials

- **Username:** admin
- **Password:** admin

Login at: http://localhost:8000/sys/login or http://localhost:5173/login

## API Response Format

**All successful API responses return HTTP status 200** (except for authentication/network errors).

Response structure:
```json
{
  "code": 200,
  "msg": "success",
  "data": {
    ...response data...
  }
}
```

Response codes:
- `200` - Success (always use this for all successful operations)
- `300` - Account/password error
- `301` - Token validation failed / login expired
- Other codes - Various errors (msg field describes the error)

Pagination response format (for list endpoints):
```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [...items...],
    "count": 10,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 1,
    "next": null,
    "previous": null
  }
}
```

## Response Preference

- User prefers not to repeat project context in every conversation.
- When possible, infer missing context from repository and this instruction file first.
