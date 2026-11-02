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
      :before-upload="beforeUpload"
      accept="image/png,image/jpeg,image/gif,image/webp"
      @change="onChangeAvatar"
    >
      <a-button size="small">更换头像</a-button>
    </a-upload>
    <div class="avatar-hint">原图不超过 5 MiB，自动处理后存储不超过 1 MiB</div>
    <a-alert v-if="uploadError" class="avatar-error" type="error" show-icon :message="uploadError" />
  </div>
</template>

<script setup>
import { getServerUrl, getMediaUrl } from '@/util/request';
import { getToken, getCurrentUser, saveCurrentUser } from '@/api/user';
import { message, Upload } from 'ant-design-vue';
import { ref } from 'vue';

const uploadAction = `${getServerUrl()}/user/changeAvatar`;
const headers = ref({
  Authorization: getToken(),
  // 上传不需要此标记；显式关闭 Antd 默认附加的请求头，避免多余的跨域预检要求。
  'X-Requested-With': null,
});
const imageUrl = ref('');
const uploadError = ref('');

function reportUploadError(reason) {
  uploadError.value = reason;
  message.error(reason);
}

function beforeUpload(file) {
  if (file.size > 5 * 1024 * 1024) {
    reportUploadError('头像原文件不得超过 5 MiB');
    return Upload.LIST_IGNORE;
  }
  uploadError.value = '';
  return true;
}

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
    uploadError.value = '';
    return;
  }
  if (info.file.status === 'error') {
    // Antd keeps non-2xx response bodies on file.response, not in the normal "done" branch.
    const status = Number(info.file.error?.status);
    const hasStatus = Number.isInteger(status) && status > 0;
    const serverMessage = info.file.response?.msg;
    let reason;
    if (typeof serverMessage === 'string' && serverMessage.trim()) {
      reason = serverMessage;
    } else if (status === 413) {
      reason = '上传请求体过大，被服务器或代理拒绝';
    } else if (hasStatus) {
      reason = '服务器未返回可读的错误原因，请查看后端或代理日志';
    } else {
      reason = '未收到服务器响应，请检查网络、跨域配置或服务是否可用';
    }
    reportUploadError(`头像上传失败：${reason}${hasStatus ? `（HTTP ${status}）` : ''}`);
    return;
  }
  if (info.file.status !== 'done') {
    return;
  }
  const result = info.file.response;
  if (result?.code !== 200) {
    reportUploadError(result?.msg || '头像上传失败');
    return;
  }
  const avatar = result.data?.avatar;
  if (typeof avatar !== 'string' || !avatar) {
    reportUploadError('头像上传响应异常');
    return;
  }
  // 缓存带目录的数据库头像地址，个人中心和顶栏使用相同的 URL 解析方式。
  imageUrl.value = getMediaUrl(avatar);
  const cached = getCurrentUser() || {};
  cached.avatar = avatar;
  if (cached.user) {
    cached.user.avatar = avatar;
  }
  saveCurrentUser(cached);
  uploadError.value = '';
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

.avatar-hint {
  font-size: 12px;
}

.avatar-error {
  max-width: 320px;
  overflow-wrap: anywhere;
}
</style>
