def jwt_payload_handler(user):
    from rest_framework_jwt.utils import jwt_payload_handler as default_handler
    payload = default_handler(user)
    payload['perms'] = getattr(user, 'perms', [])  # 显式注入权限:ml-citation{ref="3,7" data="citationList"}
    return payload
