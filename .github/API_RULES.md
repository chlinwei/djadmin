# API 响应格式规则

## 统一响应格式

所有 API 响应必须遵循以下统一格式，无论成功还是失败。

### 成功响应 (HTTP 200)

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    // 实际响应数据
  }
}
```

### 分页响应格式

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

## 状态码规定

| 状态码 | 说明 | 用途 |
|-------|------|------|
| 200 | 成功 | 所有成功的操作（列表、创建、更新、删除、登录等） |
| 300 | 账号/密码错误 | 登录凭证无效 |
| 301 | Token 验证失败 | 登录过期、Token 无效或过期 |
| 其他 | 各种错误 | msg 字段描述具体错误 |

## 实现要求

### 后端 (Django/DRF)

1. **统一返回函数** - 所有 API 响应必须调用 `utils.py` 中的 `Response_xxx` 函数：
   - `Response_200(data=None, msg="success")` - 成功响应
   - `Response_error(error: ErrorMixin, data=None)` - ErrorMixin 错误响应
   - `Response_error_str(msg, code=400, data=None)` - 自定义错误响应
   - `Response_djerror(djerror: DjadminException, data=None)` - DjadminException 错误响应
2. **响应渲染器** - `djResponseRender.py` 的 `DjAdminResponse_render` 必须确保所有响应返回完整值
3. **分页配置** - `utils.py` 的 `CustomPagination` 必须：
   - 设置 `page_size_query_param = 'page_size'` 以匹配前端参数
   - 实现 `get_paginated_response()` 返回上述结构
4. **异常处理** - `djException.py` 的 `djadmin_handler` 必须将所有异常转换为统一格式
5. **ViewSet 配置** - 所有列表 ViewSet 必须声明 `pagination_class = CustomPagination`

### 前端 (Vue 3)

1. **请求参数** - 分页时必须发送 `page` 和 `page_size` 参数
2. **响应处理** - 必须：
   - 验证 `response.data.code === 200` 
   - 特殊处理 code 301（重新登录）
   - 提取实际数据：`response.data.data`
3. **错误处理** - 显示 `response.data.msg` 给用户
4. **防御编码** - 使用可选链操作符 `?.` 处理可能缺失的字段

## 常见错误

### ❌ 错误做法

```python
# 直接返回Response对象（错误 - 应该调用Response_xxx）
from rest_framework.response import Response
return Response(data)

# 返回原始JsonResponse（错误 - 应该调用Response_xxx）
from django.http import JsonResponse
return JsonResponse(data)

# 异常处理时忘记格式化错误响应（错误）
try:
    # 业务逻辑
except Exception as e:
    return JsonResponse({'error': str(e)})  # 应该调用 Response_djerror
```

```json
// 分页返回原始数组（错误）
[{"id": 1, "name": "task"}]

// 忘记包装成 data 字段（错误）
{
  "code": 200,
  "results": [...],
  "count": 1
}

// 成功操作返回非 200 状态码（错误）
{
  "code": 201,
  "msg": "created",
  "data": {...}
}
```

### ✅ 正确做法

```python
# 使用Response_xxx函数进行返回（正确）
from djadmin.utils import Response_200, Response_error, Response_djerror
from djadmin.djException import DjadminException

# 成功响应
return Response_200(data={'id': 1, 'name': 'task'})

# 错误响应 - ErrorMixin
try:
    # 业务逻辑
except ValueError as e:
    return Response_error(ErrorMixin_instance)

# 错误响应 - DjadminException
try:
    # 业务逻辑
except DjadminException as e:
    return Response_djerror(e)

# 自定义错误响应
return Response_error_str('操作失败', code=400)
```

```json
// 分页返回正确格式
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [{"id": 1, "name": "task"}],
    "count": 1,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 1,
    "next": null,
    "previous": null
  }
}

// 所有成功操作返回 200
{
  "code": 200,
  "msg": "success",
  "data": {"id": 1, "name": "created task"}
}
```

## 关键参数匹配表

| 前端参数 | 后端参数名 | 说明 |
|---------|----------|------|
| page | page_size_query_param | 分页数，从 1 开始 |
| page_size | page_size_query_param | 每页条数，默认 10 |

> **注意**：前端发送 `page_size`，后端 CustomPagination 的 `page_size_query_param` 必须设置为 `'page_size'`，否则分页失效。

## Response_xxx 函数详解

所有 API 响应必须通过以下函数进行返回。这些函数定义在 `utils.py` 中：

### 1. Response_200(data=None, msg="success")
**用途**：返回成功响应  
**参数**：
- `data` - 响应数据，可为任何类型（dict, list, str等）
- `msg` - 成功消息（默认 "success"）

**示例**：
```python
from djadmin.utils import Response_200

# 返回单个对象
return Response_200(data={'id': 1, 'name': 'task'})

# 返回列表
return Response_200(data=[{'id': 1}, {'id': 2}])

# 返回无数据的成功响应
return Response_200()
```

### 2. Response_error(error: ErrorMixin, data=None)
**用途**：返回 ErrorMixin 类型的错误响应  
**参数**：
- `error` - ErrorMixin 实例，包含 code 和 msg
- `data` - 额外数据（可选）

**示例**：
```python
from djadmin.utils import Response_error
from djadmin.errordict import SOME_ERROR_MIXIN

try:
    # 业务逻辑
except ValueError as e:
    return Response_error(SOME_ERROR_MIXIN)
```

### 3. Response_error_str(msg, code=400, data=None)
**用途**：返回自定义错误响应  
**参数**：
- `msg` - 错误消息
- `code` - 错误代码（默认 400）
- `data` - 额外数据（可选）

**示例**：
```python
from djadmin.utils import Response_error_str

# 自定义错误
return Response_error_str('用户不存在', code=404)

# 验证失败
return Response_error_str('用户名不能为空', code=400)
```

### 4. Response_djerror(djerror: DjadminException, data=None)
**用途**：返回 DjadminException 错误响应  
**参数**：
- `djerror` - DjadminException 实例
- `data` - 额外数据（可选）

**示例**：
```python
from djadmin.utils import Response_djerror
from djadmin.djException import DjadminException

try:
    # 业务逻辑
except DjadminException as e:
    return Response_djerror(e)
```

## 测试检查清单

- [ ] 所有 API 返回都调用了 `Response_xxx` 函数
- [ ] 成功返回使用 `Response_200()`
- [ ] 异常返回使用相应的 `Response_error()`、`Response_error_str()` 或 `Response_djerror()`
- [ ] 登录返回 code 200 和有效 token
- [ ] 列表接口返回分页格式的 data 字段
- [ ] 创建/更新/删除操作返回 code 200
- [ ] 异常返回 code 300/301 和错误消息
- [ ] 前端能正确解析 response.data.data
- [ ] 前端分页参数与后端配置匹配
