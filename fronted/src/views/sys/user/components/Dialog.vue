<template>
    <div>
      <a-modal destroyOnClose v-model:open="props.open" v-model:title="props.title" v-model:user_id="props.user_id" @ok="handleOk" @cancel="handleCancel">
        <a-form
    :model="form"
    name="basic"
    :label-col="{ span: 8 }"
    :wrapper-col="{ span: 16 }"
    autocomplete="off"
    @finish="onFinish"
    @finishFailed="onFinishFailed"
  >
    <a-form-item
      label="用户名"
      name="username"
    >
      <a-input v-model:value="form.username"  disabled />
    </a-form-item>
    <a-form-item
      label="手机号"
      name="phonenumber"
    >
      <a-input v-model:value="form.phonenumber" />
    </a-form-item>
    <a-form-item  label="email" :rules="[{ type: 'email' }]">
      <a-input v-model:value="form.email" />
    </a-form-item>
    <a-form-item name="status" label="状态">
      <a-radio-group :value="String(form.status)" @change="e => form.status = Number(e.target.value)">
        <a-radio value="1">正常</a-radio>
        <a-radio value="0">禁止</a-radio>
      </a-radio-group>
    </a-form-item>
    <a-form-item :name="remark" label="备注">
      <a-textarea v-model:value="form.remark" />
    </a-form-item>
  </a-form>
      </a-modal>
    </div>
  </template>
  <script setup>

  import { ref } from 'vue';
  import { watch } from 'vue';
  import {getUserById} from '@/api/user/index.js';
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
  const form = ref({
    id: -1,
    username: "",
    password: "123456",
    status: null,
    phonenumber: "",
    email: "",
    remark: ""
    })

  const emits = defineEmits(['update:open'])
  const showModal = () => {
    open.value = true;
  };
  const handleOk = e => {
    emits('update:open',false);
  };
  watch(
    () => props.open,
    () => {
        let id = props.user_id
        console.log("watch:"+id)
        if(id === -1) {
            // 添加用户
            form.value = {
                id: -1,
                username: "",
                password: "123456",
                status: null,
                phonenumber: "",
                email: "",
                remark: ""
            }
        }else {
            if(props.open) {
                // 进入编辑界面
                console.log("编辑用户")
                getUserById(id).then(res => {
                form.value = res.data.data
                // form.value.status = form.value.status.toString()
                // console.log(form.value)
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
const handleCancel = () => {
    emits('update:open',false);
}
//   getUserById(user_id.value).then(res => {
//         title.value = res.data.data.username
//   })
  </script>