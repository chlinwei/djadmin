<template>
  <div class="user-notification-chain">
    <a-alert
      v-if="chain.can_receive === true"
      type="success"
      show-icon
      message="你当前会收到告警通知"
      style="margin-bottom: 12px"
    />
    <a-alert
      v-else-if="chain.can_receive === false"
      type="error"
      show-icon
      message="你当前不会收到告警通知"
      style="margin-bottom: 12px"
    >
      <template #description>
        <ul class="issue-list">
          <li v-for="(issue, index) in chain.summary_issues" :key="index">{{ issue }}</li>
        </ul>
      </template>
    </a-alert>

    <a-spin :spinning="loading">
      <a-empty v-if="!loading && !bindings.length" description="暂无告警媒介绑定" />
      <a-timeline v-else class="chain-timeline">
        <a-timeline-item
          v-for="binding in bindings"
          :key="binding.binding_id"
          :color="binding.enabled ? 'green' : 'gray'"
        >
          <div class="chain-node">
            <div class="chain-node-head">
              <span class="chain-node-title">绑定 #{{ binding.binding_id }}</span>
              <a-tag :color="binding.enabled ? 'success' : 'default'">{{ binding.enabled ? '已启用' : '已禁用' }}</a-tag>
              <a-tag v-for="(issue, index) in binding.issues" :key="`b-${index}`" :color="issueColor(issue)" class="issue-tag">{{ issue }}</a-tag>
            </div>

            <div v-if="binding.media" class="chain-child">
              <div class="chain-node-head">
                <span class="chain-node-title">媒介：{{ binding.media.name }}</span>
                <a-tag>{{ binding.media.media_type }}</a-tag>
                <a-tag :color="binding.media.enabled ? 'success' : 'default'">{{ binding.media.enabled ? '已启用' : '已禁用' }}</a-tag>
              </div>
              <div v-if="binding.recipients.length" class="chain-recipients">
                收件人：
                <a-tag v-for="recipient in binding.recipients" :key="recipient" class="recipient-tag">{{ recipient }}</a-tag>
              </div>

              <div v-if="binding.policies.length" class="chain-routes">
                <div v-for="policy in binding.policies" :key="policy.id" class="chain-route">
                  <div class="chain-node-head">
                    <span class="chain-node-title">通知策略：{{ policy.name }}</span>
                    <a-tag color="blue">{{ policy.path }}</a-tag>
                    <a-tag :color="policy.notify_on_firing ? 'red' : 'default'">firing {{ policy.notify_on_firing ? '开' : '关' }}</a-tag>
                    <a-tag :color="policy.notify_on_resolved ? 'green' : 'default'">resolved {{ policy.notify_on_resolved ? '开' : '关' }}</a-tag>
                  </div>
                  <div class="chain-matchers">
                  接收组：
                  <template v-if="policy.user_group_ids">
                    <a-tag v-for="name in policy.user_group_names" :key="name" color="purple">{{ name }}</a-tag>
                    <a-tag v-if="policy.user_in_group === false" color="red">当前用户不在组内，收不到</a-tag>
                  </template>
                  <a-tag v-else color="default">不限</a-tag>
                </div>
                <div v-if="policy.matchers && policy.matchers !== '[]'" class="chain-matchers">matchers：{{ policy.matchers }}</div>
                </div>
              </div>
              <div v-else class="chain-no-route">未被任何通知策略出口命中</div>
            </div>
          </div>
        </a-timeline-item>
      </a-timeline>
    </a-spin>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { getUserNotificationChain } from '@/api/monitor'

defineOptions({
  name: 'UserNotificationChain',
})

// userId 为空表示查看当前登录用户自己的链路（个人中心），传值则为管理员查看指定用户。
const props = defineProps({
  userId: { type: [Number, String], default: null },
})

const loading = ref(false)
const chain = ref({
  can_receive: null,
  summary_issues: [],
  bindings: [],
})

const bindings = computed(() => (Array.isArray(chain.value.bindings) ? chain.value.bindings : []))

// issue 的严重级别展示：轻微问题用橙色，其余红色。
function issueColor(issue) {
  return /警告|未|建议|禁用/.test(String(issue)) ? 'orange' : 'red'
}

async function loadChain() {
  loading.value = true
  try {
    const res = await getUserNotificationChain(props.userId)
    const data = res?.data?.data || {}
    chain.value = {
      can_receive: data.can_receive,
      summary_issues: Array.isArray(data.summary_issues) ? data.summary_issues : [],
      bindings: Array.isArray(data.bindings) ? data.bindings : [],
    }
  } finally {
    loading.value = false
  }
}

onMounted(loadChain)
</script>

<style scoped>
.user-notification-chain {
  width: 100%;
}

.issue-list {
  margin: 4px 0 0;
  padding-left: 18px;
}

.chain-timeline {
  margin-top: 8px;
  padding-left: 4px;
}

.chain-node-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.chain-node-title {
  color: rgba(0, 0, 0, 0.88);
  font-weight: 600;
}

.chain-child {
  margin-top: 8px;
  padding: 8px 12px;
  border: 1px solid #f0f0f0;
  border-radius: 4px;
  background: #fafafa;
}

.chain-routes {
  margin-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.chain-route {
  padding-top: 6px;
  border-top: 1px dashed #f0f0f0;
}

.chain-scope {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
  margin-left: 8px;
  color: rgba(0, 0, 0, 0.65);
  font-size: 12px;
}

.chain-recipients {
  margin-top: 6px;
  color: rgba(0, 0, 0, 0.65);
}

.recipient-tag {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chain-matchers {
  margin-top: 4px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
  word-break: break-all;
}

.chain-no-route {
  margin-top: 6px;
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

.issue-tag {
  margin-inline-end: 0;
}
</style>
