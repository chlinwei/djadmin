from rest_framework.views import exception_handler,Response
from rest_framework import status
from djadmin import settings
from djadmin.errordict import DjadminException
from djadmin.utils import Response_error,Response_djerror
from djadmin.errordict import CommonError,ErrorMixin

def djadmin_handler(err,context):
    #获取rest 标准的错误响应对象
    print("===========异常====================")
    response = exception_handler(err,context)
    if response is None:
        #自定义异常,会在这里
        if isinstance(err,DjadminException):
            return Response_djerror(err)
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
        # res.update(response.data)
        res['msg'] = msg
        res['code'] = 600
        res['data'] = data
        return Response(res,status=response.status_code,exception=True)