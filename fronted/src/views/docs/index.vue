<template>
  <div class="document-center">
    <a-card class="document-directory" title="文档目录" size="small">
      <a-input-search v-model:value="search" placeholder="搜索文档名称或路径" allow-clear />
      <a-tree
        :expanded-keys="search ? allDirectoryKeys(tree) : expandedKeys"
        :tree-data="tree"
        :selected-keys="[selectedKey]"
        :auto-expand-parent="Boolean(search)"
        block-node
        @select="selectDocument"
        @expand="expandedKeys = $event"
      />
      <a-empty v-if="!tree.length" description="没有匹配的文档" />
    </a-card>
    <a-card class="document-reader" size="small">
      <template #title>
        <span>{{ title }}</span>
        <div class="document-path">{{ currentPath }}</div>
      </template>
      <a-alert v-if="error" type="error" show-icon :message="error" />
      <nav v-if="headings.length" class="document-toc" aria-label="本篇目录">
        <strong>本篇目录</strong>
        <a v-for="heading in headings" :key="heading.id" :href="`#${heading.id}`" @click.prevent="scrollToAnchor(heading.id)">
          {{ heading.title }}
        </a>
      </nav>
      <a-spin :spinning="loading">
        <div ref="reader" class="document-body" @click="followLink">
          <article v-html="html" />
        </div>
      </a-spin>
    </a-card>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  buildDocumentTree, directoryMarkdown, DOCUMENT_HOME, documentLabel, documentLoaders, renderDocument,
} from '@/util/documentCenter'
import { documentAncestorKeys, flattenDocumentCatalog } from '@/util/documentCatalog'
import { renderDocumentDiagrams } from '@/util/documentDiagrams'

const search = ref('')
const expandedKeys = ref(['assets', 'automation', 'monitor', 'system'])
const currentPath = ref(DOCUMENT_HOME)
const selectedKey = ref(DOCUMENT_HOME)
const tree = computed(() => buildDocumentTree(Object.keys(documentLoaders), search.value))
const title = ref('文档中心')
const html = ref('')
const loading = ref(false)
const error = ref('')
const reader = ref(null)
const headings = ref([])
let requestId = 0

function allDirectoryKeys(nodes) {
  return nodes.flatMap((node) => node.children ? [node.key, ...allDirectoryKeys(node.children)] : [])
}

async function scrollToAnchor(anchor) {
  await nextTick()
  if (!anchor) {
    if (reader.value) reader.value.scrollTop = 0
    return
  }
  const target = Array.from(reader.value?.querySelectorAll('[id]') || []).find((node) => node.id === anchor)
  if (target) target.scrollIntoView({ block: 'start' })
  else message.warning(`未找到文档章节：${anchor}`)
}

async function openDocument(path, anchor = '') {
  const exactKey = `${path}${anchor ? `#${anchor}` : ''}`
  const catalog = flattenDocumentCatalog()
  selectedKey.value = catalog.some((node) => node.key === exactKey) ? exactKey
    : catalog.find((node) => node.key.split('#')[0] === path)?.key || path
  const parents = documentAncestorKeys(selectedKey.value) || ['references']
  expandedKeys.value = Array.from(new Set([...expandedKeys.value, ...parents]))
  if (path === currentPath.value && html.value && !error.value) {
    await scrollToAnchor(anchor)
    return
  }
  const id = ++requestId
  currentPath.value = path
  loading.value = true
  error.value = ''
  html.value = ''
  headings.value = []
  title.value = documentLabel(path)
  try {
    const source = Object.hasOwn(documentLoaders, path)
      ? await documentLoaders[path]()
      : directoryMarkdown(path)
    // Rapid tree navigation must not let a slower old import replace the selected document.
    if (id !== requestId) return
    const rendered = renderDocument(source, path)
    const diagramHtml = await renderDocumentDiagrams(rendered.html)
    if (id !== requestId) return
    html.value = diagramHtml
    headings.value = rendered.headings.filter((heading) => heading.level === 2)
    title.value = rendered.headings.find((heading) => heading.level === 1)?.title || documentLabel(path)
    await scrollToAnchor(anchor)
  } catch (cause) {
    if (id === requestId) error.value = `文档加载失败：${cause instanceof Error ? cause.message : String(cause)}`
  } finally {
    if (id === requestId) loading.value = false
  }
}

function selectDocument(keys) {
  if (keys.length) {
    const [path, anchor = ''] = String(keys[0]).split('#')
    openDocument(path, anchor)
  }
}

function followLink(event) {
  const link = event.target.closest('a')
  if (!link || !reader.value?.contains(link)) return
  if (link.hasAttribute('data-document')) {
    event.preventDefault()
    openDocument(link.dataset.document, link.dataset.anchor)
  } else if (link.hasAttribute('data-unavailable')) {
    event.preventDefault()
    message.warning('此链接指向未发布的文件；文档中心不提供源码、协作指令或配置文件。')
  }
}

onMounted(() => openDocument(DOCUMENT_HOME))
</script>

<style scoped>
.document-center {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.document-directory {
  flex: 0 0 300px;
  max-height: calc(100vh - 190px);
  overflow: auto;
}
.document-directory :deep(.ant-tree) {
  margin-top: 12px;
  overflow-wrap: anywhere;
}
.document-reader {
  flex: 1;
  min-width: 0;
}
.document-path {
  color: #64748b;
  font-size: 12px;
  font-weight: normal;
  overflow-wrap: anywhere;
}
.document-toc {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  padding: 12px;
  margin-bottom: 12px;
  background: #f0f5ff;
  border-radius: 6px;
}
.document-body {
  max-height: calc(100vh - 245px);
  min-height: 360px;
  overflow: auto;
  padding: 0 12px;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
.document-body :deep(h1),
.document-body :deep(h2),
.document-body :deep(h3) {
  margin-top: 24px;
  scroll-margin-top: 12px;
}
.document-body :deep(pre) {
  padding: 16px;
  border-radius: 6px;
  background: #f1f5f9;
  overflow-x: auto;
}
.document-body :deep(code) {
  background: #f1f5f9;
  padding: 2px 4px;
  border-radius: 3px;
}
.document-body :deep(pre code) {
  padding: 0;
}
.document-body :deep(table) {
  display: block;
  overflow-x: auto;
  border-collapse: collapse;
  margin: 16px 0;
}
.document-body :deep(th),
.document-body :deep(td) {
  border: 1px solid #e2e8f0;
  padding: 8px 12px;
}
.document-body :deep(blockquote) {
  border-left: 4px solid #1677ff;
  padding: 12px 16px;
  background: #f0f5ff;
  color: #1e3a5f;
}
.document-body :deep(blockquote p:last-child) {
  margin-bottom: 0;
}
.document-body :deep(.document-diagram) {
  overflow-x: auto;
  padding: 16px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #fff;
}
.document-body :deep(.document-diagram svg) {
  min-width: 420px;
  max-width: 100%;
  height: auto;
}
.document-body :deep(h2) {
  padding-bottom: 8px;
  border-bottom: 1px solid #e2e8f0;
}
@media (max-width: 900px) {
  .document-center { flex-direction: column; }
  .document-directory { flex: auto; width: 100%; max-height: 260px; }
  .document-reader { width: 100%; }
}
</style>
