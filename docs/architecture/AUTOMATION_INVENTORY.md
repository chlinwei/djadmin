# 自动化 Inventory 管理（autoadmin）

描述 `automation_inventory` 的 CRUD 语义。后端唯一实现为 Go 版 autoadmin（`internal/automation/runtime.go`）。

## API 语义

- `POST /automation/inventories/`：全量创建，`name` 必填；`enabled` 缺省 true，`update_on_launch` 缺省 false，`update_cache_timeout` 缺省 300 且必须非负。
- `PUT/PATCH /automation/inventories/:id/`：**支持部分更新**——未提供的字段保留原值（name/remark/selected_host_ids/enabled/update_on_launch/update_cache_timeout 逐字段合并后整体 UPDATE）。
  - 前端列表"启用状态"开关只发 `{enabled}`，不携带 name，必须走部分更新（此前因全量校验 `name is required` 报错，已修复）。
  - 更新后返回完整资源（`inventoryByID`）。
- `GET`：单条/列表；`DELETE`：物理删除，目标不存在报 404 语义错误。

## 关键决策

- 部分更新在应用层合并（先 SELECT 现值再整体 UPDATE，`resolveInventoryUpdate` 纯函数，见 `runtime_inventory_test.go`），避免动态拼 SET 子句；`selected_host_ids` 以 JSON 数字数组列存储，合并时经 `decodeJSONInt64Array` 解析原值。
- name 语义：显式传空 name 与未传等价，均按"保留原值"处理；仅当现值 name 也为空（脏数据）且未提供 name 时报 400 `name is required`。
- 库中 `update_cache_timeout` 为 NULL 的存量行，合并时回退默认 300。
