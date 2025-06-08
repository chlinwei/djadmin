<template>
    <div>

        <a-modal cancelText="取消" okText="保存" destroyOnClose :open="props.open" v-model:title="props.title"
            v-model:item_id="props.item_id" @ok="handleOk" @cancel="handleCancel">

            <a-spin :spinning="loading">
            </a-spin>
            <a-form v-if="!loading" :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }"
                :wrapper-col="{ span: 16 }" autocomplete="off" :rules="get_rules(form)">
                <a-form-item name="name" label="ssh用户">
                    <a-input v-model:value="form.name" />
                </a-form-item>
                <a-form-item name="password" label="ssh密码">
                    <a-input-password v-model:value="form.password" />
                </a-form-item>
                <a-form-item name="remark" label="备注">
                    <a-textarea v-model:value="form.remark" />
                </a-form-item>
            </a-form>
        </a-modal>
    </div>
</template>
<script setup>
import { ref } from 'vue';
import { watch } from 'vue';
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
        }
    }
)


const get_rules = (obj) => {
    var add_rules = {
        name: [
            { required: true, message: "必填字段" }
        ],
    }
    var edit_rules = {
        name: [
            { required: true, message: "必填字段" }
        ]
    }
    if(obj.id == -1) {
        return add_rules
    }else {
        return edit_rules
    }
}



const form = ref({
    id: -1,
    name: '',
    password: '',
    remark: '',
})


const emits = defineEmits(['update:open', 'initList'])
import { SaveOrCreateCredential } from '@/api/assets/credential/index.js';
import {getCredentailById} from '@/api/assets/credential/index'
const getItemById  = (id) =>{
    return getCredentailById(id)
}

const handleOk = (e) => {
    const res = formRef.value?.validate().then((r1) => {
        var  obj = form.value;
        if (obj.id == -1) {
            SaveOrCreateCredential(obj).then(result => {
                message.success("新增credential成功");
                emits('initList')
                emits('update:open', false)
            })
        } else {
            SaveOrCreateCredential(obj).then(result => {
                message.success("保存credential成功");
                emits('initList')
                emits('update:open', false);
            })
        }
    })

};



watch(
    () => props.open,
    () => {

        let id = props.item_id
        if (id === -1) {
            // 添加credential
            form.value = {
                id: -1,
                name: '',
                password: '',
                remark: '',
            }
        } else {
            if (props.open) {
                // 进入编辑界面
                loading.value = true
                getItemById(id).then(res => {
                    form.value = res.data.data
                }).finally(() => {
                    loading.value = false
                })
            } else {
                // 关闭编辑框框
                form.value = {
                    id: -1,
                    name: '',
                    password: '',
                    remark: '',
                }
            }
        }
    }

)



import { message } from 'ant-design-vue';
// 取消窗口
const handleCancel = () => {
    emits('update:open', false);
}
</script>