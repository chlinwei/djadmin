<template>
    <div>
      <a-modal cancelText="取消" okText="保存" destroyOnClose :open="props.open" v-model:title="props.title" v-model:user_id="props.user_id" @ok="handleOk" @cancel="handleCancel">
        <a-form
    :model="form"
    ref="formRef"
    name="basic"
    :label-col="{ span: 8 }"
    :wrapper-col="{ span: 16 }"
    autocomplete="off"
    :rules="props.user_id === -1 ? userAdd_rules:userEdit_rules"
  >
  <!-- :validate-messages="validateMessages" -->
    <a-form-item
      label="角色名称"
      name="name">
      <a-input  v-model:value="form.name"  :disabled="props.role_id !== -1"  />
    </a-form-item>
    <a-form-item
      label="权限字符"
      name="code"
    >
      <a-input v-model:value="form.code" />
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
  import {getUserById,checkUserName} from '@/api/user/index.js';
  const formRef = ref(null)
  const props = defineProps(
    {
        open: {
            type: Boolean,
            default:false,
            required: true
        },
        title: {
            type: String,
            default: '错误界面',
            required: true
        },
        user_id: {
            type: Number,
            default: -1,
            required: true
        },
    }
  )
 



const userAdd_rules = {
    name: [
        {required: true,message: "必填字段"},
    ],
    code: [
        {required: true,message: "必填字段"},
    ]
}
const userEdit_rules = {
    name: [
        {required: true,message: "必填字段"},
    ],
    code: [
        {required: true,message: "必填字段"},
    ]
}

  const form = ref({
    id: -1,
    name: "",
    code: "",
    remark: ""
    })


const emits = defineEmits(['update:open','initList'])

import { saveUserInfo,addUser } from '@/api/user/index.js';
const handleOk = e => {
    // 校验
    const res = formRef.value?.validate().then((r1)=>{
        let userInfo = form.value;
        if(userInfo.id==-1) {
            // 表示是新增
            if(userInfo.status == null) {
                userInfo.status = 0
            }
            addUser(userInfo).then(result => {
                console.log(result)
                message.success("新增角色成功");
                emits('initList')
                emits('update:open',false)
            })
        }else {
            saveUserInfo(userInfo).then(result => {
            message.success("保存角色成功");
            emits('initList')
            emits('update:open',false);
        })
        }
    })
   
  };
  watch(
    () => props.open,
    () => {
        let id = props.user_id
        if(id === -1) {
            // 添加角色
            form.value = {
                id: -1,
                username: "",
                password: "123456",
                status: 0,
                phonenumber: "",
                email: "",
                remark: ""
            }
            
        }else {
            if(props.open) {
                // 进入编辑界面
                getUserById(id).then(res => {
                form.value = res.data.data
            })
            }else{
                // 关闭编辑框框
                form.value = {
                    id: -1,
                    username: "",
                    password: "123456",
                    status: null,
                    phonenumber: "",
                    email: "",
                    remark: ""
                }
            }
        }
    }
  )
 

import { message } from 'ant-design-vue';
// 取消窗口
const handleCancel = () => {
    emits('update:open',false);   
}
  </script>