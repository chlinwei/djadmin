# 前端多标签页与 KeepAlive 缓存

> 适用范围：`fronted/src/layout/`、`fronted/src/router/index.js`、`fronted/src/store/index.js` 的多标签页浏览与页面缓存逻辑。

## 数据流

1. 菜单点击 / 路由变化 → `store.state.tabs` 追加 `{title, key}`，`key` 为路由 `fullPath`（可能带 query）。
2. `layout/index.vue` 的 `tab_includes` 计算 `KeepAlive` 的 `include` 名单。
3. `layout/tabs/index.vue` 渲染标签，关闭标签时提交 `remove_tab` / `close_all_tabs`，并由 `activeKey` 的 watcher 触发 `router.push`。
4. 路由组件（所有 `views/**` 页面）由 `router/index.js` 的 `stampKeepAliveNames` / `withKeepAliveName` 在懒加载解析后盖上按路由定义路径生成的唯一 `name`，即 `keepAliveNameOf(route.path)`。

## 关键决策

- **唯一组件名**：项目几乎全是 `index.vue` 且未声明 `name`，Vue 会把它们推断成同名 `index`，导致关闭一个标签连带剪掉其他页面缓存。必须按路由路径盖唯一名（`view_<path 归一化>`），使缓存项与 `include` 一一对应。
- **别名归一（关键）**：`alias` 路由在 vue-router 中会生成独立 record，`record.path` 是别名，但它与原名路由共享同一个组件对象，组件名是按原 `route.path` 盖的。取缓存名必须用 `keepAliveNameOfRecord(record)`（内部走 `record.aliasOf.path || record.path`）归一，否则别名页面的 `include` 名与组件名对不上：页面不缓存，且标签增删时会错误 prune 缓存实例，触发 `Cannot set properties of null (setting '__vnode')` / `Cannot read properties of null (reading 'type'/'emitsOptions')`。
- **`include` 组成**：`include = 当前路由缓存名 + 所有标签页缓存名`。`tab.key` 存的是 `fullPath`，取缓存名前先 `router.resolve(path).matched` 解析到路由记录再按上一条归一。
- **稳定引用**：`include` 数组内容未变时必须复用同一引用，否则每次路由跳转都会触发 `KeepAlive.pruneCache` 卸载缓存实例。
- **关闭当前/全部标签的顺序**：`remove_tab` / `close_all_tabs` 会先于 `router.push` 生效。此时当前路由仍是旧页面，`include` 必须继续包含当前路由缓存名，等路由真正切换后再剔除该缓存；否则会卸载正在挂载中的实例。

## 失败语义

`include` 与组件名不匹配时会错误触发 `KeepAlive.pruneCache`，卸载正在挂载或仍在复用的实例，Vue 在后续 patch 时拿到 `el` 为 `null` 的旧 vnode，表现为：

- `Cannot read properties of null (reading 'parentNode')`
- `Cannot set properties of null (setting '__vnode')`（`patchElement`）
- `Cannot read properties of null (reading 'type' / 'emitsOptions')`（`shouldUpdateComponent`）

均伴随 `Unhandled error during execution of component update`。根因是 `include` 名单与实际组件 `name` 不一致（最常见于别名路由未归一），而不是页面本身的数据逻辑。

## 与虚拟滚动（rc-virtual-list）的冲突（2026-09-21）

被 KeepAlive 缓存的页面里，ant-design-vue 4.2.6 的虚拟滚动组件（`a-tree` / `a-select` /
`a-tree-select` 的 `virtual`）存在两个与缓存生命周期相关的上游问题：

- **失活→激活后列表顶部整块空白**：`a-tree` 长时间挂载，页面失活时容器高度变 0，激活后
  rc-virtual-list 内部 `scrollTop`/偏移未复位，从中间下标开始渲染，前面留出空白。
  已在所有资产页共用的 `ServiceTree.vue` 上以 `:virtual="false"` 修复（保留 `:height` 做固定高度滚动）。
- **`ScrollBar` 卸载竞态**：`vc-virtual-list/ScrollBar.js` 的 `beforeUnmount → removeEvents`
  读 `scrollbarRef.current` 未做空判断，缓存实例被 `pruneCache` 卸载时可能抛
  `Cannot read properties of null (reading 'removeEventListener')`。`userCenter` 时区下拉即因此禁用虚拟。

**约定**：凡是选项数量有界（固定枚举、项目 / 环境 / 模板 / 角色 / 用户组 / 菜单等，量级数十到数百）
的 `a-select` / `a-tree-select`，统一加 `:virtual="false"` 规避上述问题；**真·大数据量**
（全部主机 / 全部部署实例 / 全部用户等可能上千项）保留虚拟滚动，避免一次性渲染导致卡顿。
`a-tree` 只有显式传 `:height` 才会启用虚拟滚动（`vc-virtual-list` 要求 `height`），因此不带
`:height` 的树本就不受影响；带 `:height` 的树按上一条禁用。

## 维护约定

- 新增页面无需声明 `name`，缓存名由路由自动生成；不要手写 `include`。
- 新增加路由时确保走 `stampKeepAliveNames`（静态表）或 `withKeepAliveName`（动态菜单路由），否则缓存名与 `include` 不匹配，页面不会被缓存。
- 新增/修改 `a-select` / `a-tree-select` 时按上一节判断是否需要 `:virtual="false"`。
