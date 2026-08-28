<template>
    <div>

        <!-- Context-bound select popups need stable Teleport anchors during the close transition. -->
        <a-modal cancelText="取消" okText="保存" :open="props.open" :title="props.title" width="1000px"
            :bodyStyle="{ maxHeight: '72vh', overflowY: 'auto' }"
            @ok="handleOk" @cancel="handleCancel" @afterClose="handleAfterClose">

            <a-spin :spinning="loading">
            <a-form :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }"
                :wrapper-col="{ span: 16 }" autocomplete="off" :rules="get_rules(form)">
                <a-form-item name="name" label="应用名称">
                    <a-input v-model:value="form.name" />
                </a-form-item>
                <a-form-item name="code" label="应用编码">
                    <a-input v-model:value="form.code" />
                </a-form-item>
                <a-form-item name="category" label="应用类别">
                    <a-select v-model:value="form.category" :options="categoryOptions" :getPopupContainer="getPopupContainer" />
                </a-form-item>
                <a-form-item name="vendor" label="厂商">
                    <a-input v-model:value="form.vendor" />
                </a-form-item>
                <a-form-item name="description" label="描述">
                    <a-textarea v-model:value="form.description" />
                </a-form-item>
                <a-form-item label="允许使用">
                    <a-switch v-model:checked="form.enabled" />
                </a-form-item>
                <a-form-item name="remark" label="备注">
                    <a-textarea v-model:value="form.remark" />
                </a-form-item>
            </a-form>
            </a-spin>
        </a-modal>
    </div>
</template>
<script setup>
import { ref, watch } from 'vue';
import { resolvePopupContainerByContext } from '@/util/popupContainer'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const categoryOptions = [
    { label: 'Web 容器', value: 'web_container' },
    { label: '数据库', value: 'database' },
    { label: '中间件', value: 'middleware' },
    { label: '业务应用', value: 'business' },
    { label: '其他', value: 'other' },
]
const formRef = ref(null)
const loading = ref(false)
const props = defineProps(
    {
        open: {
            type: Boolean,
            default: false,
            required: true
        },
        title: {
            type: String,
            default: '错误界面',
            required: true
        },
        item_id: {
            type: Number,
            default: -1,
            required: true
        },
        appname: {
            type: String,
            default: '应用',
            required: true
        }
    }
)


const get_rules = (obj) => {
    var add_rules = {
        name: [
            { required: true, message: "必填字段" }
        ],
        code: [
            { required: true, message: "必填字段" }
        ],
    }
    var edit_rules = {
        name: [
            { required: true, message: "必填字段" }
        ],
        code: [
            { required: true, message: "必填字段" }
        ],
    }
    if(obj.id == -1) {
        return add_rules
    }else {
        return edit_rules
    }
}


const createInitialForm = () => ({
    id: -1,
    name: '',
    code: '',
    category: 'other',
    vendor: '',
    description: '',
    enabled: true,
    remark: '',
})

const form = ref(createInitialForm())
let detailLoadToken = 0


const emits = defineEmits(['update:open', 'initList', 'saved'])
import {
    SaveOrCreateApplication,
    getApplicationById,
} from '@/api/assets/application/index.js';
const getItemById  = (id) =>{
    return getApplicationById(id)
}



const handleOk = (e) => {
    const res = formRef.value?.validate().then((r1) => {
        const obj = { ...form.value };
        if (obj.id == -1) {
            SaveOrCreateApplication(obj).then(result => {
                message.success("新增"+ props.appname+"成功");
                emits('saved', result?.data?.data)
                emits('initList')
                emits('update:open', false)
            })
        } else {
            SaveOrCreateApplication(obj).then(result => {
                message.success("保存"+ props.appname +"成功");
                emits('initList')
                emits('update:open', false);
            })
        }
    })

};



watch(
    [() => props.open, () => props.item_id],
    ([open, id]) => {
        const currentToken = ++detailLoadToken
        if (!open) {
            loading.value = false
            return
        }
        if (id === -1) {
            loading.value = false
            form.value = createInitialForm()
            return
        }

        loading.value = true
        getItemById(id).then(res => {
            if (currentToken !== detailLoadToken || !props.open || props.item_id !== id) return
            const data = res.data.data || {}
            form.value = {
                ...createInitialForm(),
                ...data,
            }
        }).finally(() => {
            if (currentToken === detailLoadToken) loading.value = false
        })
    },
)



import { message } from 'ant-design-vue';
// 取消窗口
const handleCancel = () => {
    detailLoadToken += 1
    emits('update:open', false);
}

const handleAfterClose = () => {
    form.value = createInitialForm()
    formRef.value = null
}
</script>