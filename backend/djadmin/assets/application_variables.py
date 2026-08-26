import re

APP_HOME_VARIABLE = '${APP_HOME}'
RUN_USER_VARIABLE = '${RUN_USER}'
MACRO_PATTERN = re.compile(r'\$\{([A-Za-z_][A-Za-z0-9_]*)\}')


class ApplicationVariableError(ValueError):
    pass


def resolve_application_variables(value, *, app_home, run_user):
    """解析应用内置变量，同时保留 JAVA_HOME 等其他 shell 环境变量。"""
    text = str(value or '')
    normalized_run_user = str(run_user or '').strip()
    raw_app_home = str(app_home or '').strip()

    if RUN_USER_VARIABLE in text and normalized_run_user == '':
        raise ApplicationVariableError('引用 ${RUN_USER} 前必须填写运行用户')
    if APP_HOME_VARIABLE in raw_app_home:
        raise ApplicationVariableError('App Home 不能引用自身')

    resolved_app_home = raw_app_home.replace(RUN_USER_VARIABLE, normalized_run_user)
    if resolved_app_home != '/':
        resolved_app_home = resolved_app_home.rstrip('/')
    if APP_HOME_VARIABLE in text and resolved_app_home == '':
        raise ApplicationVariableError('引用 ${APP_HOME} 前必须填写 App Home')

    return text.replace(APP_HOME_VARIABLE, resolved_app_home).replace(RUN_USER_VARIABLE, normalized_run_user)


def resolve_macro_variables(value, *, definitions, values):
    """替换模板声明且由逻辑服务提供的宏。"""
    text = str(value or '')
    names = {item['name'] for item in definitions if isinstance(item, dict) and item.get('name')}

    def replace(match):
        name = match.group(1)
        if name not in names:
            return match.group(0)
        resolved = str(values.get(name, '')).strip()
        if not resolved:
            raise ApplicationVariableError(f'宏 {name} 未填写')
        return resolved

    return MACRO_PATTERN.sub(replace, text)