<template>
    <div>

        <a-modal  cancelText="取消" okText="保存" destroyOnClose :open="props.open" v-model:title="props.title" v-model:treeData="props.treeData"
            v-model:item_id="props.item_id" @ok="handleOk" @cancel="handleCancel">
            
            <a-spin :spinning="loading">
             </a-spin>

            <a-form v-if="!loading" :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }"
                autocomplete="off" :rules="props.user_id === -1 ? add_rules : edit_rules">

                <a-form-item  name="parent_id" label="上级菜单">
                    <a-tree-select v-model:value="form.parent_id" show-search style="width: 100%"
                        :dropdown-style="{ maxHeight: '400px', overflow: 'auto' }" placeholder="请选上级菜单" allow-clear
                        tree-default-expand-all :tree-data="treeData" tree-node-filter-prop="label"
                        :fieldNames="{ label: 'name', value: 'key', key: 'key', children: 'children' }">
                    </a-tree-select>
                </a-form-item>
                <a-form-item name="menu_type" label="菜单类型">
                    <a-radio-group v-model:value="form.menu_type" name="menu_type">
                    <a-radio value="M">目录</a-radio>
                    <a-radio value="C">菜单</a-radio>
                </a-radio-group>
                </a-form-item>

                <a-form-item name="icon" label="菜单图标">
                    <a-input v-model:value="form.icon" />
                </a-form-item>
                <a-form-item label="菜单名称" name="name">
                    <a-input v-model:value="form.name" />
                </a-form-item>
                <a-form-item label="权限标识" name="perms">
                    <a-input v-model:value="form.perms"/>
                </a-form-item>
                <a-form-item label="路由路径" name="path">
                    <a-input v-model:value="form.path"/>
                </a-form-item>
                <a-form-item label="组件路径" name="component">
                    <a-input v-model:value="form.component"/>
                </a-form-item>
                <a-form-item label="显示顺序" name="order_num">
                    <a-input v-model:value="form.order_num"/>
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
        },
        treeData: {
            type: Array,
            default: [],
            required: true
        }
    }
)



const add_rules = {
    name: [
        { required: true, message: "必填字段" },
    ],
}
const edit_rules = {
    name: [
        { required: true, message: "必填字段" },
    ]
}

const form = ref({
    id: -1,
    parent_id: '',
    menu_type: "C",
    icon: '',
    name: '',
    perms: '',
    path: '',
    component: '',
    order_num: 1,
    remark: ''
})


const emits = defineEmits(['update:open', 'initList'])

import { saveOrCreateMenu, getMenuById } from '@/api/menu/index.js';
const handleOk = e => {
    // 校验
    const res = formRef.value?.validate().then((r1) => {
        let obj = form.value;
        if (obj.id == -1) {
            // 表示是新增
            saveOrCreateMenu(obj).then(result => {
                message.success("新增菜单成功");
                emits('initList')
                emits('update:open', false)
            })
        } else {
            saveOrCreateMenu(obj).then(result => {
                message.success("保存菜单成功");
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
            // 添加菜单
            form.value = {
                id: -1,
                parent_id: '',
                menu_type: "C",
                icon: '',
                name: '',
                perms: '',
                path: '',
                component: '',
                order_num: 1,
                remark: ''
            }
        } else {
            if (props.open) {
                // 进入编辑界面
                loading.value = true
                getMenuById(id).then(res => {
                    form.value = res.data.data
                }).finally(()=>{
                    loading.value = false
                })
            } else {
                // 关闭编辑框框
                form.value = {
                    id: -1,
                    parent_id: '',
                    menu_type: "M",
                    icon: '',
                    name: '',
                    perms: '',
                    path: '',
                    component: '',
                    order_num: 1,
                    remark: ''
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