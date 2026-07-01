# 测试报告

- **生成时间**: 2026-06-30 18:23:09
- **整体结果**: ✅ 全部通过
- **总耗时**: 23.803 秒

## 汇总

| 指标 | 数量 |
| --- | --- |
| 总用例数 | 54 |
| ✅ 通过 | 54 |
| ❌ 失败 | 0 |
| 🔥 错误 | 0 |
| ⏭️ 跳过 | 0 |
| 通过率 | 100.0% |

## 模块汇总

| 模块 | 用例数 | 耗时(秒) |
| --- | --- | --- |
| assets | 23 | 21.943 |
| role | 8 | 0.153 |
| user | 23 | 0.840 |

## 用例明细

| 模块 | 测试类 | 用例 | 说明 | 耗时(秒) | 结果 |
| --- | --- | --- | --- | --- | --- |
| assets | ApplicationTest | test_batch_delete_applications | 批量删除应用 | 0.017 | ✅ 通过 |
| assets | ApplicationTest | test_create_application | 新增应用 | 0.016 | ✅ 通过 |
| assets | ApplicationTest | test_list_applications | 应用列表返回分页格式 | 0.019 | ✅ 通过 |
| assets | CredentialTest | test_batch_delete_credentials | 批量删除凭证 | 0.021 | ✅ 通过 |
| assets | CredentialTest | test_create_credential | 新增凭证 | 0.012 | ✅ 通过 |
| assets | CredentialTest | test_get_credential_detail | 获取凭证详情格式正确 | 0.012 | ✅ 通过 |
| assets | CredentialTest | test_list_credentials_paginated | 凭证列表返回分页格式 | 0.012 | ✅ 通过 |
| assets | CredentialTest | test_list_without_token | 无 token 返回 301 | 0.004 | ✅ 通过 |
| assets | CredentialTest | test_search_credentials | 搜索凭证 | 0.017 | ✅ 通过 |
| assets | CredentialTest | test_update_credential | 编辑凭证 | 0.018 | ✅ 通过 |
| assets | HostCollectTest | test_collect_host_with_invalid_connection | 采集主机时凭证错误应该返回 failed 状态 | 21.082 | ✅ 通过 |
| assets | HostCollectTest | test_collect_host_without_credential | 采集主机时没有配置凭证应该返回 failed 状态 | 0.051 | ✅ 通过 |
| assets | HostGroupTest | test_create_host_group | 新增主机分组 | 0.024 | ✅ 通过 |
| assets | HostGroupTest | test_create_nested_group | 创建嵌套子分组 | 0.065 | ✅ 通过 |
| assets | HostGroupTest | test_delete_host_group | 删除主机分组 | 0.022 | ✅ 通过 |
| assets | HostGroupTest | test_get_host_group_detail | 获取主机分组详情 | 0.020 | ✅ 通过 |
| assets | HostGroupTest | test_get_host_group_tree | 获取分组树形结构 | 0.044 | ✅ 通过 |
| assets | HostGroupTest | test_list_host_groups | 主机分组列表 | 0.030 | ✅ 通过 |
| assets | HostGroupTest | test_update_host_group | 编辑主机分组 | 0.254 | ✅ 通过 |
| assets | HostTest | test_create_host | 新增主机 | 0.046 | ✅ 通过 |
| assets | HostTest | test_get_host_detail | 获取主机详情 | 0.043 | ✅ 通过 |
| assets | HostTest | test_list_hosts | 主机列表返回分页格式 | 0.047 | ✅ 通过 |
| assets | HostTest | test_update_host | 编辑主机后返回完整主机信息 | 0.066 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_cascades_user_role | 删除角色时级联删除用户-角色关联 | 0.043 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_empty_ids | 批量删除不传 ids 应报错 | 0.006 | ✅ 通过 |
| role | RoleManageTest | test_batch_delete_roles | 批量删除角色 | 0.028 | ✅ 通过 |
| role | RoleManageTest | test_create_role | 新增角色 | 0.010 | ✅ 通过 |
| role | RoleManageTest | test_get_current_user_role_list | 获取当前用户角色列表 | 0.017 | ✅ 通过 |
| role | RoleManageTest | test_get_role_detail | 获取角色详情 | 0.011 | ✅ 通过 |
| role | RoleManageTest | test_list_roles | 角色列表返回分页格式 | 0.015 | ✅ 通过 |
| role | RoleManageTest | test_update_role | 编辑角色 | 0.024 | ✅ 通过 |
| user | LoginViewTest | test_login_disabled_user | 禁用用户不能登录 | 0.453 | ✅ 通过 |
| user | LoginViewTest | test_login_response_format | 验证响应格式有 code/msg/data 三个字段 | 0.020 | ✅ 通过 |
| user | LoginViewTest | test_login_success | 正常登录返回 token | 0.028 | ✅ 通过 |
| user | LoginViewTest | test_login_user_not_exist | 用户不存在返回 code=300 | 0.009 | ✅ 通过 |
| user | LoginViewTest | test_login_wrong_password | 密码错误返回 code=300 | 0.011 | ✅ 通过 |
| user | UserCenterTest | test_update_password_success | 修改密码成功 | 0.014 | ✅ 通过 |
| user | UserCenterTest | test_update_password_wrong_old | 旧密码错误修改失败 | 0.008 | ✅ 通过 |
| user | UserCenterTest | test_update_user_info | 更新个人信息 | 0.013 | ✅ 通过 |
| user | UserManageTest | test_assign_roles | 分配用户角色 | 0.021 | ✅ 通过 |
| user | UserManageTest | test_batch_delete_empty_ids | 批量删除不传 ids 应报错 | 0.006 | ✅ 通过 |
| user | UserManageTest | test_batch_delete_users | 批量删除用户 | 0.034 | ✅ 通过 |
| user | UserManageTest | test_change_user_status | 修改用户状态为禁用 | 0.025 | ✅ 通过 |
| user | UserManageTest | test_check_username_exists | 检查用户名存在 | 0.013 | ✅ 通过 |
| user | UserManageTest | test_check_username_not_exists | 检查用户名不存在 | 0.011 | ✅ 通过 |
| user | UserManageTest | test_create_user | 新增用户 | 0.020 | ✅ 通过 |
| user | UserManageTest | test_create_user_duplicate_username | 重复用户名应失败 | 0.030 | ✅ 通过 |
| user | UserManageTest | test_get_current_user | 获取当前用户信息 | 0.015 | ✅ 通过 |
| user | UserManageTest | test_get_user_detail | 获取用户详情 | 0.014 | ✅ 通过 |
| user | UserManageTest | test_get_user_roles | 获取用户角色列表 | 0.021 | ✅ 通过 |
| user | UserManageTest | test_list_users_returns_paginated | 用户列表返回分页格式 | 0.026 | ✅ 通过 |
| user | UserManageTest | test_list_users_without_token_returns_301 | 无 token 访问返回 301 | 0.005 | ✅ 通过 |
| user | UserManageTest | test_reset_password | 重置用户密码为 123456 | 0.019 | ✅ 通过 |
| user | UserManageTest | test_update_user | 编辑用户 | 0.024 | ✅ 通过 |
