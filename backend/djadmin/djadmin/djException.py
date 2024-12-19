from rest_framework.views import exception_handler,Response
from rest_framework import status
from djadmin import settings


def djadmin_handler(err,context: dict):
    #获取rest 标准的错误响应对象
    response: Response = exception_handler(err,context)
    if response is None:
        #Debug模式下，不处理系统异常
        if settings.DEBUG:
            raise err
        res = {
            'msg': 'server internal error',
            'code':500,
            'data':err
        }
        return Response(res,status=status.HTTP_500_INTERNAL_SERVER_ERROR,exception=True)
    else:
        msg = response.reason_phrase
        if "detail" in response.data:
            data = response.data["detail"]
        else:
            data = []
            for k,v in response.data.items():
                if isinstance(v,list):
                    data.append(k+v[0])
        res = {}
        res.update(response.data)
        res['msg'] = msg
        res['code'] = 200
        res['data'] = data
        return Response(res,status=response.status_code,exception=True)