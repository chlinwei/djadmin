<template>
  <div class="user-group-page">
    <div class="user-group-layout">
      <!-- 左：组列表 -->
      <a-card title="用户组" size="small" class="group-panel">
        <template #extra>
          <a-button v-if="canCreate" size="small" type="primary" @click="openCreateModal">
            <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
            <span>&nbsp;新增</span>
          </a-button>
        </template>
        <a-spin :spinning="listLoading">
          <a-empty v-if="!listLoading && !groupList.length" description="暂无用户组" />
          <a-menu v-else mode="inline" :selected-keys="selectedKeys" @click="handleSelectGroup">
            <a-menu-item v-for="group in groupList" :key="group.id">
              <div class="group-menu-item">
                <span class="group-name">{{ group.name }}</span>
                <a-badge :count="group.member_count" :number-style="{ backgroundColor: '#e6f4ff', color: '#1677ff' }" />
              </div>
            </a-menu-item>
          </a-menu>
        </a-spin>
      </a-card>

      <!-- 右：成员管理 -->
      <a-card size="small" class="member-panel">
        <template #title>
          <a-space wrap>
            <span>{{ currentGroup ? currentGroup.name : '成员' }}</span>
            <a-button v-if="currentGroup && canUpdate" size="small" @click="openEditModal(currentGroup)">编辑组信息</a-button>
          </a-space>
        </template>
        <template #extra>
          <a-button v-if="currentGroup && canDelete" size="small" danger @click="handleDelete(currentGroup)">删除组</a-button>
        </template>

        <a-empty v-if="!currentGroup" description="在左侧选择一个用户组" />
        <template v-else>
          <div v-if="canAddMembers" class="member-add-row">
            <a-select
              v-model:value="memberToAdd"
              show-search
              allow-clear
              :options="addableUserOptions"
              :filter-option="filterUserOption"
              :getPopupContainer="getPopupContainer"
              placeholder="选择要添加的平台用户"
              style="flex: 1"
            />
            <a-button type="primary" :disabled="!memberToAdd" :loading="memberSaving" @click="addMember">添加</a-button>
          </div>

          <a-table
            rowKey="user_id"
            :columns="memberColumns"
            :data-source="currentGroup.members"
            :loading="memberSaving"
            size="small"
            :locale="tableLocale"
            :pagination="false"
            :scroll="{ x: 640 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'operation'">
                <a-tooltip title="移出本组" placement="top">
                  <a-button size="small" type="primary" danger class="delBtn" @click="removeMember(record)">移出</a-button>
                </a-tooltip>
              </template>
            </template>
          </a-table>
        </template>
      </a-card>
    </div>

    <!-- 新增/编辑组（组名、描述） -->
    <a-modal
      v-model:open="modalVisible"
      :title="editingId ? '编辑用户组' : '新增用户组'"
      :footer="null"
      @cancel="closeModal"
    >
      <a-form :label-col="{ span: 4 }" :wrapper-col="{ span: 19 }">
        <a-form-item label="组名" required>
          <a-input v-model:value="groupForm.name" placeholder="例如：告警值班-后端" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="groupForm.remark" :rows="2" />
        </a-form-item>
      </a-form>
      <div class="modal-actions">
        <a-button @click="closeModal">取消</a-button>
        <a-button type="primary" :loading="saveLoading" @click="saveGroup">保存</a-button>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { tableLocale } from '@/util/tableStyle'
import { message } from 'ant-design-vue'
import {
  batchDeleteUserGroups,
  createUserGroup,
  getUserGroupList,
  updateUserGroup,
} from '@/api/userGroup'
import { getUserList } from '@/api/user/index.js'
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { fetchAllPages } from '@/util/fetchAllPages'
import { checkPermission } from '@/directives/permission/permission'

defineOptions({
  name: 'UserGroupsPage',
})

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)

const canCreate = computed(() => checkPermission('system:usergroups:create'))
const canUpdate = computed(() => checkPermission('system:usergroups:update'))
const canDelete = computed(() => checkPermission('system:usergroups:delete'))
const canAddMembers = computed(() => canUpdate.value && checkPermission('system:users:view'))
const memberColumns = computed(() => [
  { title: '用户', dataIndex: 'username', key: 'username' },
  ...(canUpdate.value ? [{ title: '操作', key: 'operation', width: 100, fixed: 'right' }] : []),
])

const groupList = ref([])
const userList = ref([])
const listLoading = ref(false)
const saveLoading = ref(false)
const memberSaving = ref(false)
const selectedGroupId = ref(null)
const memberToAdd = ref(null)
const modalVisible = ref(false)
const editingId = ref(null)
const groupForm = ref({ name: '', remark: '' })

const currentGroup = computed(() => groupList.value.find((group) => group.id === selectedGroupId.value) || null)
const selectedKeys = computed(() => (selectedGroupId.value ? [selectedGroupId.value] : []))

// 成员添加候选：全量用户 - 已在组内的成员。
const addableUserOptions = computed(() => {
  const memberIDs = new Set((currentGroup.value?.members || []).map((member) => member.user_id))
  return userList.value
    .filter((user) => !memberIDs.has(user.id))
    .map((user) => ({ label: `${user.username}（#${user.id}）`, value: user.id }))
})

function parseApiData(response) {
  return response?.data?.data || {}
}

function filterUserOption(input, option) {
  return option.label.toLowerCase().includes(input.toLowerCase())
}

function handleSelectGroup({ key }) {
  selectedGroupId.value = key
  memberToAdd.value = null
}

// 确认框打开后权限可能变化，隐藏入口不能替代提交前的动作校验。
function requireAction(permission) {
  if (checkPermission(permission)) return true
  message.warning('无权执行此操作，请检查用户组操作权限')
  return false
}

function openCreateModal() {
  if (!requireAction('system:usergroups:create')) return
  editingId.value = null
  groupForm.value = { name: '', remark: '' }
  modalVisible.value = true
}

function openEditModal(record) {
  if (!requireAction('system:usergroups:update')) return
  editingId.value = record.id
  groupForm.value = { name: record.name, remark: record.remark || '' }
  modalVisible.value = true
}

function closeModal() {
  modalVisible.value = false
}

async function saveGroup() {
  if (!requireAction(editingId.value ? 'system:usergroups:update' : 'system:usergroups:create')) return
  const name = groupForm.value.name.trim()
  if (!name) {
    message.warning('请填写用户组名称')
    return
  }
  saveLoading.value = true
  try {
    if (editingId.value) {
      // 成员整表替换语义：编辑组信息时带上当前成员，避免被清空。
      await updateUserGroup({
        id: editingId.value,
        name,
        remark: groupForm.value.remark.trim(),
        user_ids: (currentGroup.value?.members || []).map((member) => member.user_id),
      })
      message.success('用户组已更新')
    } else {
      const response = await createUserGroup({ name, remark: groupForm.value.remark.trim(), user_ids: [] })
      selectedGroupId.value = parseApiData(response)?.id || null
      message.success('用户组已创建')
    }
    modalVisible.value = false
    await loadGroups()
  } finally {
    saveLoading.value = false
  }
}

// 成员变更 = 以当前成员列表 + 变更调整 update 接口（整表替换）。
async function saveMembers(group, userIDs) {
  if (!requireAction('system:usergroups:update')) return false
  memberSaving.value = true
  try {
    await updateUserGroup({
      id: group.id,
      name: group.name,
      remark: group.remark || '',
      user_ids: userIDs,
    })
    await loadGroups()
    return true
  } finally {
    memberSaving.value = false
  }
}

async function addMember() {
  if (!requireAction('system:usergroups:update') || !requireAction('system:users:view')) return
  if (!memberToAdd.value || !currentGroup.value) return
  const userIDs = currentGroup.value.members.map((member) => member.user_id)
  userIDs.push(memberToAdd.value)
  const added = userList.value.find((user) => user.id === memberToAdd.value)
  if (!await saveMembers(currentGroup.value, userIDs)) return
  memberToAdd.value = null
  message.success(`已添加成员 ${added?.username || ''}`)
}

async function removeMember(record) {
  if (!requireAction('system:usergroups:update')) return
  // 确认期间可能切换用户组，更新目标必须固定为点击时的组。
  const group = currentGroup.value
  if (!group) {
    message.warning('请先选择用户组')
    return
  }
  await openDeleteConfirm({
    title: '确认移出用户组成员',
    okText: '确认移出',
    summary: '仅移除该用户与本组的成员关系，不会删除平台用户；会影响通过该组选择的通知收件范围。',
    items: [`用户组：${group.name}（#${group.id}）`, `成员：${record.username}（#${record.user_id}）`],
    onConfirm: async () => {
      if (!await saveMembers(group, group.members.map((member) => member.user_id).filter((id) => id !== record.user_id))) {
        throw new Error('用户组更新权限已失效')
      }
      message.success(`已将 ${record.username} 移出用户组 ${group.name}`)
    },
  })
}

async function loadGroups() {
  listLoading.value = true
  try {
    const response = await getUserGroupList()
    groupList.value = parseApiData(response).items || []
    if (selectedGroupId.value && !groupList.value.some((group) => group.id === selectedGroupId.value)) {
      selectedGroupId.value = null
    }
    if (!selectedGroupId.value && groupList.value.length) {
      selectedGroupId.value = groupList.value[0].id
    }
  } finally {
    listLoading.value = false
  }
}

async function handleDelete(record) {
  if (!requireAction('system:usergroups:delete')) return
  await openDeleteConfirm({
    title: '确认删除用户组',
    summary: '删除后引用该用户组的功能（如告警接收组）将不再包含这些成员。',
    items: [record.name],
    onConfirm: async () => {
      if (!requireAction('system:usergroups:delete')) throw new Error('用户组删除权限已失效')
      await batchDeleteUserGroups([record.id])
      message.success('用户组已删除')
      await loadGroups()
    },
  })
}

watch([canCreate, canUpdate], () => {
  if (modalVisible.value && !(editingId.value ? canUpdate.value : canCreate.value)) closeModal()
})

// 候选用户只用于添加成员；撤权后丢弃迟到结果，不能因查看用户组而额外查询平台用户。
watch(canAddMembers, async (allowed, _previous, onCleanup) => {
  let active = true
  onCleanup(() => { active = false })
  userList.value = []
  memberToAdd.value = null
  if (!allowed) return
  try {
    const users = await fetchAllPages(getUserList)
    if (active) userList.value = users
  } catch (error) {
    if (active) message.error(error?.response?.data?.msg || '加载候选用户失败')
  }
}, { immediate: true })

onMounted(async () => {
  try {
    await loadGroups()
  } catch (error) {
    message.error(error?.response?.data?.msg || '用户组数据加载失败')
  }
})
</script>

<style scoped>
.user-group-page {
  padding: 8px;
}

.user-group-layout {
  display: grid;
  grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}

.group-menu-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-right: 8px;
}

.group-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-add-row {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 22px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}

@media (max-width: 800px) {
  .user-group-layout {
    grid-template-columns: 1fr;
  }
}
</style>
