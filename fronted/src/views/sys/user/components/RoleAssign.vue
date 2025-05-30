<template>
    <a-modal cancelText="取消" okText="保存" destroyOnClose :open="props.open2" v-model:user_id2="props.user_id2" @ok="handleOk"
        @cancel="handleCancel">
        <div>
            <a-spin :spinning="transferLoading">
                <a-transfer v-if="!transferLoading" v-model:target-keys="user_roleKey_list_ref" v-model:selected-keys="selected_keys" show-search
                :data-source="roleList_ref" :titles="['所有角色', '已拥有角色']" :render="item => item.title" @change="handleChange"
                :locale="locale"
                    :list-style="{
                        width: '300px',
                        height: '500px',
                    }"
                @selectChange="handleSelectChange" />
            </a-spin>
            

        </div>
    </a-modal>
</template>

<script setup>

import { ref ,reactive} from 'vue';
import { getRoleList, getUserRoleListByUserId } from '@/api/role';
import {saveUserPwd} from '@/api/user';
import { watch } from 'vue';
import { message } from 'ant-design-vue';
const locale = reactive(
    { itemUnit: '项', itemsUnit: '项', notFoundContent: '列表为空', searchPlaceholder: '请输入搜索内容' }
)
const transferLoading = ref(true)
const props = defineProps(
    {
        open2: {
            type: Boolean,
            default: false,
            required: true
        },
        user_id2: {
            type: Number,
            default: -1,
            required: true
        },
    }
)

const emits = defineEmits(['update:open2', 'initUserList'])
const handleCancel = () => {
    emits('update:open2', false)
}
const handleOk = () => {
    let id = props.user_id2
    saveUserPwd(id,user_roleKey_list_ref.value).then(res=>{
        if(res.data.code==200) {
            emits('initUserList')
            emits('update:open2', false)
            message.success("保存角色成功")
        }else {
            message.error("保存角色失败:" + res.data.msg)
        }
    })
}
var  roleList = []
const roleList_ref = ref([])

var  user_roleKey_list = []
const user_roleKey_list_ref = ref([])

const selected_keys = ref([])
watch(
    () => props.open2,
    () => {
        let id = props.user_id2
        if (props.open2) {
            transferLoading.value = true
            // 获取角色列表
            getRoleList().then((res) => {
                console.log(res)
                if (res.data.code === 200) {
                    res.data.data.forEach(role => {
                        roleList.push({
                            key: role.id.toString(),
                            title: role.name,
                            description: role.name,
                        })
                    });
                    roleList_ref.value = roleList

                }
            })
            // 正常的id
            
            getUserRoleListByUserId(id).then((res) => {
                res.data.data.roleList.forEach(role =>{
                    user_roleKey_list.push(role.id.toString())
                })
                user_roleKey_list_ref.value = user_roleKey_list
                
            }).finally(()=>{
                transferLoading.value = false
            })
            
        }else {
            user_roleKey_list = []
            roleList = []
            selected_keys.value = []
            console.log("清空")
        }

    }
)




const handleChange = (nextTargetKeys, direction, moveKeys) => {

};
const handleSelectChange = (sourceSelectedKeys, targetSelectedKeys) => {

};
</script>
<style scoped></style>