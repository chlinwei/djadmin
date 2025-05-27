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
      label="用户名"
      name="username">
      <a-input  v-model:value="form.username"  :disabled="props.user_id !== -1"  />
    </a-form-item>
    <a-form-item
      label="手机号"
      name="phonenumber"
    >
      <a-input v-model:value="form.phonenumber" />
    </a-form-item>
    <a-form-item name="email"  label="email">
      <a-input v-model:value="form.email" />
    </a-form-item>
    <a-form-item name="status" label="状态">
      <a-radio-group :value="String(form.status)" @change="e => form.status = Number(e.target.value)">
        <a-radio value="1">正常</a-radio>
        <a-radio value="0">禁止</a-radio>
      </a-radio-group>
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
 
// 自定义校验 async await 版本
// const checkUserName_rule=async  (rule, value) => {
//     if (!value) {
//     return callback(new Error('用户名不能为空'));
//   }
//   try {
//     const response = await checkUserName(value);
//     console.log(response)
//     return response.data.data.exists 
//       ? Promise.reject('用户名已存在') 
//       : Promise.resolve();
//   } catch (error) {
//     return Promise.reject('校验服务异常');
//   }
    
// }
// 传统版本
const checkUserName_rule = (rule, value) => {
  if (!value) {
    return Promise.reject('用户名不能为空');
  }
  try {

  
  return checkUserName(value)
    .then(response => {
      return response.data.data.exists 
        ? Promise.reject('用户名已存在')
        : Promise.resolve();
    })
}
catch (error) {
    return Promise.reject('校验服务异常');
  }
}

const userAdd_rules = {
    username: [
        {validator: checkUserName_rule,trigger: 'blur'}
    ],
    email: [
        {required: true,message: "必填字段"},
        {type: "email",message: "邮箱格式不正常"}
    ]
}
const userEdit_rules = {
    email: [
        {required: true,message: "必填字段"},
        {type: "email",message: "邮箱格式不正常"}
    ]
}

  const form = ref({
    id: -1,
    username: "",
    password: "123456",
    status: null,
    phonenumber: null,
    email: null,
    remark: null
    })


const emits = defineEmits(['update:open','initUserList'])

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
                message.success("新增用户成功");
                emits('initUserList')
                emits('update:open',false)
            })
        }else {
            saveUserInfo(userInfo).then(result => {
            message.success("保存用户成功");
            emits('initUserList')
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
            // 添加用户
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