from rest_framework.renderers import JSONRenderer

class DjAdminResponse_render(JSONRenderer):
    def render(self, data, accepted_media_type=None, renderer_context=None):

        print("========render===============")
        if isinstance(data,dict):
            if 'msg' in data and 'code' in data and 'data' in data:
                # 证明格式是正确的
                if data['code'] != 200:
                #  证明这个data来自于自定义异常
                    print("正常格式且来自异常")
                    return super().render(data, accepted_media_type, renderer_context)
                else:
                    print("正常格式")
                    return super().render(data, accepted_media_type, renderer_context)
            else:
                print("不正常格式来自系统的json返回")
                ret = {
                }
                #这个不是正常格式的json
                ret['msg'] = 'sucess'
                ret['code'] = 200
                ret['data'] = data
                return super().render(ret, accepted_media_type, renderer_context)
        else:
            print("不正常格式来自系统的json返回")
            ret = {
            }
            #这个不是正常格式的json
            ret['msg'] = 'sucess'
            ret['code'] = 200
            ret['data'] = data
            return super().render(ret, accepted_media_type, renderer_context)


    

