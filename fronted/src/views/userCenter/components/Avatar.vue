<template>
  <div class="avatar-uploader">
    <a-avatar :size="96" :src="imageUrl || undefined" class="avatar-preview">
      <template #icon>
        <SvgIcon name="user" />
      </template>
    </a-avatar>
    <a-upload
      name="avatar"
      :action="uploadAction"
      :headers="headers"
      :show-upload-list="false"
      accept="image/png,image/jpeg,image/gif,image/webp"
      @change="onChangeAvatar"
    >
      <a-button size="small">更换头像</a-button>
    </a-upload>
  </div>
</template>

<script setup>
import { getServerUrl, getMediaUrl } from '@/util/request';
import { getToken, getCurrentUser, saveCurrentUser } from '@/api/user';
import { message } from 'ant-design-vue';
import { ref } from 'vue';

const uploadAction = `${getServerUrl()}/user/changeAvatar`;
const headers = ref({
  Authorization: getToken(),
});
const imageUrl = ref('');

function storedAvatar() {
  const cached = getCurrentUser() || {};
  return cached.avatar || cached.user?.avatar || '';
}

const initialAvatar = storedAvatar();
if (initialAvatar) {
  imageUrl.value = getMediaUrl(initialAvatar);
}

const onChangeAvatar = info => {
  if (info.file.status === 'uploading') {
    return;
  }
  if (info.file.status === 'error') {
    message.error(`${info.file.name} 头像上传失败`);
    return;
  }
  if (info.file.status !== 'done') {
    return;
  }
  const payload = info.file.response?.data || {};
  const avatar = payload.avatar || payload.new_file_name || '';
  if (!avatar) {
    message.error('头像上传响应异常');
    return;
  }
  // 立即回显，并把新文件名写回本地 currentUser 缓存，刷新后仍显示。
  imageUrl.value = getMediaUrl(avatar);
  const cached = getCurrentUser() || {};
  cached.avatar = avatar;
  if (cached.user) {
    cached.user.avatar = avatar;
  }
  saveCurrentUser(cached);
  message.success('头像已更新');
};
</script>

<style scoped>
.avatar-uploader {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}

.avatar-preview {
  background: #f0f2f5;
}
</style>
