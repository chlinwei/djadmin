from rest_framework.renderers import JSONRenderer




class DjAdminResponse_render(JSONRenderer):
    def render(self, data, accepted_media_type=None, renderer_context=None):
        if renderer_context:
            if isinstance(data,dict):
                # msg = data.pop('msg','success')
                # code = data.pop('code',2)
                msg = 'error'
                code = 300
            else:
                msg = 'success'
                code = 200
            ret = {
                'msg': msg,
                'code': code,
                'data': data,
            }
            return super().render(ret, accepted_media_type, renderer_context)
        else:
            return super().render(data, accepted_media_type, renderer_context)

    

