<template>

                            <a-upload v-model:file-list="fileList" name="avatar"
                            :action="getServerUrl() + '/user/changeAvatar'" @change="onChangeAvatar" :headers="headers">
                            <img v-if="imageUrl" :src="imageUrl" class="avatar"/>
                            <SvgIcon name="user"  v-else></SvgIcon>
                            <a-button>
                                确认更换
                                </a-button>
                        </a-upload>
    </template>

<script setup>
import { getServerUrl } from '@/util/request';
import { getToken } from '@/api/user/index.js';
import { message } from 'ant-design-vue';
import {ref} from 'vue';
const imageUrl = ref("");
const headers = ref({
    Authorization: getToken()
});
const fileList = ref([]);
const onChangeAvatar = info => {
    if (info.file.status !== 'uploading') {
        console.log(info.file, info.fileList);
    }
    if (info.file.status === 'done') {
        message.success(`${info.file.name} 头像上传成功`);
    } else if (info.file.status === 'error') {
        message.error(`${info.file.name} 头像上传失败`);
    }
}
</script>

<style scoped>

</style>