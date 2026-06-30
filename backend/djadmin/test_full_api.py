import urllib.request
import urllib.parse
import json

# 登录获取 token
print("=== Login Test ===")
login_url = 'http://localhost:8000/sys/login'
login_data = urllib.parse.urlencode({'username': 'admin', 'password': 'admin'}).encode('utf-8')

try:
    response = urllib.request.urlopen(login_url, login_data)
    login_result = json.loads(response.read().decode('utf-8'))
    
    print(f"Status: OK")
    print(f"Code: {login_result.get('code')}")
    print(f"Msg: {login_result.get('msg')}")
    
    if login_result.get('code') == 200:
        token = login_result.get('data', {}).get('token')
        print(f"Token: {token[:50]}...\n")
        
        # 用 token 测试调度器 API
        print("=== Testing Scheduler API ===")
        api_url = 'http://localhost:8000/sys/scheduler/tasks/?page=1&page_size=10'
        req = urllib.request.Request(api_url, headers={'Authorization': token})
        
        try:
            api_response = urllib.request.urlopen(req)
            response_content = api_response.read().decode('utf-8')
            print(f"Raw response: {response_content[:200]}...\n")
            
            api_data = json.loads(response_content)
            
            print(f"Code: {api_data.get('code')}")
            print(f"Msg: {api_data.get('msg')}")
            
            if api_data.get('data'):
                data = api_data.get('data', {})
                print(f"\nPagination Info:")
                print(f"  Results count: {len(data.get('results', []))}")
                print(f"  Total count: {data.get('count')}")
                print(f"  Page number: {data.get('pageNumber')}")
                print(f"  Total pages: {data.get('totalPages')}")
                
                if data.get('results'):
                    print(f"\nFirst task:")
                    first = data['results'][0]
                    print(f"  Name: {first.get('name')}")
                    print(f"  Code: {first.get('code')}")
                    print(f"  Interval: {first.get('interval_minutes')} minutes")
        except urllib.error.HTTPError as e:
            error_content = e.read().decode('utf-8')
            print(f"HTTP Error {e.code}:")
            print(error_content[:300])
    else:
        print(f"Login failed: {login_result.get('msg')}")
        
except Exception as e:
    print(f"Error: {e}")
