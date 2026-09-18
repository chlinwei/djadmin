import { computed, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'

import { getMonitorHostGroupTree, getMonitorHostOverview } from '@/api/monitor'
import { createPagination } from '@/util/tableStyle'

// 「纳管目标」主机表的共享机械：左侧主机分组树 + 右侧主机列表（搜索/筛选/分页/行选择），
// 以及列表加载后刷新当前页真实运行态的能力。
//
// 为什么抽出来：exporter 目标与日志采集（Filebeat）目标用的是**同一份主机视角数据**
// （GET /monitor/targets/host-overview/ 一行一台主机，同时带 exporters[] 与 filebeat 子对象），
// 主机树、分页、行选择、状态刷新这些都不区分目标类型。抽成组合式函数后两个页面各自只提供
// 「列定义 + 筛选参数 + 行状态刷新 + 行动作/批量动作」，避免把同一套机械复制两份。
//
// 用法见 views/monitor/index.vue（exporter）与 views/monitor/log-collectors/index.vue（日志采集）。

function parseApiData(resp) {
  return resp?.data?.data || {}
}

// 分组树的 key 约定：顶层为 'all'，分组为 `group-<id>`。
export function collectHostGroupKeys(nodes) {
  const keys = []
  const walk = (list) => {
    ;(list || []).forEach((node) => {
      if (node?.key) keys.push(node.key)
      if (Array.isArray(node?.children) && node.children.length) walk(node.children)
    })
  }
  walk(nodes)
  return keys
}

// 分组树节点：title 里带「已纳管/总数」，与原「纳管目标」页的展示口径一致。
export function buildHostGroupTreeData(groups, keyword, totals) {
  const kw = String(keyword || '').trim().toLowerCase()
  const build = (nodes) => (Array.isArray(nodes) ? nodes : []).reduce((rows, node) => {
    const children = build(node.children)
    const matched = !kw || String(node.name || '').toLowerCase().includes(kw)
    // 自身命中、或子节点命中（children 非空）都要保留，否则搜到深层分组时父级会被裁掉。
    if (matched || children.length) {
      rows.push({
        key: `group-${node.id}`,
        title: `${node.name}（${node.managed_count}/${node.host_count}）`,
        children,
      })
    }
    return rows
  }, [])
  return [{
    key: 'all',
    title: `全部主机（${totals?.managed || 0}/${totals?.total || 0}）`,
    children: build(groups),
  }]
}

export function useHostTargetTable(options = {}) {
  const {
    // 目标类型特有的查询参数（exporter_type/… 或 filebeat_managed/config_state）。
    extraQuery = () => ({}),
    // 列表加载后刷新当前页每行的**真实运行态**（exporter/systemctl 与 Filebeat 各自实现）。
    refreshRowStatuses = null,
    // 数据到达后的钩子（如接住页面级 config_state_error）。
    onLoaded = null,
  } = options

  const hosts = ref([])
  const loading = ref(false)
  const selectedHostIds = ref([])
  const groupTree = ref([])
  const groupTotals = reactive({ total: 0, managed: 0 })
  const groupKeyword = ref('')
  const groupExpandedKeys = ref([])
  const selectedGroupKeys = ref(['all'])
  const keyword = ref('')
  const pagination = reactive(createPagination())

  const groupTreeData = computed(() => buildHostGroupTreeData(groupTree.value, groupKeyword.value, groupTotals))

  const selectedRows = computed(() =>
    hosts.value.filter((item) => selectedHostIds.value.includes(item.host_id)),
  )

  const rowSelection = computed(() => ({
    selectedRowKeys: selectedHostIds.value,
    onChange: (keys) => {
      selectedHostIds.value = keys
    },
  }))

  async function load() {
    loading.value = true
    try {
      const groupKey = selectedGroupKeys.value[0]
      const data = parseApiData(
        await getMonitorHostOverview({
          page: pagination.current,
          page_size: pagination.pageSize,
          group_id: String(groupKey || '').startsWith('group-')
            ? String(groupKey).slice('group-'.length)
            : undefined,
          search: keyword.value.trim() || undefined,
          ...extraQuery(),
        }),
      )
      hosts.value = Array.isArray(data.results) ? data.results : []
      pagination.total = Number(data.count || 0)
      if (typeof onLoaded === 'function') onLoaded(data)
      if (typeof refreshRowStatuses === 'function') await refreshRowStatuses(hosts.value)
    } finally {
      loading.value = false
    }
  }

  async function loadGroupTree() {
    try {
      const data = parseApiData(await getMonitorHostGroupTree())
      groupTree.value = Array.isArray(data.groups) ? data.groups : []
      groupTotals.total = Number(data.total_host_count || 0)
      groupTotals.managed = Number(data.total_managed_count || 0)
      groupExpandedKeys.value = collectHostGroupKeys(groupTreeData.value)
    } catch (error) {
      message.error(error?.response?.data?.msg || error?.message || '主机分组树加载失败')
    }
  }

  // 搜索/筛选变化时回到第一页并清空勾选；勾选态只对当前页有效，翻页/筛选后不清会把
  // 上一页的 id 带进批量请求。
  function reload() {
    pagination.current = 1
    selectedHostIds.value = []
    load()
  }

  function handleTableChange(nextPagination) {
    pagination.current = Number(nextPagination?.current || 1)
    pagination.pageSize = Number(nextPagination?.pageSize || 10)
    selectedHostIds.value = []
    load()
  }

  function handleGroupSelect(keys) {
    // 点已选中的节点时 antd 会回传空数组，这里保持原选中，避免过滤条件被意外清空。
    selectedGroupKeys.value = keys.length ? keys : selectedGroupKeys.value
    reload()
  }

  function clearSelection() {
    selectedHostIds.value = []
  }

  return {
    hosts, loading, selectedHostIds, groupTree, groupTotals, groupKeyword, groupExpandedKeys,
    selectedGroupKeys, keyword, pagination,
    groupTreeData, selectedRows, rowSelection,
    load, reload, loadGroupTree, handleTableChange, handleGroupSelect, clearSelection,
  }
}
