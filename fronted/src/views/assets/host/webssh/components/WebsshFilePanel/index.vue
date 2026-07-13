<template>
    <div
        v-if="showFilePanel"
        :ref="setFilePanelRef"
        class="file-panel"
        :class="{ 'file-panel-offline': !fileOperationsEnabled }"
        :style="{ width: `${filePanelWidth}px` }"
    >
        <div class="file-toolbar">
            <a-button size="small" :disabled="!fileOperationsEnabled || !previousDirectoryPath" @click="emit('go-parent-dir')">↑</a-button>
            <a-button size="small" :loading="fileLoading" :disabled="!fileOperationsEnabled" @click="emit('refresh-files')">刷新</a-button>
            <a-input
                :value="fileFilterKeyword"
                size="small"
                allow-clear
                class="file-filter-input"
                placeholder="过滤当前目录"
                :disabled="!fileOperationsEnabled"
                @update:value="emit('update:file-filter-keyword', $event)"
            />
            <a-button size="small" :disabled="!fileOperationsEnabled" @click="emit('create-directory')">新建目录</a-button>
            <a-upload :show-upload-list="false" :disabled="!fileOperationsEnabled" :custom-request="(options) => emit('upload-file', options)">
                <a-button size="small" :disabled="!fileOperationsEnabled">上传文件</a-button>
            </a-upload>
        </div>
        <div class="file-current-path">
            <span>路径：</span>
            <a-input
                :value="filePathInput"
                size="small"
                placeholder="输入目录路径后按 Enter"
                :disabled="!fileOperationsEnabled"
                @update:value="emit('update:file-path-input', $event)"
                @pressEnter="emit('handle-path-enter')"
            />
        </div>
        <div v-if="fileErrorText" class="file-error">{{ fileErrorText }}</div>
        <div :ref="setFileTableWrapRef" class="file-table-wrap">
            <a-table
                :columns="fileColumns"
                :data-source="filteredFileEntries"
                :loading="fileLoading"
                :pagination="false"
                row-key="path"
                size="small"
                :scroll="{ y: fileTableScrollY }"
                :custom-row="onBindFileRowEvents"
            >
                <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'name'">
                        <a-button
                            v-if="record.is_dir"
                            type="link"
                            size="small"
                            class="file-link-btn"
                            :disabled="!fileOperationsEnabled"
                            @click="emit('open-directory', record.path)"
                        >
                            📁 {{ record.name }}
                        </a-button>
                        <span v-else>📄 {{ record.name }}</span>
                    </template>
                    <template v-else-if="column.key === 'size'">
                        {{ onFormatFileSize(record.size) }}
                    </template>
                    <template v-else-if="column.key === 'mtime'">
                        {{ onFormatFileMtime(record.mtime) }}
                    </template>
                </template>
            </a-table>
        </div>
        <div
            v-if="fileContextMenuVisible"
            class="file-context-menu"
            :style="fileContextMenuStyle"
            @click.stop
        >
            <div
                v-for="action in fileContextMenuActions"
                :key="action.key"
                class="file-context-menu-item"
                :class="{ danger: action.danger }"
                @click="emit('handle-file-context-action', action.key)"
            >
                {{ action.label }}
            </div>
        </div>
    </div>
</template>

<script setup>
const emit = defineEmits([
    'update:file-filter-keyword',
    'update:file-path-input',
    'go-parent-dir',
    'refresh-files',
    'create-directory',
    'upload-file',
    'handle-path-enter',
    'open-directory',
    'handle-file-context-action',
])

defineProps({
    showFilePanel: { type: Boolean, default: true },
    setFilePanelRef: { type: Function, required: true },
    fileOperationsEnabled: { type: Boolean, default: false },
    filePanelWidth: { type: Number, default: 380 },
    fileFilterKeyword: { type: String, default: '' },
    fileLoading: { type: Boolean, default: false },
    previousDirectoryPath: { type: String, default: '' },
    filePathInput: { type: String, default: '' },
    fileErrorText: { type: String, default: '' },
    setFileTableWrapRef: { type: Function, required: true },
    fileColumns: { type: Array, default: () => [] },
    filteredFileEntries: { type: Array, default: () => [] },
    fileTableScrollY: { type: Number, default: 520 },
    onBindFileRowEvents: { type: Function, required: true },
    onFormatFileSize: { type: Function, required: true },
    onFormatFileMtime: { type: Function, required: true },
    fileContextMenuVisible: { type: Boolean, default: false },
    fileContextMenuStyle: { type: Object, default: () => ({}) },
    fileContextMenuActions: { type: Array, default: () => [] },
})
</script>

<style src="./style.css"></style>
