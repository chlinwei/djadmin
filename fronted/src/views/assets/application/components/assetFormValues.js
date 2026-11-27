// 资产宏没有单独的密文控件；查看时不要把凭据类宏值绑定到 DOM（包括 disabled input）。
export function isSensitiveMacro(name) {
  return /pass(?:word|phrase|wd)|secret|token|(?:private|access|api)[_-]?key|credential/i.test(name)
}

export function displayMacro(name, value, readOnly) {
  return readOnly && value && isSensitiveMacro(name) ? '••••••' : value
}

// 详情可能含运行快照等非表单字段，只接收表单声明的键，防止编辑时原样回传。
export function formFields(defaults, detail = {}) {
  return Object.fromEntries(Object.entries(defaults).map(([key, initial]) =>
    [key, detail != null && Object.hasOwn(detail, key) ? detail[key] : initial]))
}
