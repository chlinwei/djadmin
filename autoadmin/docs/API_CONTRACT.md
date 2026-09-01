# API contract

The existing frontend contract remains unchanged during migration.

## Success

Successful business operations use HTTP 200:

```json
{"code": 200, "msg": "success", "data": {}}
```

## Business errors

Expected business errors retain HTTP 200 and use application codes:

- `300`: account or password error
- `301`: invalid or expired token
- `400`: invalid request
- `403`: permission denied
- `404`: resource not found

Transport, malformed HTTP and unavailable dependency errors may use an appropriate HTTP status, but still use the same envelope.

## Pagination

List endpoints accept `page` and `page_size`. The compatibility default is 10 and the maximum is 30.

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [],
    "count": 0,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 0,
    "next": null,
    "previous": null
  }
}
```

## Compatibility rules

- Preserve existing route prefixes and JSON field names until a coordinated frontend change is approved.
- Preserve JWT permission strings in `module:resource:action` form.
- Keep `/monitor/prometheus/http-sd/` and `/monitor/alert-webhook/api/v2/alerts` as explicitly authenticated machine endpoints according to their current behavior.
- Contract tests must compare Django and Go responses for each migrated endpoint before traffic switches.