# 开发与设计约定（强制）

> 本文是项目级约定的**唯一汇总入口**，避免规范碎片化。新增约定先查本文，属同类主题的并入对应章节，不要另起新文件。违反约定的代码在 review 时直接打回。
> API 响应体格式另见 [.github/API_RULES.md](../../.github/API_RULES.md)。

<a id="browser-tool-approval"></a>

## AI 浏览器工具审批

当前仓库默认使用源码、单测和构建验证；浏览器操作必须逐次经用户确认，不能把共享页面、
历史批准或前端工作本身当作授权。没有经授权的测试登录条件时，停止浏览器验收，
不重复访问登录页，不宣称功能已验证。不得通过终端、子代理或批量包装绕过此限制。

Copilot 运行时通过仓库级 [browser-approval.json](../../.github/hooks/browser-approval.json)
注册 `preToolUse` 命令钩子，执行 [browser-approval.cjs](../../.github/hooks/browser-approval.cjs)：

- VS Code 浏览器工具及带浏览器/Playwright 命名空间的工具返回 `permissionDecision: "ask"`，
  每次调用都重新请求确认，不记录或复用批准；不支持交互确认的模式应拒绝调用。
- 混入浏览器调用的批量包装直接拒绝，必须拆成逐次审批。
- 非浏览器工具返回空决定，保留宿主原有权限检查，不自动放行。
- 输入不合法时返回明确拒绝；脚本不访问网络，也不输出工具参数、URL、登录口令或令牌。

本地逻辑测试：`node --test .github/hooks/browser-approval.test.cjs`。
这些测试只给脚本传入模拟 JSON，不调用浏览器或访问网站。
宿主加载测试使用无害 Bash 命令 `printf DJADMIN_BROWSER_APPROVAL_PROBE`：
正确加载时必须被钩子拒绝并返回 `probe intercepted successfully`；
如果命令实际输出标记，则当前宿主尚未加载钩子，不能宣称审批闸已生效。

仓库文件存在、配置解析成功和本地测试通过均不能代替宿主加载验证。
新增钩子后按宿主要求重新加载或新建 Copilot 会话，再用无害探针确认。
本配置使用共享 Copilot 运行时的原生格式，不为其他运行时提供兼容分支。
钩子仍受宿主能力与配置控制，且命令钩子超时会回到宿主原权限流程；
因此不能把项目钩子描述为不依赖宿主、不可绕过的安全沙箱。
格式与失败语义以 [Copilot 官方钩子参考](https://docs.github.com/en/copilot/reference/hooks-reference)
为准。

## 查询请求方法

查询必须使用 GET，筛选条件通过 URL 查询参数传递；不能因为需要传 ID 列表或多个筛选条件而使用 POST，也不保留 POST 查询分支。ID 列表默认使用逗号分隔的参数，并严格校验；缺失或非法参数不得意外扩大查询范围。创建任务、执行动作及写入业务状态的接口不属于纯查询。

## 一、删除类 API：只保留批量删除

列表型资源的删除接口**只允许一个批量删除 API**，不提供单条删除接口；删除单条即传 `ids: [id]`。

- 路由：`POST <资源前缀>/batch-delete/`
- 请求体：`{"ids": [<数字>...]}`
- 权限点：沿用删除权限（如 `inspection:tasks:delete`）
- 响应 data：`{"count": <成功条数>, "results": [{"id": 1, "ok": true, "message": ""}, ...]}`，不存在的 id 记 `ok:false`，不整体失败
- handler 范式：逐 id 复用单删的全部前置校验，参考 `autoadmin/internal/logcollect/log_target_actions.go` 的 `BatchDeleteLogTargets`
- 前端：每个资源在 `src/api/**` 只保留一个 `batchDeleteXxx(ids)`；单删按钮传 `[record.id]`
- 带 body 的删除统一用 POST（避免 DELETE+body 的网关兼容性问题）

**例外**（非列表型删除，允许保留原接口）：

- 树/级联删除：菜单 `deleteMenuById`
- 嵌套子资源：基线分类/检查项（`/baselines/:id/categories/:cid/`）
- 从属资源：主机 WebSSH 文件

### 一.1 批量更新：沿用同一响应口径，但**语义按资源定**

"只保留批量删除"不延伸到更新——更新接口的批量与否由**这一批改动是否可拆**决定，不搞"一律批量"：

- 路由：`POST <资源前缀>/batch-update/` 或嵌在已有子路径下（如
  `/assets/application-services/:id/log-config/settings/batch/`，与单条 `…/settings/` 同级）
- 响应 data：**与批量删除同一口径** —— `{"count": <成功条数>, "results": [{"id": …, "ok": true, "message": ""}, ...]}`，
  不适用/不存在的项记 `ok:false` 并写明原因
- **可拆的改动逐条独立、不整体失败**（每个 id 的改法不同，败一条不该拖累其余）
- **不可拆的改动（整批改的是同一列的同一个值）用一次事务写完**：要么都落库、要么都不落，
  失败即返回错误码，不返回"部分成功"——留半套配置比整体失败更难排查。
  范式：`autoadmin/internal/assets/log_setting.go` 的 `BatchSaveServiceLogOverrides`
  （校验读一次 → 过滤不属于本资源的项 → `UpsertServiceLogOverrides` 一个事务 → 逐条结果）
- 参数非法（空集合、超过上限）在服务层直接 `ErrInvalid`，不进事务
- 前端：一次请求 + 成功后**重拉列表与下发态**；把失败的 id 翻成用户认识的名字（日志名/主机名）再报出来，
  只回 id 用户不知道是哪一条

## 二、前端 a-table 统一风格

所有数据列表性质的 `<a-table>` 统一为：

1. **分页**：从 `@/util/tableStyle` 导入 `createPagination()`，以 `const pagination = reactive(createPagination())` 创建；`total/current/pageSize` 请求后由页面赋值。默认口径：
   - `pageSizeOptions: ['10','20','30','50']`、`showSizeChanger`、`showQuickJumper`
   - `showTotal` 统一为 `共 ${total} 条记录`（改文案只改 tableStyle.js 一处）
   - 特殊需求通过 extra 参数覆盖（如日志场景 `['50','100','200']`）
2. **密度**：`size="small"`。
3. **空状态**：默认 `:locale="tableLocale"`（中文「暂无数据」）；有更精确业务提示时保留自定义。
4. **不分页表格**：显式声明 `:pagination="false"`。

禁止在页面里手写 `showTotal`、`pageSizeOptions` 等分页配置；新增页面一律 import `createPagination` / `tableLocale`。

### 前端运行入口与动态引用边界

- `fronted/src/main.js` 注册路由、Vuex、权限指令、Ant Design Vue、FontAwesome 和全局 `SvgIcon`。
- 路由同时包含静态入口和菜单驱动入口；`src/router/index.js` 通过 `import.meta.glob("../views/**")`
  按菜单的 `component` 字段解析页面，不能仅凭缺少显式 import 判定视图未使用。
- SVG 图标由 Vite 的 `createSvgIconsPlugin` 扫描 `src/assets/icons/svg`，通过
  `virtual:svg-icons-register` 注册；图标名属于运行时引用，不以单个 SVG 的 import 数量判定存活。
- API 模块由调用页面/控制器显式导入；保留实际调用的请求封装，后端仍提供某个端点不代表前端需要为其保留未使用的封装。

## 三、懒挂载弹窗的数据加载

父组件用 `v-if` + `open` 懒挂载的弹窗（关闭延迟卸载动画结束后 `v-if` 置 false 的模式），
**首次打开时组件挂载瞬间 `open` 已经是 `true`**。组件内 `watch(() => props.open)` 若不带
`{ immediate: true }`，永远观察不到变更，首开不会加载数据（表格空白且不发请求）。
约定：懒挂载弹窗的 open watch 必须写 `watch(() => props.open, (v) => { if (v) load() }, { immediate: true })`，
加载函数内部自行用 `props.xxx?.id` 等守卫兜住"挂载但未就绪"的场景。

## 四、文档组织约定

1. **新增文档前先查 [文档中心](../README.md)**：按系统菜单找到模块手册，主题已存在则并入，不另起重复文件；
   根 README 与模块 README 只保留导航和运行说明，不再复制架构或 API 契约。
2. 文档按用途归位：
   - `docs/overview/`——项目概览与全局上下文
   - `docs/architecture/`——功能**最终逻辑**（数据流、入口、关键决策、失败语义），不写变更流水账
   - `docs/architecture/modules/`——按系统菜单组织的模块手册，统一使用“重点 → 入口/操作 → 流程图 →
     判断与失败 → 技术细节”。每个模块必须提供可渲染的 Mermaid 流程图；只写当前实现，
     不把未上线方案当成功能。底层架构参考按需链接，不把实现文件名当作主阅读目录。
   - `docs/plans/`——未完成的计划/待办清单（如缺失接口补齐、规模化改造），完成一项更新一项状态，整体完成后归档
   - `docs/archive/`——历史方案、旧实现说明（文件头必须标注"历史归档"）
   - `docs/ops/`——面向部署、验证和排障的操作手册，不复制功能内部设计
3. 变更记录/总结类内容（"本次改了什么"）一律不进 architecture；要么并入对应架构文档的"最终逻辑"，要么进 `docs/archive/`。
4. 后端唯一实现为 Go 版 autoadmin；Django 实现已从版本库移除，文档禁止引用其源码，历史实现只允许引用 `docs/archive/`（见 AGENTS.md）。
5. **数据访问层约定（选 sqlc 还是内联 SQL、字段映射、SQL 方言可移植性）统一见 [SQL_DESIGN.md](SQL_DESIGN.md)**，本文不重复；新增或修改任何 SQL 前先读该文档。

## 五、复制到剪贴板：统一走 `@/util/clipboard`

内网部署多为 **HTTP（非安全上下文）**，此时 `navigator.clipboard` 不存在；直接调
`navigator.clipboard.writeText(...)` 必然抛错，表现为"复制失败，请手动选中复制"
（2026-09-21 日志规则页运行结果复制即此）。约定：所有复制按钮统一用
`import { copyTextWithFallback } from '@/util/clipboard'`——它优先用 Clipboard API，
不可用时回退 `document.execCommand('copy')`；返回布尔值，调用方据此提示成功/失败。


<a id="api-contract"></a>

## 六、API 契约

响应格式以 [API 规则](../../.github/API_RULES.md) 为准；以下补充现有契约及后端实现边界。

### 监控与日志采集的可选列表筛选

两域的可选布尔、整数、字符串及 `search` 条件统一由
`autoadmin/internal/shared/binding/query.go` 解析，返回对应的 `sql.Null*`，
供列表及计数查询复用。参数去除首尾空白；缺失、空白以及无法解析的布尔/整数不启用该筛选。
整数按有符号 64 位十进制解析，零和负数不额外拦截；重复参数取首值。
非空 `search` 两侧加 `%`，不转义输入中的 SQL LIKE 通配符。
这组函数仅用于上述可选筛选，不替代资源 ID、权限条件及其他必须拒绝非法值的参数校验。

### Success

Successful business operations use HTTP 200:

```json
{"code": 200, "msg": "success", "data": {}}
```

### Business errors

Expected business errors retain HTTP 200 and use application codes:

- `300`: account or password error
- `301`: invalid or expired token
- `400`: invalid request
- `403`: permission denied
- `404`: resource not found

Transport, malformed HTTP and unavailable dependency errors may use an appropriate HTTP status, but still use the same envelope.

### Pagination

List endpoints accept `page` and `page_size`. The compatibility default is 10 and the maximum is 30.

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [],
    "count": 0,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 0,
    "next": null,
    "previous": null
  }
}
```

### Compatibility rules

- Preserve existing route prefixes and JSON field names until a coordinated frontend change is approved.
- Preserve JWT permission strings in `module:resource:action` form.
- Keep `/monitor/prometheus/http-sd/` and `/monitor/alert-webhook/api/v2/alerts` as explicitly authenticated machine endpoints according to their current behavior.
- Contract tests pin the frontend-visible response envelope; the authoritative response rules are [API_RULES.md](../../.github/API_RULES.md).


<a id="error-conventions"></a>

## 七、后端错误分层

autoadmin uses three error layers.

### 1. Error mechanism

`internal/shared/apperror/error.go` defines the immutable `Error` type, accessors, `errors.As` support and `WithCause`. It must not contain domain-specific errors.

Repository and infrastructure packages should preserve technical context with `%w`:

```go
return fmt.Errorf("query users: %w", err)
```

These errors are for logs and tracing. They must not be returned directly to clients.

### 2. Common catalog

`internal/shared/apperror/common.go` contains errors whose meaning is identical in every module:

- invalid request, ID and pagination
- invalid or expired token
- permission denied
- generic resource not found
- generic internal failure

Handlers reference these values directly through `response.Error`.

### 3. Module catalogs

Every business module owns an `errors.go`. Examples:

- `identity/errors.go`: invalid credentials, disabled user, user not found
- `rbac/errors.go`: role/menu validation and lookup errors
- `sysconfig/errors.go`: readonly config, missing default, typed-value errors

An error belongs to a module when its wording or code expresses that module's business meaning. Do not duplicate common errors in each module.

### Internal causes

Module catalogs contain stable public errors. Attach an operation failure without mutating the catalog value:

```go
response.Error(ctx, apperror.WithCause(identity.ErrUserQueryInternal, err))
```

The client receives only the stable code/message/status. Logging middleware may inspect `errors.Unwrap` to record the cause after redaction.

### Rules

1. Do not use `errors.New` for API or domain errors.
2. Do not construct code/message pairs inside handlers.
3. Do not return `err.Error()` to clients.
4. Use existing common or module errors before adding a new one.
5. Give new domain modules an `errors.go` as part of their first API slice.
6. Preserve existing frontend application codes during migration.
7. Unknown errors are mapped to common code `600` without exposing the cause.
