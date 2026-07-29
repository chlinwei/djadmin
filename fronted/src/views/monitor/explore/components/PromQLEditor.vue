<template>
  <div ref="editorRoot" class="promql-editor"></div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { autocompletion } from '@codemirror/autocomplete'
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { PromQLExtension } from '@prometheus-io/codemirror-promql'

const props = defineProps({
  modelValue: {
    type: String,
    default: '',
  },
  completionRemoteUrl: {
    type: String,
    default: '',
  },
})

const emit = defineEmits(['update:modelValue', 'run'])

const editorRoot = ref(null)
let editorView = null

function buildPromQLExtension() {
  let promQLExtension = new PromQLExtension()

  if (props.completionRemoteUrl) {
    promQLExtension = promQLExtension.setComplete({
      remote: {
        url: props.completionRemoteUrl,
        // 每次请求都动态读取最新 token，避免登录态刷新后仍沿用旧值。
        fetchFn: (url, options = {}) => {
          const token = localStorage.getItem('token') || ''
          const headers = new Headers(options?.headers || {})
          if (token) {
            // 与后端 JwtAuthenticationMiddleware 对齐：读取 AUTHORIZATION。
            headers.set('AUTHORIZATION', token)
          }
          // 防止 baseUrl 与 sdk 拼接时出现 proxy//api 的双斜杠。
          const normalizedUrl = String(url).replace('/prometheus/proxy//', '/prometheus/proxy/')
          return fetch(normalizedUrl, {
            ...options,
            headers,
            credentials: 'same-origin',
          })
        },
      },
      maxMetricsMetadata: 10000,
    })
  }

  return promQLExtension.asExtension()
}

function createEditor(initialDoc) {
  if (!editorRoot.value) {
    return
  }

  const extensions = [
    // 通过 EditorView.theme 直接控制编辑器内部高度，确保整个容器区域都可点击
    EditorView.theme({
      '&': { height: '60px' },
      '.cm-scroller': { overflow: 'auto', fontFamily: 'Menlo, Consolas, Monaco, monospace', fontSize: '13px' },
    }),
    // 不使用 @codemirror/basic-setup，避免不同版本 @codemirror/state 并存时的 extension set 冲突。
    EditorView.lineWrapping,
    // CodeMirror 官方的括号自动配对功能
    closeBrackets(),
    // 仅启用补全能力：PromQL 提供候选源，CodeMirror 负责触发与展示补全面板。
    autocompletion({ activateOnTyping: true }),
    keymap.of([
      ...closeBracketsKeymap,
      // 无补全面板时 Enter 触发查询
      { key: 'Enter', run: () => { emit('run'); return true } },
    ]),
    buildPromQLExtension(),
    EditorView.updateListener.of((update) => {
      if (!update.docChanged) {
        return
      }
      emit('update:modelValue', update.state.doc.toString())
    }),
  ]

  editorView = new EditorView({
    parent: editorRoot.value,
    state: EditorState.create({
      doc: initialDoc,
      extensions,
    }),
  })
}

function recreateEditor() {
  const currentValue = editorView ? editorView.state.doc.toString() : props.modelValue
  if (editorView) {
    editorView.destroy()
    editorView = null
  }
  createEditor(currentValue)
}

onMounted(() => {
  createEditor(props.modelValue || '')
})

watch(
  () => props.modelValue,
  (nextValue) => {
    if (!editorView) {
      return
    }
    const current = editorView.state.doc.toString()
    if ((nextValue || '') === current) {
      return
    }
    editorView.dispatch({
      changes: {
        from: 0,
        to: current.length,
        insert: nextValue || '',
      },
    })
  },
)

watch(
  () => props.completionRemoteUrl,
  (nextValue, prevValue) => {
    if (nextValue === prevValue || !editorRoot.value) {
      return
    }
    recreateEditor()
  },
)

onBeforeUnmount(() => {
  if (editorView) {
    editorView.destroy()
    editorView = null
  }
})
</script>

<style scoped>
.promql-editor {
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  overflow: visible;
}

.promql-editor :deep(.cm-tooltip-autocomplete) {
  z-index: 2000;
}
</style>
