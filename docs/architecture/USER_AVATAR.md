# 用户头像与 media 静态文件（autoadmin）

> 适用范围：`autoadmin/internal/identity/avatar.go`、`internal/api/router/router.go` 的 `/media` 路由、
> `fronted/src/views/userCenter/components/Avatar.vue`、`fronted/src/layout/header/components/avatar.vue`。

## 数据与接口

- 头像文件名存在 `sys_user.avatar`（`varchar(255)`，只存文件名，如 `20250918123000.jpg`）。
- 上传：`POST /user/changeAvatar`（登录即可，`multipart` 字段名 `avatar`）。
- 展示：静态文件服务 `GET /media/*`，根目录为进程工作目录下的 `media/`（即 `autoadmin/media`）。

## 上传链路（`ChangeAvatar`）

1. 从 token 取当前用户；请求体限制 5MB（在解析 multipart 前用 `http.MaxBytesReader`）。
2. 后缀白名单 `png/jpg/jpeg/gif/webp`，其余 400。
3. 文件写 `media/userAvatar/<时间戳><后缀>`；写盘失败清理残留并 400。
4. 成功落库：`UpdateUserAvatar` 写 `sys_user.avatar`；落库失败则删除刚落的新文件并报错。
5. 删除该用户旧头像文件（失败不影响上传结论）。
6. 返回 `{new_file_name, avatar, avatar_url}`，`avatar_url` 形如 `/media/userAvatar/<file>`（相对地址）。

## 静态文件服务

- `router.NewWithGateway` 注册 `engine.Static("/media", <cwd>/media)`，匿名可访问；目录不存在时返回 404。
- 路径穿越由 `http.Dir` 拦截；`audit` 中间件已跳过 `/media` 与 `/static` 前缀，静态请求不落审计。
- 前端 `getMediaUrl(file)` 统一把文件名/相对路径拼成 `getServerUrl() + /media/...`（开发环境后端在 9000 端口，
  生产可同源或由 nginx 转发 `/media`）。

## 前端

- `userCenter/components/Avatar.vue`：进入时用本地缓存的 `currentUser.avatar` 回显；上传成功后把响应里的
  `avatar` 写回 localStorage 的 `currentUser`，并即时刷新预览。
- `layout/header/components/avatar.vue`：顶栏头像用 `currentUser.avatar` 显示，无头像时回退 `SvgIcon user`；
  本地缓存是 setup 时读取，上传后需刷新页面（或重新登录）顶栏才更新。

## 与 Django 的差异

- 旧 Django `changeAvatar` 只存文件、只返回文件名，不更新 `sys_user.avatar`，头像实际不可用。
- Go 版在保存文件后**写入 `sys_user.avatar` 并删除旧文件**，配合 `/media` 静态服务形成完整可显示链路。

## 失败语义

- 未登录：401。
- 无文件 / 后缀不在白名单 / 超 5MB：400，文案见实现。
- 落库失败：删除新文件并 500，不留下孤儿文件。
