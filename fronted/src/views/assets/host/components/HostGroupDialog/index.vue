<template>
    <div>
        <a-modal cancelText="取消" okText="保存" destroyOnClose :open="props.open" :title="props.title" @ok="handleOk" @cancel="handleCancel">
            <a-spin :spinning="loading" />
            <a-form v-if="!loading" :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }" autocomplete="off" :rules="rules">
                <a-form-item label="分组名称" name="name">
                    <a-input v-model:value="form.name" placeholder="例如：生产环境" />
                </a-form-item>
                <a-form-item label="备注" name="remark">
                    <a-textarea v-model:value="form.remark" :rows="3" placeholder="可填写用途、环境、负责人等信息" />
                </a-form-item>
            </a-form>
        </a-modal>
    </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { getHostGroupById, saveOrCreateHostGroup } from '@/api/assets/hostgroup/index.js'

const formRef = ref(null)
const loading = ref(false)

const props = defineProps({
    open: {
        type: Boolean,
        default: false,
        required: true,
    },
    title: {
        type: String,
        default: '错误界面',
        required: true,
    },
    item_id: {
        type: Number,
        default: -1,
        required: true,
    },
})

const form = ref({
    id: -1,
    name: '',
    remark: '',
})

const rules = {
    name: [{ required: true, message: '必填字段' }],
}

const emits = defineEmits(['update:open', 'initList'])

const handleApiError = (err) => {
    message.error(err?.response?.data?.msg || err?.data?.msg || err?.message || '操作失败，请稍后重试')
}

const handleOk = () => {
    formRef.value?.validate().then(() => {
        const payload = { ...form.value }
        return saveOrCreateHostGroup(payload).then((res) => {
            if (res.data.code === 200) {
                message.success(form.value.id === -1 ? '新增分组成功' : '保存分组成功')
                emits('initList')
                emits('update:open', false)
            } else {
                handleApiError({data: res.data})
            }
        })
    }).catch((error) => {
        if (!error?.errorFields?.length) handleApiError(error)
    })
}

const handleCancel = () => {
    emits('update:open', false)
}

watch(
    () => props.open,
    async (open, previous, onCleanup) => {
        let cancelled = false
        // A closed/reopened dialog must not receive an earlier detail response.
        onCleanup(() => { cancelled = true })
        const id = props.item_id
        form.value = { id, name: '', remark: '' }
        loading.value = false
        if (!open || id === -1) return
        loading.value = true
        try {
            const res = await getHostGroupById(id)
            if (cancelled) return
            if (res.data.code !== 200) throw new Error(res.data.msg || '获取分组详情失败')
            const data = res.data.data
            form.value = {
                id: data.id,
                name: data.name,
                remark: data.remark || '',
            }
        } catch (error) {
            if (!cancelled) {
                handleApiError(error)
                emits('update:open', false)
            }
        } finally {
            if (!cancelled) loading.value = false
        }
    }
)
</script>

<style scoped>
</style>