<template>
    <div>

        <a-modal  cancelText="取消" okText="保存" destroyOnClose :open="props.open" v-model:title="props.title" v-model:treeData="props.treeData"
            v-model:item_id="props.item_id" @ok="handleOk" @cancel="handleCancel">
            
            <a-spin :spinning="loading">
             </a-spin>
  <a-form v-if="!loading" :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }"
                autocomplete="off" :rules="get_rules(form)">
                <a-form-item  name="parent_id" label="上级菜单">
                    <a-tree-select v-model:value="form.parent_id" show-search style="width: 100%"
                        :dropdown-style="{ maxHeight: '400px', overflow: 'auto' }" placeholder="请选上级菜单" allow-clear
                        tree-default-expand-all :tree-data="getTreeDataByMenuType(treeData,form.menu_type)" tree-node-filter-prop="label"
                        :fieldNames="{ label: 'name', value: 'key', key: 'key', children: 'children' }">
                    </a-tree-select>
                </a-form-item>
                <a-form-item name="menu_type" label="菜单类型">
                    <a-radio-group v-model:value="form.menu_type" name="menu_type" :disabled="form.id !==-1">
                    <a-radio value="M">目录</a-radio>
                    <a-radio value="C">菜单</a-radio>
                    <a-radio value="F">按钮</a-radio>
                </a-radio-group>
                </a-form-item>
             
                <a-form-item name="icon" v-if="form.menu_type!=='F'">
                    <template #label>
                    <FontAwesomeIcon :icon="form.icon" />
                    <span>&nbsp;菜单图标</span>
                    </template>
                    <a-input v-model:value="form.icon" />
                </a-form-item>
                <a-form-item label="菜单名称" name="name">
                    <a-input v-model:value="form.name" />
                </a-form-item>
                <a-form-item label="权限标识" name="perms" v-if="form.menu_type!=='M'">
                    <a-input v-model:value="form.perms"/>
                </a-form-item>
                <a-form-item label="路由路径" name="path" v-if="form.menu_type!=='F'">
                    <a-input v-model:value="form.path"/>
                </a-form-item>
                <a-form-item label="组件路径" name="component" v-if="form.menu_type==='C'">
                    <a-input v-model:value="form.component"/>
                </a-form-item>
                <a-form-item name="location" label="组件位置" v-if="form.menu_type === 'C' || form.menu_type === 'M'">
                    <a-radio-group v-model:value="form.location" name="location">
                    <a-radio :value="1">左侧菜单</a-radio>
                    <a-radio :value="2">用户中心</a-radio>
                </a-radio-group>
                </a-form-item>
                <a-form-item name="is_expanded" label="目录默认展开" v-if="form.menu_type === 'M'">
                    <a-switch v-model:checked="form.is_expanded" checked-children="展开" un-checked-children="收起" />
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

// 菜单树最大层级（含虚拟根节点），可按需调整
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
        },
        max_tree_depth: {
            type: Number,
            default: 5,
        },
    }
)


const get_rules = (menu)=>{

            
         var    M_rules = {
                parent_id: [
                     { required: true, message: "必填字段" },
                ],
                menu_type: [
                    { required: true, message: "必填字段" },
                ],     
                name: [
                     { required: true, message: "必填字段" },
                ],
                path: [
                     { required: true, message: "必填字段" },
                ],
            }
        var    C_rules = {
                parent_id: [
                     { required: true, message: "必填字段" },
                ],
                menu_type: [
                    { required: true, message: "必填字段" },
                ],     
                name: [
                     { required: true, message: "必填字段" },
                ],
                perms: [
                     { required: true, message: "必填字段" },
                ],
                path: [
                     { required: true, message: "必填字段" },
                ],
                component: [
                     { required: true, message: "必填字段" },
                ],
            }
        var    F_rules = {
                parent_id: [
                     { required: true, message: "必填字段" },
                ],
                menu_type: [
                    { required: true, message: "必填字段" },
                ],     
                name: [
                     { required: true, message: "必填字段" },
                ],
                perms: [
                     { required: true, message: "必填字段" },
                ],
            }
        if(menu.menu_type==="M") {
            return M_rules
        }else if(menu.menu_type==="C") {
            return C_rules
        }else{
            return F_rules
        }

}

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
    menu_type: "F",
    icon: '',
    name: '',
    perms: '',
    path: '',
    component: '',
    order_num: 1,
    remark: '',
    location: 1,
    is_expanded: true,
})


const emits = defineEmits(['update:open', 'initList'])

import { saveOrCreateMenu, getMenuById } from '@/api/menu/index.js';

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

const handleOk = e => {
    formRef.value?.validate().then((r1) => {
        let obj = form.value;
        if (obj.id == -1) {
            saveOrCreateMenu(obj).then(result => {
                if (result.data.code === 200) {
                    message.success("新增菜单成功");
                    emits('initList')
                    emits('update:open', false)
                } else {
                    handleApiError({data: result.data})
                }
            }).catch(handleApiError)
        } else {
            saveOrCreateMenu(obj).then(result => {
                if (result.data.code === 200) {
                    message.success("保存菜单成功");
                    emits('initList')
                    emits('update:open', false);
                } else {
                    handleApiError({data: result.data})
                }
            }).catch(handleApiError)
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
                menu_type: "F",
                icon: '',
                name: '',
                perms: '',
                path: '',
                component: '',
                order_num: 1,
                remark: '',
                location: 1,
                is_expanded: true,
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
                    menu_type: "F",
                    icon: '',
                    name: '',
                    perms: '',
                    path: '',
                    component: '',
                    order_num: 1,
                    remark: '',
                    location: 1,
                    is_expanded: true,
                }
            }
        }
    }
)



const getTreeDataByMenuType = (data, menu_type, depth = 0) => {
  return data.map(item => ({
    name: item.name,
    disabled: getDisabledState(item.menu_type, menu_type) || depth >= props.max_tree_depth,
    key: item.key,
    icon: item.icon,
    order_num: item.order_num,
    perms: item.perms,
    path: item.path,
    component: item.component,
    menu_type: item.menu_type,
        is_expanded: item.is_expanded !== false,
    create_time: item.create_time,
    children: item.children ? getTreeDataByMenuType(item.children, menu_type, depth + 1) : null
  }));
};

function getDisabledState(currentType, targetType) {
  if (targetType === "M") return currentType !== "M";
  if (targetType === "C") return currentType !== "M"; 
  if (targetType === "F") return currentType === "F";
  return false;
}

import { message } from 'ant-design-vue';
// 取消窗口
const handleCancel = () => {
    emits('update:open', false);
}
</script>