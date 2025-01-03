<template>
    <div>
        <a-row>
            <a-col :span="6">
                <a-card title="个人信息" :bordered="false" style="width: 300px">
                    <div class="avatar">头像</div>
                    <ul>
                        <li>
                            <SvgIcon name="user"></SvgIcon>
                            &nbsp; <span>{{ currentUser.username }}</span>
                        </li>
                        <li>
                            <SvgIcon name="phone"></SvgIcon>
                            &nbsp; <span>{{ currentUser.phonenumber }}</span>
                        </li>
                        <li>
                            <SvgIcon name="email"></SvgIcon>
                            &nbsp; <span>{{ currentUser.email }}</span>
                        </li>
                        <li>
                            <SvgIcon name="peoples"></SvgIcon>
                            &nbsp; <span>所属角色</span>
                            <data class="pull-right"></data>
                        </li>
                        <li>
                            <SvgIcon name="date"></SvgIcon>
                            &nbsp; <span>创建时间{{ currentUser.create_time }}</span>
                        </li>


                    </ul>
                </a-card>
            </a-col>
            <a-col :span="18">
                <a-card title="基本资料" :bordered="false" style="width: 300px">
                    <a-tabs v-model:activeKey="activeKey">
                        <a-tab-pane key="1" tab="基本资料">
                            <a-form :model="formState" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }"
                                autocomplete="off" @finish="onFinish" @finishFailed="onFinishFailed">
                                <a-form-item label="手机号码" name="phone"
                                    :rules="[{ required: true, message: 'Please input your username!' }]">
                                    <a-input v-model:value="formState.phone" />
                                </a-form-item>
                                <a-form-item :name="['user', 'email']" label="用户邮箱" :rules="[{ type: 'email' },{required: true}]">
                                    <a-input v-model:value="formState.email" />
                                </a-form-item>
                            </a-form>

                        </a-tab-pane>
                        <a-tab-pane key="2" tab="修改密码">修改密码</a-tab-pane>
                    </a-tabs>
                </a-card>
            </a-col>
        </a-row>
    </div>

</template>
<script setup>
import { ref } from 'vue';
import { reactive } from 'vue';
import {getCurrentUser} from '@/api/user/index.js';
const currentUser = getCurrentUser();

const activeKey = ref('1');
const formState = reactive({
    phone: '',
    password: '',
    mail: ''
});
const onFinish = values => {
    console.log('Success:', values);
};
const onFinishFailed = errorInfo => {
    console.log('Failed:', errorInfo);
};
</script>


<style scoped>
.avatar {
    margin-bottom: 30px;
}
</style>