<template>
    <div>

        <!-- Context-bound select popups need stable Teleport anchors during the close transition. -->
        <a-modal cancelText="取消" okText="保存" :open="props.open" :title="props.title" width="1000px"
            :bodyStyle="{ maxHeight: '72vh', overflowY: 'auto' }"
            @ok="handleOk" @cancel="handleCancel" @afterClose="handleAfterClose">

            <a-spin :spinning="loading">
            <a-form :model="form" ref="formRef" name="basic" :label-col="{ span: 8 }"
                :wrapper-col="{ span: 16 }" autocomplete="off" :rules="get_rules(form)">
                <a-form-item name="name" label="应用名称">
                    <a-input v-model:value="form.name" />
                </a-form-item>
                <a-form-item name="code" label="应用编码">
                    <a-input v-model:value="form.code" />
                </a-form-item>
                <a-form-item name="category" label="应用类别">
                    <a-select v-model:value="form.category" :options="categoryOptions" :getPopupContainer="getPopupContainer" />
                </a-form-item>
                <a-form-item name="vendor" label="厂商">
                    <a-input v-model:value="form.vendor" />
                </a-form-item>
                <a-form-item name="description" label="描述">
                    <a-textarea v-model:value="form.description" />
                </a-form-item>
                <a-form-item label="允许使用">
                    <a-switch v-model:checked="form.enabled" />
                </a-form-item>
                <a-form-item name="remark" label="备注">
                    <a-textarea v-model:value="form.remark" />
                </a-form-item>
                <a-divider>应用基线</a-divider>
                <div class="baseline-toolbar">
                    <span class="baseline-count">共 {{ form.baseline_checks.length }} 项</span>
                    <a-select
                        v-model:value="selectedBaselineType"
                        class="baseline-type-select"
                        :options="fileTypeOptions"
                        :getPopupContainer="getPopupContainer"
                    />
                    <a-button size="large" @click="addBaselineCheck">
                        <template #icon><PlusCircleOutlined /></template>
                        新增检查项
                    </a-button>
                </div>
                <a-empty v-if="form.baseline_checks.length === 0" description="暂无检查项" />
                <div v-else class="baseline-editor-layout">
                    <div class="baseline-check-list">
                        <div
                            v-for="(check, index) in form.baseline_checks"
                            :key="check.local_id"
                            class="baseline-check-item"
                            :class="{ active: check.local_id === activeBaselineCheckId }"
                            role="button"
                            tabindex="0"
                            @click="activeBaselineCheckId = check.local_id"
                            @keydown.enter="activeBaselineCheckId = check.local_id"
                        >
                            <div class="baseline-check-label">
                                <strong>{{ check.name || `检查项 ${index + 1}` }}</strong>
                                <span>{{ fileTypeLabels[check.document_type] }} · {{ schemaTypeLabels[check.schema_type] }}</span>
                            </div>
                            <a-tooltip title="删除" placement="top">
                                <a-button class="delBtn" danger shape="circle" size="small" @click.stop="removeBaselineCheck(check.local_id)">
                                    <template #icon><DeleteOutlined /></template>
                                </a-button>
                            </a-tooltip>
                        </div>
                    </div>
                    <div v-if="activeBaselineCheck" class="baseline-check-editor">
                    <a-form-item label="文档类型" required>
                        <a-select
                            v-model:value="activeBaselineCheck.document_type"
                            :options="fileTypeOptions"
                            :getPopupContainer="getPopupContainer"
                            @change="changeDocumentType"
                        />
                    </a-form-item>
                    <a-form-item label="校验类型" required>
                        <a-select
                            v-model:value="activeBaselineCheck.schema_type"
                            class="schema-type-select"
                            placeholder="选择校验类型"
                            :options="activeSchemaTypeOptions"
                            :getPopupContainer="getPopupContainer"
                            @change="applySchemaType"
                        />
                    </a-form-item>
                    <a-alert
                        v-if="activeBaselineCheck.schema_description"
                        :message="activeBaselineCheck.schema_description"
                        type="info"
                        show-icon
                        class="schema-description"
                    />
                    <a-form-item label="检查项名称" required>
                        <a-input v-model:value="activeBaselineCheck.name" placeholder="例如 XML 属性合规检查" />
                    </a-form-item>
                    <a-form-item label="启用检查">
                        <a-switch v-model:checked="activeBaselineCheck.enabled" />
                    </a-form-item>
                    <a-form-item label="文件路径" required>
                        <a-input v-model:value="activeBaselineCheck.file_path" placeholder="例如 ${APP_HOME}/conf/application.xml" />
                    </a-form-item>
                    <a-form-item label="规则版本" required>
                        <a-input v-model:value="activeBaselineCheck.schema_version" disabled />
                    </a-form-item>
                    <a-form-item label="校验规则" required>
                        <a-textarea v-model:value="activeBaselineCheck.schema_content" class="schema-editor" :auto-size="{ minRows: 12, maxRows: 18 }" />
                    </a-form-item>
                    </div>
                </div>
            </a-form>
            </a-spin>
        </a-modal>
    </div>
</template>
<script setup>
import { computed, ref, watch } from 'vue';
import { DeleteOutlined, PlusCircleOutlined } from '@ant-design/icons-vue'
import { resolvePopupContainerByContext } from '@/util/popupContainer'
import { openDeleteConfirm } from '@/util/deleteConfirm'

const getPopupContainer = (triggerNode) => resolvePopupContainerByContext(triggerNode)
const categoryOptions = [
    { label: 'Web 容器', value: 'web_container' },
    { label: '数据库', value: 'database' },
    { label: '中间件', value: 'middleware' },
    { label: '业务应用', value: 'business' },
    { label: '其他', value: 'other' },
]
const fileTypeOptions = [
    { label: 'XML', value: 'xml' },
    { label: 'JSON', value: 'json' },
    { label: 'YAML', value: 'yaml' },
    { label: 'INI', value: 'ini' },
    { label: 'TOML', value: 'toml' },
    { label: 'Properties', value: 'properties' },
    { label: '普通文本', value: 'text' },
]
const fileTypeLabels = Object.fromEntries(fileTypeOptions.map(({ value, label }) => [value, label]))
const schemaDefinitions = {
    schematron: {
                label: 'Schematron / XPath',
        version: 'iso',
        description: '使用 XPath 表达 XML 阈值、条件关系和禁止项等业务基线。',
        content: `<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xslt">
  <pattern id="application-baseline">
    <rule context="/">
      <assert test="not(//setting[@name='allow-all' and @enabled='true'])">
        禁止启用 allow-all 配置
      </assert>
    </rule>
  </pattern>
</schema>`,
    },
    json_schema: {
        label: 'JSON Schema',
        version: '2020-12',
        description: '使用 JSON Schema 2020-12 校验 JSON、YAML、INI、TOML 或 Properties 的结构、类型、范围和条件。',
        content: JSON.stringify({
            $schema: 'https://json-schema.org/draft/2020-12/schema',
            type: 'object',
            properties: {
                maxPostSize: { type: 'integer', minimum: 524288000 },
            },
            required: ['maxPostSize'],
            additionalProperties: true,
        }, null, 2),
    },
    regexp: {
        label: 'Regexp',
        version: 're2',
        description: '使用 Go RE2 正则检查普通文本；expect 为 present 时要求匹配，absent 时禁止匹配。',
        content: JSON.stringify({
            pattern: '(?m)^debug\\s*=\\s*false\\s*$',
            expect: 'present',
        }, null, 2),
    },
}
const schemaTypesByDocument = {
    xml: ['schematron'],
    json: ['json_schema'],
    yaml: ['json_schema'],
    ini: ['json_schema'],
    toml: ['json_schema'],
    properties: ['json_schema'],
    text: ['regexp'],
}
const schemaTypeLabels = Object.fromEntries(Object.entries(schemaDefinitions).map(([key, value]) => [key, value.label]))
let nextBaselineCheckId = 1
const createBaselineCheck = (source = {}) => ({
    local_id: nextBaselineCheckId++,
    name: '',
    file_path: '${APP_HOME}/conf/',
    document_type: 'xml',
    schema_type: 'schematron',
    schema_version: 'iso',
    schema_content: '',
    enabled: true,
    ...source,
    schema_description: schemaDefinitions[source.schema_type || 'schematron']?.description || '',
})
const formRef = ref(null)
const loading = ref(false)
const props = defineProps(
    {
        open: {
            type: Boolean,
            default: false,
            required: true
        },
        title: {
            type: String,
            default: '错误界面',
            required: true
        },
        item_id: {
            type: Number,
            default: -1,
            required: true
        },
        appname: {
            type: String,
            default: '应用',
            required: true
        }
    }
)


const get_rules = (obj) => {
    var add_rules = {
        name: [
            { required: true, message: "必填字段" }
        ],
        code: [
            { required: true, message: "必填字段" }
        ],
    }
    var edit_rules = {
        name: [
            { required: true, message: "必填字段" }
        ],
        code: [
            { required: true, message: "必填字段" }
        ],
    }
    if(obj.id == -1) {
        return add_rules
    }else {
        return edit_rules
    }
}


const createInitialForm = () => ({
    id: -1,
    name: '',
    code: '',
    category: 'other',
    baseline_checks: [],
    vendor: '',
    description: '',
    enabled: true,
    remark: '',
})

const form = ref(createInitialForm())
const selectedBaselineType = ref('xml')
const activeBaselineCheckId = ref(null)
const activeBaselineCheck = computed(() => (
    form.value.baseline_checks.find((check) => check.local_id === activeBaselineCheckId.value) || null
))
const activeSchemaTypeOptions = computed(() => (
    (schemaTypesByDocument[activeBaselineCheck.value?.document_type] || []).map((value) => ({
        value,
        label: schemaDefinitions[value].label,
    }))
))
let detailLoadToken = 0

const addBaselineCheck = () => {
    const documentType = selectedBaselineType.value
    const schemaType = schemaTypesByDocument[documentType][0]
    const definition = schemaDefinitions[schemaType]
    const check = createBaselineCheck({
        document_type: documentType,
        schema_type: schemaType,
        schema_version: definition.version,
        schema_content: definition.content,
        schema_description: definition.description,
    })
    form.value.baseline_checks.unshift(check)
    activeBaselineCheckId.value = check.local_id
}

const applySchemaType = (schemaType) => {
    const definition = schemaDefinitions[schemaType]
    if (!definition || !activeBaselineCheck.value) return
    Object.assign(activeBaselineCheck.value, {
        schema_type: schemaType,
        schema_version: definition.version,
        schema_content: definition.content,
        schema_description: definition.description,
    })
}

const changeDocumentType = (documentType) => {
    if (!activeBaselineCheck.value) return
    const schemaType = schemaTypesByDocument[documentType][0]
    applySchemaType(schemaType)
}

const removeBaselineCheck = (localId) => {
    const index = form.value.baseline_checks.findIndex((check) => check.local_id === localId)
    if (index < 0) return
    const check = form.value.baseline_checks[index]
    openDeleteConfirm({
        title: '删除检查项',
        summary: '确认从应用基线中删除以下检查项？',
        items: [check?.name || `检查项 ${index + 1}`],
        onConfirm: () => {
            form.value.baseline_checks.splice(index, 1)
            if (activeBaselineCheckId.value === localId) {
                activeBaselineCheckId.value = form.value.baseline_checks[index]?.local_id
                    || form.value.baseline_checks[index - 1]?.local_id
                    || null
            }
        },
    })
}


const emits = defineEmits(['update:open', 'initList'])
import { SaveOrCreateApplication,getApplicationById } from '@/api/assets/application/index.js';
const getItemById  = (id) =>{
    return getApplicationById(id)
}



const handleOk = (e) => {
    const res = formRef.value?.validate().then((r1) => {
        let baselineChecks
        try {
            baselineChecks = form.value.baseline_checks.map((check, index) => {
                if (!check.document_type || !check.schema_type) {
                    throw new Error(`请选择检查项 ${index + 1} 的文档类型和校验类型`)
                }
                if (!String(check.name || '').trim() || !String(check.file_path || '').trim()) {
                    throw new Error(`请完整填写检查项 ${index + 1} 的名称和文件路径`)
                }
                if (!String(check.schema_content || '').trim()) {
                    throw new Error(`检查项 ${index + 1} 的校验规则不能为空`)
                }
                return {
                    name: String(check.name).trim(),
                    file_path: String(check.file_path).trim(),
                    document_type: check.document_type,
                    schema_type: check.schema_type,
                    schema_version: check.schema_version,
                    schema_content: check.schema_content,
                    enabled: check.enabled,
                    order: (index + 1) * 10,
                }
            })
        } catch (error) {
            message.error(error.message)
            return
        }
        const obj = { ...form.value, baseline_checks: baselineChecks };
        if (obj.id == -1) {
            SaveOrCreateApplication(obj).then(result => {
                message.success("新增"+ props.appname+"成功");
                emits('initList')
                emits('update:open', false)
            })
        } else {
            SaveOrCreateApplication(obj).then(result => {
                message.success("保存"+ props.appname +"成功");
                emits('initList')
                emits('update:open', false);
            })
        }
    })

};



watch(
    [() => props.open, () => props.item_id],
    ([open, id]) => {
        const currentToken = ++detailLoadToken
        if (!open) {
            loading.value = false
            return
        }
        if (id === -1) {
            loading.value = false
            form.value = createInitialForm()
            selectedBaselineType.value = 'xml'
            activeBaselineCheckId.value = null
            return
        }

        loading.value = true
        getItemById(id).then(res => {
            if (currentToken !== detailLoadToken || !props.open || props.item_id !== id) return
            const data = res.data.data || {}
            const baselineChecks = (data.baseline_checks || []).map((check) => createBaselineCheck(check))
            form.value = {
                ...createInitialForm(),
                ...data,
                baseline_checks: baselineChecks,
            }
            activeBaselineCheckId.value = baselineChecks[0]?.local_id || null
        }).finally(() => {
            if (currentToken === detailLoadToken) loading.value = false
        })
    },
)



import { message } from 'ant-design-vue';
// 取消窗口
const handleCancel = () => {
    detailLoadToken += 1
    emits('update:open', false);
}

const handleAfterClose = () => {
    form.value = createInitialForm()
    selectedBaselineType.value = 'xml'
    activeBaselineCheckId.value = null
    formRef.value = null
}
</script>
<style scoped>
.baseline-toolbar {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
    margin-bottom: 16px;
}

.baseline-count {
    color: rgba(0, 0, 0, 0.45);
}

.baseline-type-select {
    width: 120px;
}

.schema-type-select {
    width: 100%;
}

.schema-description {
    margin-bottom: 16px;
}

.baseline-editor-layout {
    display: grid;
    grid-template-columns: 260px minmax(0, 1fr);
    min-height: 430px;
    max-height: 520px;
    margin-bottom: 18px;
    border: 1px solid #d9d9d9;
}

.baseline-check-list {
    overflow-y: auto;
    border-right: 1px solid #d9d9d9;
    background: #fafafa;
}

.baseline-check-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    min-height: 58px;
    padding: 10px 12px;
    border-bottom: 1px solid #f0f0f0;
    cursor: pointer;
}

.baseline-check-item:hover,
.baseline-check-item.active {
    background: #e6f4ff;
}

.baseline-check-item.active {
    box-shadow: inset 3px 0 0 #1677ff;
}

.baseline-check-label {
    min-width: 0;
}

.baseline-check-label strong,
.baseline-check-label span {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.baseline-check-label span {
    margin-top: 3px;
    color: rgba(0, 0, 0, 0.45);
    font-size: 12px;
}

.baseline-check-editor {
    overflow-y: auto;
    padding: 18px 20px 4px;
}

:deep(.schema-editor textarea) {
    font-family: "JetBrains Mono", "Cascadia Code", monospace;
    font-size: 13px;
    line-height: 1.55;
}
</style>