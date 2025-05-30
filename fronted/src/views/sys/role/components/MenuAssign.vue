<template>
    <div>
       
        <a-modal  cancelText="取消" okText="保存" destroyOnClose :open="props.open" :title="props.title"
             @ok="handleOk" @cancel="handleCancel" v-model:role_id="props.role_id">
             <div>
                <a-spin :spinning="loading">
             </a-spin>
            <a-tree v-if="!loading"
             default-expand-all checkable
            :tree-data="treeData" 
            @check="check"
            :checkedKeys="checkedKeys">
            </a-tree>
             </div>
        </a-modal>
    </div>
</template>
<script setup>
import { ref, watch } from 'vue';
import { getMenuTree,getMenuListByRoleId,grantMenu } from '@/api/menu';
import { message } from 'ant-design-vue';
const loading = ref(true)
const props = defineProps(
    {
        open: {
            type: Boolean,
            default: false,
            required: true
        },
        role_id: {
            type: Number,
            default: -1,
            required: true
        },
        title: {
            type: String,
            default: "",
            required: true
        }
    }
)
const treeData = ref([])
const role_id = ref(-1)
const emits = defineEmits(['update:menuAssignVisible','initList'])
// 解析数据为树形结构
const parseTreeData = (data) => {
    return data.map(item => ({
        title: item.name,
        key: item.id,
        // icon: <a-icon type={item.icon} />,  // 注意：这里使用了Ant Design的图标组件，需要根据实际情况调整
        // disabled: !item.perms || item.perms.length === 0,  // 根据perms字段判断是否有权限
        children: item.children ? parseTreeData(item.children) : [],
    }));
};


const handleOk = () => {
    var menuIds = checkedKeys.value
    grantMenu(props.role_id,menuIds).then((res)=>{
        if(res.data.code==200) {
            emits("initList")
            emits("update:menuAssignVisible",false)
            message.success("分配权限保存成功")
        }else {
            message.success("分配权限保存失败")
        }
    })
}
const handleCancel = () => {
    emits("update:menuAssignVisible",false)
}

const checkedKeys = ref([])

watch(
    () => props.open,
    () => {
        if(props.open) {
            loading.value = true;
            getMenuTree().then((res) => {
            var data = parseTreeData(res.data.data)
            treeData.value = data
            loading.value = false
            }).finally(()=>{
                loading.value = false;
            })
            getMenuListByRoleId(props.role_id).then((res)=>{
                checkedKeys.value = res.data.data
            })
        }else {
            treeData.value = []
            checkedKeys.value = []
        }
    }
)

watch(checkedKeys, () => {
  console.log('checkedKeys', checkedKeys);
});
// 点击menu就会触发这个
const check = (res) => {
    checkedKeys.value  = res
}

const formRef = ref(null)

</script>