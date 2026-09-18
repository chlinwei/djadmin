# 开发与设计约定（强制）

> 本文是项目级约定的**唯一汇总入口**，避免规范碎片化。新增约定先查本文，属同类主题的并入对应章节，不要另起新文件。违反约定的代码在 review 时直接打回。
> API 响应体格式另见 [.github/API_RULES.md](../../.github/API_RULES.md)。

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

## 三、懒挂载弹窗的数据加载

父组件用 `v-if` + `open` 懒挂载的弹窗（关闭延迟卸载动画结束后 `v-if` 置 false 的模式），
**首次打开时组件挂载瞬间 `open` 已经是 `true`**。组件内 `watch(() => props.open)` 若不带
`{ immediate: true }`，永远观察不到变更，首开不会加载数据（表格空白且不发请求）。
约定：懒挂载弹窗的 open watch 必须写 `watch(() => props.open, (v) => { if (v) load() }, { immediate: true })`，
加载函数内部自行用 `props.xxx?.id` 等守卫兜住"挂载但未就绪"的场景。

## 四、文档组织约定

1. **新增文档前先查 `README.md` 索引**：主题已存在则并入，不另起新文件；确需新建的放对目录（见 README 分类）。
2. 文档分四类，各归其位：
   - `docs/overview/`——项目概览与全局上下文
   - `docs/architecture/`——功能**最终逻辑**（数据流、入口、关键决策、失败语义），不写变更流水账
   - `docs/plans/`——未完成的计划/待办清单（如缺失接口补齐、规模化改造），完成一项更新一项状态，整体完成后归档
   - `docs/archive/`——历史方案、旧实现说明（文件头必须标注"历史归档"）
3. 变更记录/总结类内容（"本次改了什么"）一律不进 architecture；要么并入对应架构文档的"最终逻辑"，要么进 `docs/archive/`。
4. 后端唯一实现为 Go 版 autoadmin；文档禁止引用 `backend/` 源码（见 AGENTS.md）。
5. **数据访问层约定（选 sqlc 还是内联 SQL、字段映射、SQL 方言可移植性）统一见 [SQL_DESIGN.md](SQL_DESIGN.md)**，本文不重复；新增或修改任何 SQL 前先读该文档。
