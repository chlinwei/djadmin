<template>
    <div>
        <a-row>
            <a-col :span="6">
                <a-card title="个人信息" :bordered="false" style="width: 300px">
                    <div class="avatar">头像</div>
                    <ul class="list-group">
                        <li class="list-item">
                            <SvgIcon name="user"></SvgIcon>
                          <div class="iten-wrapper">
                          
                           <div class="item-name"><span>用户名称</span></div>  
                            <div class="item-value"><span>{{ currentUser.user.username }}</span></div>
                          </div>
                          
                        </li>
                        <li class="list-item">
                            <SvgIcon name="phone"></SvgIcon>
                            <div class="iten-wrapper">
                            <div class="item-name">电话</div>
                            <div class="item-value">{{ currentUser.user.phonenumber  }}</div>
                          </div>
                    
                        </li>
                        <li class="list-item">
                            <SvgIcon name="email"></SvgIcon>
                            <div class="iten-wrapper">
                            <div class="item-name">邮箱</div>
                            <div class="item-value">{{ currentUser.user.email }}</div>
                          </div>
   
                        </li>
                        <li class="list-item">
                            <SvgIcon name="peoples"></SvgIcon>
                            <div class="iten-wrapper">
                            <div class="item-name">角色</div>
                            <div class="item-value">{{roleList}}</div>
                          </div>
                        </li>
                        <li class="list-item">
                            <SvgIcon name="date"></SvgIcon>
                            <div class="iten-wrapper">
                            <div class="item-name">创建时间</div>
                            <div class="item-value">{{ currentUser.user.create_time }}</div>
                          </div>
                        </li>


                    </ul>
                </a-card>
            </a-col>
            <a-col :span="18">
                <a-card title="基本资料" :bordered="false" style="width: 300px">
                    <a-tabs v-model:activeKey="activeKey">
                        <a-tab-pane key="1" tab="基本资料">
                            <a-form :model="formState" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }"
                                autocomplete="off" @finish="onFinish_updateUserInfo" @finishFailed="onFinishFailed_updateUserInfo">
                                <a-form-item label="手机号码" name="phonenumber"
                                    :rules="[{ required: true, message: '请输入电话号码!' }]">
                                    <a-input v-model:value="formState.phonenumber" />
                                </a-form-item>
                                <a-form-item :name="['email']" label="用户邮箱" :rules="[{ type: 'email' },{required: true}]">
                                    <a-input v-model:value="formState.email" />
                                </a-form-item>
                                <a-form-item>
                                    <a-button type="primary" html-type="submit"  style="margin-left: 10px;">保存</a-button>
                                </a-form-item>
                                
                            </a-form>

                        </a-tab-pane>
                        <a-tab-pane key="2" tab="修改密码" >
                            <a-form :model="password_formState" name="basic" :label-col="{ span: 8 }" :wrapper-col="{ span: 16 }"
                            autocomplete="off" @finish="onFinish_updateUserPassword" @finishFailed="onFinishFailed_updateUserPassword" :rules="password_rules">
                            <a-form-item name="old_password" label="旧密码">
                                    <a-input-password v-model:value="password_formState.old_password" />
                                </a-form-item>
                                <a-form-item name="new_password" label="新密码">
                                    <a-input-password v-model:value="password_formState.new_password" />
                                </a-form-item>
                                <a-form-item name="confirm_password" label="确认密码">
                                    <a-input-password v-model:value="password_formState.confirm_password" />
                                </a-form-item>
                                <a-form-item>
                                    <a-button type="primary" html-type="submit"  style="margin-left: 10px;">保存</a-button>
                                </a-form-item>
                            </a-form>

                        </a-tab-pane>
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
import { getCurrentUserRoleList } from '@/api/role';
import {updateUserInfo} from '@/api/user';
import { onMounted } from 'vue';
import { message } from 'ant-design-vue';

defineOptions({
    name: 'userCenter'
})
const currentUser = reactive({'user':getCurrentUser()});

const activeKey = ref('1');
const formState = reactive({
    phonenumber: currentUser.user.phonenumber,
    email: currentUser.user.email
});
const password_formState = reactive({
    old_password: '',
    new_password: '',
    confirm_password: ''
})
const onFinish_updateUserInfo = values => {
    updateUserInfo(values,(result) => {
        currentUser.user = getCurrentUser();
        message.success("更新成功")
    });
};
const onFinishFailed_updateUserInfo = errorInfo => {
    console.log('Failed:', errorInfo);
};


const onFinish_updateUserPassword= values => {
    console.log(values)
};
const onFinishFailed_updateUserPassword = errorInfo => {
    console.log('Failed:', errorInfo);
};
var   roleList = ref('');

onMounted(() => {
    // 初始化基本资料
    formState.phonenumber = currentUser.user.phonenumber;
    formState.email = currentUser.user.email;
    getCurrentUserRoleList().then(result =>{
      result.data.data.roleList.forEach(element => {
        roleList.value = roleList.value + element.name + ' ';
        
      });
    })
    
})

const check_confirm_password = async(_rule,value) => {
    if(value != password_formState.new_password) {
        return Promise.reject('新密码和旧密码不一致!')
    }
}

//密碼規則
const password_rules = {
    old_password: [
        { required: true, message: '请输入旧密码!' }
    ],
    new_password: [
    { required: true, message: '请输入新密码!' }
    ],
    confirm_password: [
    { required: true, message: '请输入确认密码!' },
    {validator: check_confirm_password,trigger: 'change'}
    ]
}
</script>


<style scoped>
.avatar {
    margin-bottom: 30px;
}


.list-group > .list-item{
    margin-bottom: 1px sold red !important;
    margin-top: 1px sold #e7eaec;
    margin-bottom: 2px;;
    padding: 11px 0;
    vertical-align: middle;
    display: flex;

}
.item-wrapper {
    vertical-align: middle;
}

.iten-wrapper {
    display: flex;
    width: 100%;
    justify-content: space-around;
    height: 16px;
    align-items: center;
 
}
.item-name {
    padding-left: 10px;
    width: 30%;
}
.item-value {
    width: 70%;
    text-align: right;
    
}
.item-name>span {
    vertical-align: middle;
    height:100%;
}
.item-value>span {
    vertical-align: middle;
    height:100%;
}

.text {
    height: 16px;
}
</style>