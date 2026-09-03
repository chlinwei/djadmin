<template>
    <div class="user-center-page">
        <div class="user-center-layout">
            <aside class="profile-summary">
                <h2 class="section-title">个人信息</h2>
                <div class="profile-summary-content">
                    <div class="avatar">
                        <Avatar/>
                    </div>
                    <ul class="list-group">
                        <li class="list-item">
                            <SvgIcon name="user"></SvgIcon>
                            <div class="iten-wrapper">

                                <div class="item-name"><span>用户名称</span></div>
                                <div class="item-value"><span>{{ currentUser.user.username }}</span></div>
                            </div>

                        </li>
                        <li class="list-item">
                            <SvgIcon name="phone"></SvgIcon>
                            <div class="iten-wrapper">
                                <div class="item-name">电话</div>
                                <div class="item-value">{{ currentUser.user.phonenumber }}</div>
                            </div>

                        </li>
                        <li class="list-item">
                            <SvgIcon name="peoples"></SvgIcon>
                            <div class="iten-wrapper">
                                <div class="item-name">角色</div>
                                <div class="item-value">{{ roleList }}</div>
                            </div>

                        </li>
                        <li class="list-item">
                            <SvgIcon name="email"></SvgIcon>
                            <div class="iten-wrapper">
                                <div class="item-name">告警媒介</div>
                                <div class="item-value">在右侧“关联告警媒介”中选择</div>
                            </div>
                        </li>
                        <li class="list-item">
                            <SvgIcon name="date"></SvgIcon>
                            <div class="iten-wrapper">
                                <div class="item-name">创建时间</div>
                                <div class="item-value">{{ formatDateTime(currentUser.user.create_time) }}</div>
                            </div>
                        </li>


                    </ul>
                </div>
            </aside>
            <main class="settings-panel">
                <h2 class="section-title">账户设置</h2>
                <div class="settings-panel-content">
                    <a-tabs v-model:activeKey="activeKey">
                        <a-tab-pane key="1" tab="基本资料">
                            <a-form class="settings-form" :model="formState" name="basic"
                                :label-col="{ flex: '96px' }" :wrapper-col="{ flex: 1 }"
                                autocomplete="off" @finish="onFinish_updateUserInfo"
                                @finishFailed="onFinishFailed_updateUserInfo">
                                <a-form-item label="手机号码" name="phonenumber"
                                    :rules="[{ required: true, message: '请输入电话号码!' }]">
                                    <a-input v-model:value="formState.phonenumber" />
                                </a-form-item>
                                <a-form-item label="时区" name="timezone">
                                    <!-- virtual=false：选项数量少，禁用虚拟滚动规避 rc-virtual-list 卸载竞态（KeepAlive 切页时 removeEventListener of null，ant-design-vue 4.2.6 上游 bug）。 -->
                                    <a-select
                                        v-model:value="formState.timezone"
                                        :getPopupContainer="getPopupContainer"
                                        :options="timezoneOptions"
                                        :virtual="false"
                                        @change="handleTimezoneChange"
                                    />
                                </a-form-item>
                                <a-form-item>
                                    <a-button type="primary" html-type="submit" style="margin-left: 10px;">保存</a-button>
                                </a-form-item>

                            </a-form>

                        </a-tab-pane>
                        <a-tab-pane key="2" tab="修改密码">
                            <a-form class="settings-form" :model="password_formState" name="password"
                                :label-col="{ flex: '96px' }" :wrapper-col="{ flex: 1 }"
                                autocomplete="off" @finish="onFinish_updateUserPassword"
                                @finishFailed="onFinishFailed_updateUserPassword" :rules="password_rules">
                                <a-form-item name="old_password" label="旧密码">
                                    <a-input-password v-model:value="password_formState.old_password" />
                                </a-form-item>
                                <a-form-item name="new_password" label="新密码">
                                    <a-input-password v-model:value="password_formState.new_password" />
                                </a-form-item>
                                <a-form-item name="confirm_password" label="确认密码">
                                    <a-input-password v-model:value="password_formState.confirm_password" />
                                </a-form-item>
                                <a-form-item>
                                    <a-button type="primary" html-type="submit" style="margin-left: 10px;">保存</a-button>
                                </a-form-item>
                            </a-form>

                        </a-tab-pane>
                        <a-tab-pane key="3" tab="告警媒介">
                            <div class="alert-media-container">
                                <a-space direction="vertical" style="width: 100%">
                                    <a-button @click="openBindingModal">
                                        <FontAwesomeIcon :icon="['fas', 'fa-plus-circle']" />
                                        <span>&nbsp;添加媒介绑定</span>
                                    </a-button>
                                    <a-table
                                        :columns="bindingColumns"
                                        :data-source="alertMediaBindings"
                                        :loading="bindingLoading"
                                        :pagination="false"
                                        :scroll="{ x: 760 }"
                                        row-key="id"
                                        size="small"
                                    >
                                        <template #bodyCell="{ column, record }">
                                            <template v-if="column.key === 'recipients'">
                                                {{ record.recipients.join(', ') }}
                                            </template>
                                            <template v-else-if="column.key === 'enabled'">
                                                <a-tag :color="record.enabled ? 'success' : 'default'">
                                                    {{ record.enabled ? '已启用' : '已禁用' }}
                                                </a-tag>
                                            </template>
                                            <template v-else-if="column.key === 'operation'">
                                                <a-space>
                                                    <a-tooltip title="编辑">
                                                        <a-button size="small" type="primary" @click="editBinding(record)">
                                                            <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
                                                        </a-button>
                                                    </a-tooltip>
                                                    <a-tooltip title="删除">
                                                        <a-button class="delBtn" size="small" type="primary" danger @click="deleteBinding(record)">
                                                            <FontAwesomeIcon :icon="['fas', 'trash-can']" />
                                                        </a-button>
                                                    </a-tooltip>
                                                </a-space>
                                            </template>
                                        </template>
                                    </a-table>
                                </a-space>
                            </div>
                        </a-tab-pane>
                    </a-tabs>

                    <a-modal
                        v-model:open="bindingModalVisible"
                        :title="editingBindingId ? '编辑媒介绑定' : '添加媒介绑定'"
                        cancel-text="取消"
                        :ok-text="editingBindingId ? '保存' : '添加'"
                        :confirm-loading="bindingSaving"
                        centered
                        @ok="saveBinding"
                    >
                        <a-form layout="vertical" :model="bindingForm">
                            <a-form-item label="告警媒介" required>
                                <a-select
                                    v-model:value="bindingForm.media_id"
                                    :options="alertMediaOptions"
                                    :getPopupContainer="getPopupContainer"
                                    placeholder="请选择告警媒介"
                                    :disabled="Boolean(editingBindingId)"
                                />
                            </a-form-item>
                            <a-form-item label="收件人邮箱" required>
                                <a-textarea
                                    v-model:value="bindingForm.recipientsText"
                                    placeholder="请输入收件人邮箱"
                                    :rows="3"
                                />
                            </a-form-item>
                            <a-form-item label="启用此绑定">
                                <a-switch v-model:checked="bindingForm.enabled" />
                            </a-form-item>
                        </a-form>
                    </a-modal>
                </div>
            </main>
        </div>
    </div>

</template>
<script setup>
import { ref } from 'vue';
import { reactive } from 'vue';
import {
    getCurrentUser,
    getCurrentUserAlertMediaBindings,
    saveCurrentUser,
    updateCurrentUserAlertMediaBindings,
} from '@/api/user/index.js';
import { getCurrentUserRoleList } from '@/api/role';
import { updateUserInfo, updateUserPassword } from '@/api/user';
import { updateUserTimezone, getCurrentUserInfo } from '@/api/sys/userTimezone'
import { onMounted } from 'vue';
import { message } from 'ant-design-vue';
import Avatar from '@/views/userCenter/components/Avatar.vue';
import { openDeleteConfirm } from '@/util/deleteConfirm'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { TIMEZONE_LIST, formatTimeWithTimezone } from '@/util/timezone'
import { emitUserTimezoneChanged } from '@/util/userTimezoneSync'



defineOptions({
    name: 'userCenter'
})
const currentUser = reactive({ 'user': getCurrentUser() });
const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const activeKey = ref('1');
const formState = reactive({
    phonenumber: currentUser.user.phonenumber,
    timezone: currentUser.user.timezone || 'UTC'
});
const alertMediaOptions = ref([])
const alertMediaBindings = ref([])
const bindingLoading = ref(false)
const bindingSaving = ref(false)
const bindingModalVisible = ref(false)
const editingBindingId = ref(null)
const bindingForm = reactive({
    media_id: undefined,
    recipientsText: '',
    enabled: true,
})
const password_formState = reactive({
    old_password: '',
    new_password: '',
    confirm_password: ''
})

// 时区选项
const timezoneOptions = ref(TIMEZONE_LIST.map(tz => ({
    label: tz.label,
    value: tz.value
})))

const normalizeUtcTime = (value) => {
    if (!value || typeof value !== 'string') {
        return value
    }
    const text = value.trim()
    if (!text) {
        return value
    }
    if (/[zZ]$|[+-]\d{2}:\d{2}$/.test(text)) {
        return text
    }
    return `${text.replace(' ', 'T')}Z`
}

const formatDateTime = (value) => {
    if (!value) {
        return '-'
    }
    try {
        const timezone = formState.timezone || 'UTC'
        return formatTimeWithTimezone(normalizeUtcTime(value), timezone, 'YYYY-MM-DD HH:mm:ss')
    } catch (error) {
        return value
    }
}

const syncTimezoneToLocalUser = (timezone) => {
    const localUser = getCurrentUser()
    if (!localUser) {
        return
    }
    localUser.timezone = timezone
    saveCurrentUser(localUser)
    emitUserTimezoneChanged(timezone)
}

const refreshCurrentUserCache = async () => {
    try {
        const res = await getCurrentUserInfo()
        const userData = res?.data?.data
        if (userData) {
            currentUser.user = userData
            formState.phonenumber = userData.phonenumber || ''
            formState.timezone = userData.timezone || 'UTC'
            saveCurrentUser(userData)
            if (userData.timezone) {
                emitUserTimezoneChanged(userData.timezone)
            }
        }
    } catch (error) {
        console.error('刷新用户缓存失败:', error)
    }
}

const onFinish_updateUserInfo = values => {
    updateUserInfo(values, (result) => {
        currentUser.user = getCurrentUser();
        // 同时保存时区
        if (formState.timezone && currentUser.user && currentUser.user.id) {
            updateUserTimezone(currentUser.user.id, formState.timezone).then(() => {
                syncTimezoneToLocalUser(formState.timezone)
                refreshCurrentUserCache()
                message.success("更新成功")
            }).catch(error => {
                console.error('保存时区失败:', error)
            })
        } else {
            message.success("更新成功")
        }
    });
};
const onFinishFailed_updateUserInfo = errorInfo => {
    console.log('Failed:', errorInfo);
};

const loadAlertMediaBindings = async () => {
    bindingLoading.value = true
    try {
        const res = await getCurrentUserAlertMediaBindings()
        const data = res?.data?.data || {}
        const options = Array.isArray(data.options) ? data.options : []
        const mediaTypeById = new Map(options.map((item) => [item.id, item.media_type]))
        alertMediaOptions.value = options.map((item) => ({
            label: `${item.name} (${item.media_type})`,
            value: item.id,
        }))
        alertMediaBindings.value = Array.isArray(data.selected_bindings)
            ? data.selected_bindings.map((item) => ({
                ...item,
                media_type: mediaTypeById.get(item.media_id) || '-',
                recipients: Array.isArray(item.recipients) ? item.recipients : [],
            }))
            : []
    } finally {
        bindingLoading.value = false
    }
}

const onFinish_updateUserPassword = values => {
    var password_pair = {
        "old_password": values.old_password,
        "new_password": values.new_password
    }
    updateUserPassword(password_pair, () => {


    });
};
const onFinishFailed_updateUserPassword = errorInfo => {
    console.log('Failed:', errorInfo);
};
var roleList = ref('');

const openBindingModal = () => {
    editingBindingId.value = null
    bindingForm.media_id = undefined
    bindingForm.recipientsText = ''
    bindingForm.enabled = true
    bindingModalVisible.value = true
}

const editBinding = (binding) => {
    editingBindingId.value = binding.id
    bindingForm.media_id = binding.media_id
    bindingForm.recipientsText = binding.recipients.join('\n')
    bindingForm.enabled = binding.enabled
    bindingModalVisible.value = true
}

const toBindingPayload = (bindings) => bindings.map((binding) => ({
    media_id: binding.media_id,
    recipients: binding.recipients,
    enabled: binding.enabled,
}))

const saveBinding = async () => {
    if (!bindingForm.media_id) {
        message.warning('请选择告警媒介')
        return
    }

    const recipients = [...new Set(
        bindingForm.recipientsText
            .split(/[,;，；\n]/)
            .map((item) => item.trim())
            .filter(Boolean),
    )]
    if (!recipients.length || recipients.some((item) => !item.includes('@'))) {
        message.warning('请输入有效的收件人邮箱')
        return
    }
    if (!editingBindingId.value && alertMediaBindings.value.some((item) => item.media_id === bindingForm.media_id)) {
        message.warning('该告警媒介已经绑定')
        return
    }

    const nextBinding = {
        media_id: bindingForm.media_id,
        recipients,
        enabled: bindingForm.enabled,
    }
    const nextBindings = editingBindingId.value
        ? alertMediaBindings.value.map((item) => item.id === editingBindingId.value ? nextBinding : item)
        : [...alertMediaBindings.value, nextBinding]

    bindingSaving.value = true
    try {
        await updateCurrentUserAlertMediaBindings(toBindingPayload(nextBindings))
        bindingModalVisible.value = false
        message.success(editingBindingId.value ? '媒介绑定已更新' : '媒介绑定已添加')
        await loadAlertMediaBindings()
    } finally {
        bindingSaving.value = false
    }
}

const deleteBinding = (binding) => {
    openDeleteConfirm({
        title: '删除媒介绑定',
        summary: '删除后，该媒介将不再向当前用户发送告警。',
        items: [binding.media_name],
        onConfirm: async () => {
            const nextBindings = alertMediaBindings.value.filter((item) => item.id !== binding.id)
            await updateCurrentUserAlertMediaBindings(toBindingPayload(nextBindings))
            message.success('媒介绑定已删除')
            await loadAlertMediaBindings()
        },
    })
}

const bindingColumns = [
    { title: '媒介名称', dataIndex: 'media_name', key: 'media_name', width: 150 },
    { title: '媒介类型', dataIndex: 'media_type', key: 'media_type', width: 100 },
    { title: '收件人', dataIndex: 'recipients', key: 'recipients', width: 300 },
    { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 80 },
    { title: '操作', key: 'operation', fixed: 'right', width: 100 },
]

// 处理时区变更 - 实时保存
const handleTimezoneChange = (value) => {
    if (currentUser.user && currentUser.user.id) {
        updateUserTimezone(currentUser.user.id, value).then(() => {
            syncTimezoneToLocalUser(value)
            refreshCurrentUserCache()
            message.success("时区已保存")
        }).catch(error => {
            console.error('保存时区失败:', error)
            message.error("保存时区失败")
        })
    }
}

onMounted(() => {
    // 从API加载最新用户信息（包括时区）
    getCurrentUserInfo().then(res => {
        if (res.data && res.data.data) {
            const userData = res.data.data
            // 更新currentUser
            currentUser.user = userData
            // 更新表单字段
            formState.phonenumber = userData.phonenumber || ''
            formState.timezone = userData.timezone || 'UTC'
            saveCurrentUser(userData)
        }
    }).catch(error => {
        console.error('获取用户信息失败:', error)
        // 降级处理
        const localUser = getCurrentUser()
        if (localUser) {
            formState.phonenumber = localUser.phonenumber || ''
            formState.timezone = localUser.timezone || 'UTC'
        }
    })
    
    // 初始化基本资料
    formState.phonenumber = currentUser.user.phonenumber;
    formState.timezone = currentUser.user.timezone || 'UTC'
    getCurrentUserRoleList().then(result => {
        result.data.data.roleList.forEach(element => {
            roleList.value = roleList.value + element.name + ' ';
        });
    })
    loadAlertMediaBindings().catch(error => {
        console.error('加载告警媒介关联失败:', error)
    })

})

const check_confirm_password = async (_rule, value) => {
    if (value != password_formState.new_password) {
        return Promise.reject('新密码和旧密码不一致!')
    } else {
        Promise.resolve();
    }

}

//密码规则
const password_rules = {
    old_password: [
        { required: true, message: '请输入旧密码!' }
    ],
    new_password: [
        { required: true, message: '请输入新密码!' }
    ],
    confirm_password: [
        { required: true, message: '请输入确认密码!' },
        { validator: check_confirm_password, trigger: 'change' }
    ]
}
</script>


<style scoped>
.user-center-page {
    width: 100%;
    min-height: calc(100vh - 140px);
    padding: 24px;
    background: #fff;
}

.user-center-layout {
    display: grid;
    grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
    min-height: 620px;
}

.profile-summary {
    padding-right: 28px;
    border-right: 1px solid #f0f0f0;
}

.settings-panel {
    min-width: 0;
    padding-left: 32px;
}

.section-title {
    margin: 0;
    padding-bottom: 18px;
    border-bottom: 1px solid #f0f0f0;
    color: rgba(0, 0, 0, 0.88);
    font-size: 18px;
    font-weight: 600;
    line-height: 1.5;
}

.profile-summary-content {
    padding-top: 24px;
}

.settings-panel-content {
    padding-top: 8px;
}

.settings-form {
    width: min(100%, 760px);
    padding-top: 16px;
}

.avatar {
    margin-bottom: 30px;
}

.alert-media-container {
    padding: 20px 0;
}


.list-group>.list-item {
    margin-bottom: 1px sold red !important;
    margin-top: 1px sold #e7eaec;
    margin-bottom: 2px;
    ;
    padding: 11px 0;
    vertical-align: middle;
    display: flex;

}

.item-wrapper {
    vertical-align: middle;
}

.iten-wrapper {
    display: flex;
    width: 100%;
    justify-content: space-around;
    height: 16px;
    align-items: center;

}

.item-name {
    padding-left: 10px;
    width: 30%;
}

.item-value {
    width: 70%;
    text-align: right;

}

.item-name>span {
    vertical-align: middle;
    height: 100%;
}

.item-value>span {
    vertical-align: middle;
    height: 100%;
}

.text {
    height: 16px;
}

@media (max-width: 900px) {
    .user-center-page {
        padding: 16px;
    }

    .user-center-layout {
        grid-template-columns: 1fr;
    }

    .profile-summary {
        padding-right: 0;
        padding-bottom: 24px;
        border-right: 0;
        border-bottom: 1px solid #f0f0f0;
    }

    .settings-panel {
        padding-top: 24px;
        padding-left: 0;
    }
}
</style>