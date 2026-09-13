# Plan A 实现总结 - 告警媒介收件人配置

## 概述
实现了 Zabbix 风格的告警媒介体系，用户可以在创建/编辑媒介时直接配置接收告警的邮箱地址，完整支持从路由匹配到邮件发送的全链路。

---

## 后端实现

### 1. 数据库迁移
**文件**: `backend/djadmin/monitor/migrations/0047_alertmedia_recipients.py`
- 添加 `recipients` JSON 字段到 `AlertMedia` 表
- 支持存储多个收件人邮箱

### 2. 数据模型更新
**文件**: `backend/djadmin/monitor/models.py`
```python
class AlertMedia(BaseModel):
    name = models.CharField(max_length=128)
    media_type = models.CharField(max_length=16, choices=MediaType.choices)
    config = models.JSONField(default=dict, blank=True)  # SMTP 配置
    recipients = models.JSONField(default=list, blank=True)  # 收件人邮箱列表
    enabled = models.BooleanField(default=True)
```

### 3. 序列化器增强
**文件**: `backend/djadmin/monitor/serializer.py`
- `AlertMediaSerializer` 新增 `recipients` 字段处理
- 验证逻辑：Email 类型媒介必须至少有一个收件人
- `_normalize_recipients()` 方法：
  - 支持字符串输入（逗号/分号分隔）
  - 支持列表输入
  - 自动验证邮箱格式（包含 @）
  - 去重

### 4. 告警发送任务
**文件**: `backend/djadmin/monitor/tasks.py`

#### 关键改动：
- `_send_email_alert(media, alert, recipients)`: 新增邮件发送辅助函数
  - 从 `AlertMedia.recipients` 读取收件人
  - 使用 Django `EmailMultiAlternatives` 发送
  - 支持 HTML 和文本格式
  - 完整的错误处理和日志

- `send_alert_notification(event_id)`: 完整实现告警发送链路
  - 路由匹配 → 媒介筛选 → 邮件发送
  - 支持 Celery 重试机制（指数退避）
  - 记录发送状态和错误信息
  - 最多重试 5 次

### 5. 单元测试更新
**文件**: `backend/djadmin/monitor/test_notifications.py`
- 更新 `_create_media()` 支持 `recipients` 参数
- 更新测试用例验证收件人验证逻辑

---

## 前端实现

### 1. 媒介管理页面更新
**文件**: `fronted/src/views/monitor/media/index.vue`

#### 新增表单字段
- **收件人邮箱**：Textarea 输入框
  - 支持多种分隔符：`,`、`;`、`,，`、`；` 和 换行
  - 实时提示格式示例
  - 前端验证：至少一个有效邮箱（包含 @）

#### 表格列更新
- 新增"收件人"列（250px 宽），显示逗号分隔的邮箱列表
- 调整其他列宽以容纳新列

#### 编辑时数据恢复
```javascript
// 从 recipients 数组恢复为文本格式
recipientsText: Array.isArray(recipients) 
  ? recipients.join(', ') 
  : recipients || ''
```

#### 保存时数据转换
```javascript
// 将文本格式转换为数组，支持多种分隔符
const recipients = createForm.value.recipientsText
  .split(/[,;，；\n]/)
  .map((item) => item.trim())
  .filter((item) => item.length > 0 && item.includes('@'))
```

---

## 完整工作流

### 创建告警媒介流程
```
1. 用户点击"新增" → 打开表单弹框
2. 填写媒介名称、SMTP 配置、收件人邮箱
3. 点击保存 → 发送 POST /monitor/media/
   - 后端校验收件人至少 1 个
   - 保存到 AlertMedia.recipients
4. 成功提示 → 刷新列表
```

### 告警发送流程
```
1. Prometheus → webhook → AlertHistory / AlertNotificationEvent 创建
2. Celery 任务触发 send_alert_notification
3. 路由匹配：
   - 遍历启用的 AlertRoute
   - 按 severity/alertname/instance 标签匹配
   - 收集命中的 AlertMedia
4. 邮件发送：
   - 遍历匹配的媒介
   - 读取 media.recipients
   - 构建邮件内容（告警名、严重级别、标签等）
   - 通过 SMTP 发送
5. 记录投递结果 → AlertNotificationEvent.status 更新
```

---

## API 数据结构

### 新增/编辑媒介请求
```json
{
  "name": "运维邮箱通知",
  "media_type": "email",
  "enabled": true,
  "recipients": ["ops@example.com", "admin@example.com"],
  "config": {
    "provider": "custom",
    "smtpServer": "smtp.example.com",
    "smtpPort": 587,
    "authType": "password",
    "email": "sender@example.com",
    "username": "user@example.com",
    "password": "encrypted_password",
    "messageFormat": "html"
  },
  "remark": "主要运维团队通知"
}
```

### 获取媒介列表响应
```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [
      {
        "id": 1,
        "name": "运维邮箱通知",
        "media_type": "email",
        "recipients": ["ops@example.com", "admin@example.com"],
        "config": { ... },
        "enabled": true,
        "remark": "...",
        "create_time": "2026-08-20T10:00:00Z",
        "update_time": "2026-08-20T10:00:00Z"
      }
    ],
    "count": 1,
    "pageNumber": 1,
    "pageSize": 100
  }
}
```

---

## 关键特性

✅ **完整链路实现**
- 路由 → 媒介 → 收件人 → 发送

✅ **灵活的收件人配置**
- 支持多格式输入（逗号、分号、换行）
- 前后端双重验证
- 自动去重

✅ **Celery 重试机制**
- 失败自动重试最多 5 次
- 指数退避算法：2^n * 10 秒（最多 300 秒）

✅ **完整的错误日志**
- 记录发送失败原因
- 便于问题排查和审计

✅ **易用的用户界面**
- 直观的表单设计
- 实时反馈和验证
- 表格中显示收件人信息

---

## 验证清单

### 后端验证
- [ ] 迁移运行成功：`python manage.py migrate monitor`
- [ ] Pylance 无类型错误
- [ ] 单元测试通过：`python manage.py test monitor --settings=djadmin.test_settings`

### 前端验证
- [ ] 新增媒介表单显示收件人字段
- [ ] 收件人输入支持多种分隔符
- [ ] 编辑时收件人数据正确恢复
- [ ] 表格显示收件人列
- [ ] 保存前验证：至少一个有效邮箱

### 集成验证
- [ ] 新增媒介成功保存
- [ ] 路由绑定媒介后，告警能正确匹配
- [ ] Celery worker 接收到任务
- [ ] 邮件成功发送到配置的收件人
- [ ] 发送失败时有详细错误日志

---

## 后续可扩展方向

1. **Webhook 媒介支持**：当前仅实现 Email，可补充 Webhook POST 功能
2. **钉钉/企业微信集成**：添加专有媒介类型
3. **邮件模板自定义**：当前使用固定格式，可支持用户自定义模板
4. **批量操作**：支持批量启用/禁用/删除媒介
5. **投递审计日志**：详细记录每次投递的时间、内容、结果

---

## 依赖库
- Django 3.2+
- Django REST Framework
- Celery 5.0+
- django.core.mail
- cryptography（用于密码加密）

