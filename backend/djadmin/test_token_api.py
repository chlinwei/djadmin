import urllib.request
import urllib.parse
import json

# 登录获取 token
login_url = 'http://localhost:8000/system/login/'
login_data = urllib.parse.urlencode({'username': 'admin', 'password': '123456'}).encode('utf-8')

try:
    response = urllib.request.urlopen(login_url, login_data)
    login_result = json.loads(response.read().decode('utf-8'))
    
    print('=== Login Response ===')
    print(json.dumps({k: v for k, v in login_result.items() if k != 'data'}, indent=2, ensure_ascii=False))
    
    if login_result.get('code') == 200:
        token = login_result.get('data', {}).get('token')
        print(f'\nToken obtained: {token[:30]}...\n')
        
        # 用 token 测试调度器 API
        print('=== Testing Scheduler API ===')
        api_url = 'http://localhost:8000/sys/scheduler/tasks/?page=1&page_size=10'
        req = urllib.request.Request(api_url, headers={'Authorization': token})
        api_response = urllib.request.urlopen(req)
        api_data = json.loads(api_response.read().decode('utf-8'))
        
        print(f'Response code: {api_data.get("code")}')
        print(f'Response msg: {api_data.get("msg")}')
        
        if api_data.get('data'):
            data = api_data.get('data', {})
            print(f'Results count: {len(data.get("results", []))}')
            print(f'Total count: {data.get("count")}')
            print(f'Page number: {data.get("pageNumber")}')
            print(f'Total pages: {data.get("totalPages")}')
            
            if data.get('results'):
                print(f'\nFirst task: {data["results"][0].get("name")}')
except Exception as e:
    print(f'Error: {e}')
