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
const MAX_TREE_DEPTH = 5

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
        disabled: item.depth >= MAX_TREE_DEPTH,
        children: item.children ? markSelectableTree(item.children) : undefined,
    }))
}

const treeSelectData = computed(() => markSelectableTree(props.treeData))

const handleOk = () => {
    formRef.value?.validate().then(() => {
        saveOrCreateHostGroup(form.value).then((res) => {
            if (res.data.code === 200) {
                message.success(form.value.id === -1 ? '新增分组成功' : '保存分组成功')
                emits('initList')
                emits('update:open', false)
            } else {
                message.error(res.data.msg || '保存失败')
            }
        })
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
                parent_id: 0,
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
                parent_id: 0,
                name: '',
                remark: '',
            }
        }
    }
)
</script>

<style scoped>
</style>