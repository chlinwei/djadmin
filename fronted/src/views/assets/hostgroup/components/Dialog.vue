<template>
    <div>
        <a-modal cancelText="取消" okText="保存" destroyOnClose :open="props.open" v-model:title="props.title" @ok="handleOk" @cancel="handleCancel">
            <a-spin :spinning="loading" />
            <a-form v-if="!loading" :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }" autocomplete="off" :rules="rules">
                <a-form-item name="parent_id" label="上级分组">
                    <a-tree-select
                        v-model:value="form.parent_id"
                        show-search
                        style="width: 100%"
                        :dropdown-style="{ maxHeight: '400px', overflow: 'auto' }"
                        placeholder="请选择上级分组"
                        allow-clear
                        tree-default-expand-all
                        :tree-data="treeSelectData"
                        tree-node-filter-prop="label"
                        :fieldNames="{ label: 'name', value: 'key', key: 'key', children: 'children' }"
                    />
                </a-form-item>
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
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { getHostGroupById, saveOrCreateHostGroup } from '@/api/assets/hostgroup/index.js'

const formRef = ref(null)
const loading = ref(false)
// 最大层级从 props 接收（由父组件从 sys_config 动态加载）
// 前端默认值 5 仅作为 fallback

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
    treeData: {
        type: Array,
        default: () => [],
        required: true,
    },
    default_parent_id: {
        type: Number,
        default: null,
    },
    max_tree_depth: {
        type: Number,
        default: 5,
    },
})

const form = ref({
    id: -1,
    parent_id: 0,
    name: '',
    remark: '',
})

const rules = {
    name: [{ required: true, message: '必填字段' }],
}

const emits = defineEmits(['update:open', 'initList'])

const markSelectableTree = (nodes) => {
    return nodes.map((item) => ({
        ...item,
        disabled: item.depth >= props.max_tree_depth,
        children: item.children ? markSelectableTree(item.children) : undefined,
    }))
}

const treeSelectData = computed(() => markSelectableTree(props.treeData))

const handleApiError = (err) => {
    // 从响应中提取错误数据
    const responseData = err?.response?.data || err?.data
    // 如果是统一格式的错误响应（code 和 msg），data 字段才是实际错误内容
    let errorObj = responseData
    if (responseData && responseData.data && typeof responseData.data === 'object') {
        errorObj = responseData.data
    }
    
    if (errorObj && typeof errorObj === 'object') {
        const firstKey = Object.keys(errorObj)[0]
        const msg = Array.isArray(errorObj[firstKey]) ? errorObj[firstKey][0] : errorObj[firstKey]
        if (msg) { message.error(msg); return }
    }
    message.error('操作失败，请稍后重试')
}

const handleOk = () => {
    formRef.value?.validate().then(() => {
        const payload = {
            ...form.value,
            parent_id: form.value.parent_id ?? 0,
        }
        saveOrCreateHostGroup(payload).then((res) => {
            if (res.data.code === 200) {
                message.success(form.value.id === -1 ? '新增分组成功' : '保存分组成功')
                emits('initList')
                emits('update:open', false)
            } else {
                // 任何非 200 的返回都认为是错误
                handleApiError({data: res.data})
            }
        }).catch(handleApiError)
    })
}

const handleCancel = () => {
    emits('update:open', false)
}

watch(
    () => props.open,
    () => {
        const id = props.item_id
        if (id === -1) {
            form.value = {
                id: -1,
                parent_id: (props.default_parent_id && props.default_parent_id !== 0) ? props.default_parent_id : null,
                name: '',
                remark: '',
            }
        } else if (props.open) {
            loading.value = true
            getHostGroupById(id).then((res) => {
                if (res.data.code === 200) {
                    const data = res.data.data || {}
                    form.value = {
                        id: data.id ?? id,
                        parent_id: data.parent_id ?? data.parent?.id ?? 0,
                        name: data.name || '',
                        remark: data.remark || '',
                    }
                } else {
                    message.error(res.data.msg || '获取分组详情失败')
                }
            }).finally(() => {
                loading.value = false
            })
        } else {
            form.value = {
                id: -1,
                parent_id: (props.default_parent_id && props.default_parent_id !== 0) ? props.default_parent_id : null,
                name: '',
                remark: '',
            }
        }
    }
)
</script>

<style scoped>
</style>