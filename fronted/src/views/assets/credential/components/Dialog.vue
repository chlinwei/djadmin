<template>
    <a-modal :open="open" :title="title" destroy-on-close @cancel="close">
        <a-spin :spinning="loading">
            <a-form v-if="!loading" ref="formRef" :model="form" :label-col="{ span: 6 }" :wrapper-col="{ span: 18 }" autocomplete="off">
                <a-form-item name="name" label="凭证名称" :rules="[{ required: true, message: '必填字段' }]">
                    <a-input v-model:value="form.name" :readonly="readOnly" />
                </a-form-item>
                <a-form-item name="auth_type" label="认证类型">
                    <a-radio-group v-model:value="form.auth_type" :disabled="readOnly">
                        <a-radio :value="1">密码</a-radio><a-radio :value="2">密钥</a-radio>
                    </a-radio-group>
                </a-form-item>
                <a-form-item name="username" label="用户名"><a-input v-model:value="form.username" :readonly="readOnly" /></a-form-item>
                <a-form-item name="port" label="端口"><a-input-number v-model:value="form.port" :disabled="readOnly" :min="1" :max="65535" /></a-form-item>
                <a-form-item v-if="form.auth_type === 2" name="private_key" label="SSH私钥">
                    <a-textarea v-model:value="form.private_key" :readonly="readOnly" />
                </a-form-item>
                <a-form-item v-else name="password" label="密码">
                    <a-input-password v-model:value="form.password" :readonly="readOnly" :visibility-toggle="!readOnly" />
                </a-form-item>
                <a-form-item name="remark" label="备注"><a-textarea v-model:value="form.remark" :readonly="readOnly" /></a-form-item>
            </a-form>
        </a-spin>
        <template #footer>
            <a-button @click="close">{{ readOnly ? '关闭' : '取消' }}</a-button>
            <a-button v-if="!readOnly" type="primary" :disabled="loading" :loading="saving" @click="save">保存</a-button>
        </template>
    </a-modal>
</template>

<script setup>
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { SaveOrCreateCredential, getCredentailById } from '@/api/assets/credential/index'

const props = defineProps({ open: Boolean, readOnly: Boolean, title: String, item_id: Number })
const emit = defineEmits(['update:open', 'initList'])
const empty = () => ({ id: -1, name: '', username: '', auth_type: 1, port: 22, password: '', private_key: null, remark: '' })
const form = ref(empty())
const formRef = ref()
const loading = ref(false)
const saving = ref(false)
const close = () => emit('update:open', false)

watch(() => [props.open, props.item_id], async ([open, id], _, onCleanup) => {
    let current = true
    onCleanup(() => { current = false })
    form.value = empty()
    loading.value = open && id !== -1
    if (!loading.value) return
    try {
        const { data } = await getCredentailById(id)
        if (!current) return
        if (data.code !== 200) throw new Error(data.msg)
        const row = data.data
        // The backend recognizes this mask as “keep existing”; never copy secret material into the form.
        form.value = { ...row, password: row.password ? '******' : '', private_key: row.private_key ? '******' : null }
    } catch (error) {
        if (current) { message.error(error.message || '获取凭证失败'); close() }
    } finally { if (current) loading.value = false }
}, { immediate: true })

async function save() {
    if (!props.open || props.readOnly || loading.value || saving.value) return
    saving.value = true
    try {
        await formRef.value.validate()
        if (!props.open || props.readOnly) return
        const { data } = await SaveOrCreateCredential({ ...form.value })
        if (data.code !== 200) throw new Error(data.msg)
        message.success('保存凭证成功')
        emit('initList')
        close()
    } catch (error) {
        if (!error.errorFields) message.error(error.message || '保存凭证失败')
    } finally { saving.value = false }
}
</script>
