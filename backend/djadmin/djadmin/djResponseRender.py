from rest_framework.renderers import JSONRenderer

class DjAdminResponse_render(JSONRenderer):
    def render(self, data, accepted_media_type=None, renderer_context=None):

        print("========render===============")
        print(f"Data type: {type(data)}, Keys: {list(data.keys()) if isinstance(data, dict) else 'N/A'}")
        
        if isinstance(data, dict):
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
                # 检查是否是 DRF 验证错误格式（所有值都是列表）
                is_validation_error = all(isinstance(v, list) for v in data.values()) if data else False
                if is_validation_error:
                    # 这是 DRF 验证错误，不进行重新封装
                    print("DRF 验证错误格式，直接返回")
                    return super().render(data, accepted_media_type, renderer_context)
                else:
                    # 其他字典格式，直接返回
                    print("其他字典格式，直接返回")
                    return super().render(data, accepted_media_type, renderer_context)
        else:
            # 非字典格式（如列表等），直接返回
            print("非字典格式，直接返回")
            return super().render(data, accepted_media_type, renderer_context)


    

