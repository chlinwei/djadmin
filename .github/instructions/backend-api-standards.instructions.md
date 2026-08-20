---
description: "Auto-apply API response format and Django best practices to all backend API views. Enforces consistent response structure, proper status codes, pagination format, type hints, and API contract compliance."
name: "Backend API View Standards"
applyTo: "backend/djadmin/*/views.py"
---

# Backend API View Standards

When reviewing or editing API views in `backend/djadmin/*/views.py`, apply these standards automatically.

## API Response Format (Standard)

**All successful API responses must return HTTP 200** and follow this exact structure:

```python
{
  "code": 200,
  "msg": "success",
  "data": {
    ...response data...
  }
}
```

### Response Codes
- `200` — Success (all successful operations, including create/update/delete)
- `300` — Account/password error
- `301` — Token validation failed / login expired
- Other codes — Various errors (describe in `msg` field)

**Never use**:
- ❌ HTTP 201 (Created) — use code 200 instead
- ❌ HTTP 204 (No Content) — use code 200 with appropriate data
- ❌ HTTP 202 (Accepted) — use code 200 unless documented exception exists

### Example Responses

**Create endpoint**:
```python
# ✅ CORRECT
{
  "code": 200,
  "msg": "success",
  "data": {
    "id": 1,
    "username": "admin",
    ...
  }
}

# ❌ WRONG
{
  "code": 201,  # Wrong code
  "message": "User created",  # Wrong field name
  "user": { ... }  # Wrong data wrapper
}
```

**Delete endpoint**:
```python
# ✅ CORRECT
{
  "code": 200,
  "msg": "success",
  "data": null  # or { "deleted_count": 1 }
}
```

## Pagination Response Format

All list endpoints returning paginated results must use this structure:

```python
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [...items...],
    "count": 10,           # Total items
    "pageNumber": 1,       # Current page (1-indexed)
    "pageSize": 10,        # Items per page
    "totalPages": 1,       # Total pages
    "next": null,          # URL to next page or null
    "previous": null       # URL to previous page or null
  }
}
```

**Never**:
- ❌ Use `total` instead of `count`
- ❌ Use `page` instead of `pageNumber`
- ❌ Wrap results in extra `pagination` object
- ❌ Return different field names across endpoints

## Type Hints and Pylance Compliance

All view methods must include proper type hints:

```python
from typing import Optional, Dict, Any, List
from rest_framework.response import Response
from rest_framework.request import Request

class UserViewSet(ViewSet):
    def list(self, request: Request) -> Response:
        """List users with pagination."""
        ...
    
    def retrieve(self, request: Request, pk: int) -> Response:
        """Get a single user by ID."""
        ...
    
    def create(self, request: Request) -> Response:
        """Create a new user."""
        ...
    
    def update(self, request: Request, pk: int) -> Response:
        """Update a user."""
        ...
```

**Pylance Type Checking**:
1. When editing backend code, run `get_errors` to check Pylance diagnostics
2. Fix real type errors: missing type hints, incompatible types, undefined attributes
3. For library false positives, add `# type: ignore[error-code]` with a comment explaining why
4. Do NOT ignore type errors without understanding them

## Migration Impact Analysis

Before merging model changes:

1. **Identify the change**: New field, removed field, renamed field, type change, unique constraint
2. **Impact on existing data**: Will this cause data loss? Will queries break?
3. **Impact on API contract**: Will existing client code break? Will pagination queries fail?
4. **Migration strategy**: Is a data migration needed? Can the change be backwards-compatible?

Example issues to flag:
- ❌ Removing a field used by API without deprecation warning
- ❌ Adding `null=False` to existing nullable field without default value
- ❌ Changing field type (e.g., CharField → IntegerField) without data migration
- ✅ Adding new optional field with `blank=True, null=True`
- ✅ Adding new `Meta.ordering` that doesn't break existing queries

## API Format Compliance Testing

Tests must verify API response format using `BaseTestCase.assertResponseOK`:

```python
from common.test_utils import BaseTestCase

class UserAPITest(BaseTestCase):
    def test_create_user(self):
        """Test user creation returns proper format."""
        response = self.client.post('/api/user/', {'username': 'newuser'})
        
        # This checks: code==200, msg=='success', data exists, structure is correct
        self.assertResponseOK(response)
        self.assertEqual(response.data['data']['username'], 'newuser')
    
    def test_list_users(self):
        """Test user list returns pagination format."""
        response = self.client.get('/api/user/?page=1&page_size=10')
        self.assertResponseOK(response)
        
        # Verify pagination structure
        data = response.data['data']
        self.assertIn('results', data)
        self.assertIn('count', data)
        self.assertIn('pageNumber', data)
```

**When to use `assertSuccess`**: Only when the API response must also have `msg == 'success'` (not just `code == 200`).

## Backwards Compatibility

**Default: Do not break existing API contracts** unless user explicitly requests.

When reviewing API changes:
- ❌ Removing an existing endpoint without deprecation notice
- ❌ Changing response field names or structure
- ❌ Adding new `required` fields to existing responses
- ✅ Adding new optional fields to existing responses
- ✅ Adding new optional request parameters
- ✅ Deprecating old response fields while maintaining backwards compatibility

## Celery Task Best Practices

For background tasks in `tasks.py`:

```python
from celery import shared_task
from celery.utils.log import get_task_logger

logger = get_task_logger(__name__)

@shared_task(bind=True, max_retries=3, default_retry_delay=60)
def process_user_data(self, user_id: int) -> Dict[str, Any]:
    """Process user data asynchronously.
    
    Args:
        user_id: User ID to process
    
    Returns:
        Result dictionary
    
    Raises:
        Retries on transient errors
    """
    try:
        user = User.objects.get(id=user_id)
        # Task logic here
        return {'status': 'success', 'user_id': user_id}
    except User.DoesNotExist as exc:
        logger.error(f"User {user_id} not found")
        return {'status': 'error', 'user_id': user_id}
    except Exception as exc:
        # Retry on transient errors
        raise self.retry(exc=exc, countdown=60)
```

## Review Checklist

When editing an API view, verify:
- [ ] Success responses return HTTP 200 (not 201, 202, etc.)
- [ ] Response structure is `{code: 200, msg: 'success', data: {...}}`
- [ ] Error responses have appropriate code (300, 301, etc.) and descriptive `msg`
- [ ] List endpoints return pagination format with all required fields
- [ ] All view methods have type hints
- [ ] Pylance reports no type errors (or errors are documented with `# type: ignore`)
- [ ] If models changed, migration impact is documented
- [ ] Tests use `assertResponseOK` to verify response format
- [ ] Existing API contracts are not broken (unless explicitly requested)
- [ ] Celery tasks have proper retry logic and error handling
