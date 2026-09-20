import { describe, expect, it } from 'vitest'

import { resolvePathMacros, unexpandedMacros, hasGlobMeta } from './logPathMacro'

// 前端这套展开是服务端 internal/shared/logmacro 的移植：合并顺序必须逐字对齐，
// 否则会出现"界面显示的路径与主机上实际采的不一样"（比显示占位符更糟）。
const templateMacros = [
  { name: 'APP_HOME', value: '/opt/default-tomcat' },
  { name: 'LOG_DIR', value: '/opt/default-tomcat/logs' },
]

describe('resolvePathMacros', () => {
  it('模板默认值先生效', () => {
    expect(resolvePathMacros('${LOG_DIR}/catalina.out', { templateMacros }))
      .toBe('/opt/default-tomcat/logs/catalina.out')
  })

  it('服务级 macro_values 覆盖模板默认', () => {
    expect(resolvePathMacros('${LOG_DIR}/catalina.out', {
      templateMacros,
      serviceMacros: { LOG_DIR: '/home/esb/tomcat/logs' },
    })).toBe('/home/esb/tomcat/logs/catalina.out')
  })

  // 与后端同序：模板 app_home 先覆盖模板默认里的 APP_HOME，服务级再覆盖它。
  it('app_home 作为 APP_HOME 的默认值，但服务级 APP_HOME 优先', () => {
    expect(resolvePathMacros('${APP_HOME}/logs/x.log', {
      templateMacros,
      appHome: '/srv/tomcat',
    })).toBe('/srv/tomcat/logs/x.log')
    expect(resolvePathMacros('${APP_HOME}/logs/x.log', {
      templateMacros,
      appHome: '/srv/tomcat',
      serviceMacros: { APP_HOME: '/home/esb/tomcat' },
    })).toBe('/home/esb/tomcat/logs/x.log')
  })

  it('展开不出来的宏保持占位符（实例级宏由 unexpandedMacros 标出来）', () => {
    const resolved = resolvePathMacros('${INSTANCE_DIR}/x.log', { templateMacros })
    expect(resolved).toBe('${INSTANCE_DIR}/x.log')
    expect(unexpandedMacros(resolved)).toEqual(['${INSTANCE_DIR}'])
    // 展开值是绝对路径时，字面结果就是"前面多一个 /"——后端 Resolve 也是纯字符串替换，
    // 这里保持一致（多为一个斜杠无害，语义分叉才是问题）。
    expect(resolvePathMacros('${INSTANCE_DIR}/${LOG_DIR}/a.log', { templateMacros }))
      .toBe('${INSTANCE_DIR}//opt/default-tomcat/logs/a.log')
  })

  it('没有模板/没有覆盖时原样返回，不猜值', () => {
    expect(resolvePathMacros('/var/log/a.log')).toBe('/var/log/a.log')
    expect(resolvePathMacros('', { templateMacros })).toBe('')
    expect(resolvePathMacros(undefined, { templateMacros })).toBe('')
  })

  it('同名宏出现多次全部替换', () => {
    expect(resolvePathMacros('${LOG_DIR}/a.log;${LOG_DIR}/b.log', {
      templateMacros,
      serviceMacros: { LOG_DIR: '/x' },
    })).toBe('/x/a.log;/x/b.log')
  })
})

describe('hasGlobMeta', () => {
  it('识别单层通配（现场模式：/var/log/*.log、/var/log/*/*/*.log）', () => {
    expect(hasGlobMeta('/var/log/*.log')).toBe(true)
    expect(hasGlobMeta('/var/log/*/*/*.log')).toBe(true)
    expect(hasGlobMeta('/home/esb/data/logs/*/log_error.log')).toBe(true)
    expect(hasGlobMeta('/var/log/app?.log')).toBe(true)
    expect(hasGlobMeta('/var/log/app[12].log')).toBe(true)
  })

  it('普通绝对路径不算通配', () => {
    expect(hasGlobMeta('/var/log/app.log')).toBe(false)
    expect(hasGlobMeta('/home/esb/data/logs/app1/log_error.log')).toBe(false)
    expect(hasGlobMeta('')).toBe(false)
    expect(hasGlobMeta(undefined)).toBe(false)
  })
})
