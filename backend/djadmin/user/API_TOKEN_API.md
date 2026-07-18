# ApiToken API 对接文档

本文档用于前端和 Agent 侧对接 djadmin 的 ApiToken 能力。

## 统一响应格式

成功与失败都遵循统一外层结构。

成功示例：
{
  "code": 200,
  "msg": "success",
  "data": {}
}

失败示例：
{
  "code": 400,
  "msg": "错误信息",
  "data": {}
}

认证失败示例：
{
  "code": 301,
  "msg": "Token验证失败！",
  "data": {}
}

## 字段说明

ApiToken 关键字段：
- agent_id: Token 绑定标识，唯一
- bind_mode: 绑定模式
  - agent: Agent共享模式，对应全局共享 token，agent_id 固定为 global
  - api: Api绑定模式，必须绑定具体 agent_id
- token_hash: 仅后端保存哈希值，不会返回明文
- is_active: 是否启用
- expires_at: 过期时间，可为空
- last_used_at: 最后使用时间

## 管理接口（用户中心）

接口前缀：/sys/usercenter/

这些接口都要求登录态 JWT（HTTP_AUTHORIZATION）。

### 1) 获取 ApiToken 列表

- Method: GET
- URL: /sys/usercenter/apiTokens/

返回 data 结构：
{
  "results": [
    {
      "id": 1,
      "agent_id": "global",
      "bind_mode": "agent",
      "name": "全局共享令牌",
      "is_active": true,
      "expires_at": null,
      "last_used_at": null,
      "created_by": 1,
      "created_by_username": "admin",
      "remark": "",
      "create_time": "2026-07-18T10:00:00Z",
      "update_time": "2026-07-18T10:00:00Z"
    }
  ],
  "count": 1
}

### 2) 创建 ApiToken

- Method: POST
- URL: /sys/usercenter/createApiToken/
- Body(JSON):
  - bind_mode: api 或 agent
  - agent_id: api 模式必填；agent 模式可不传（会固定成 global）
  - name: 可选
  - is_active: 可选，默认 true
  - expires_at: 可选，ISO 时间格式，且必须晚于当前时间
  - remark: 可选

agent 请求示例：
{
  "bind_mode": "agent",
  "name": "全局共享令牌"
}

api 请求示例：
{
  "bind_mode": "api",
  "agent_id": "agent-001",
  "name": "主机A令牌"
}

成功返回 data：
{
  "id": 1,
  "agent_id": "global",
  "bind_mode": "agent",
  "token": "明文token仅返回一次",
  "expires_at": null,
  "is_active": true
}

典型失败信息：
- bind_mode仅支持api或agent
- api模式下agent_id不能为空
- agent_id不能为global保留字
- agent_id已存在
- expires_at格式错误，需使用ISO时间格式
- expires_at必须晚于当前时间

### 3) 轮换 ApiToken

- Method: POST
- URL: /sys/usercenter/rotateApiToken/
- Body(JSON):
{
  "id": 1
}

成功返回 data：
{
  "agent_id": "agent-001",
  "bind_mode": "api",
  "token": "新明文token仅返回一次"
}

典型失败信息：
- id不能为空
- ApiToken不存在
- ApiToken已禁用

### 4) 禁用 ApiToken

- Method: POST
- URL: /sys/usercenter/disableApiToken/
- Body(JSON):
{
  "id": 1
}

成功返回 data：
{
  "agent_id": "agent-001",
  "bind_mode": "api",
  "is_active": false
}

典型失败信息：
- id不能为空
- ApiToken不存在
- ApiToken已禁用

### 5) 删除 ApiToken

- Method: POST
- URL: /sys/usercenter/deleteApiToken/
- Body(JSON):
{
  "id": 1
}

成功返回 data：
{
  "agent_id": "agent-001",
  "bind_mode": "api",
  "deleted": true
}

典型失败信息：
- id不能为空
- ApiToken不存在

## Agent 鉴权说明

以下路径按 Api Token 鉴权，不走用户 JWT：
- /api/agent/
- /sys/agent/

请求头：
- HTTP_AUTHORIZATION: 直接传 token 明文，不带 Bearer 前缀

鉴权逻辑：
- token 为空: 返回 code=301，msg=Api Token不能为空
- token 不匹配/过期/未启用: 返回 code=301，msg=Api Token验证失败！
- 通过后中间件会在请求上下文挂载：
  - request.agent_id
  - request.bind_mode

## 前端对接建议

- 创建/轮换接口返回的 token 明文只出现一次，前端拿到后应立即提示复制并安全保存。
- api 与 agent 建议在 UI 上做互斥选择：
  - agent: 禁用 agent_id 输入
  - api: 强制填写 agent_id
- 对于错误提示，优先展示后端 msg 原文，减少二次翻译偏差。
