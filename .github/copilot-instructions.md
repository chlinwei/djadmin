# Copilot Instructions for djadmin

## Project Context & Rules

- **Primary Context**: Use [PROJECT_CONTEXT.md](../PROJECT_CONTEXT.md) as the primary context source for this repository.
- **API Standards**: Follow [API_RULES.md](API_RULES.md) for all API response formats, status codes, and implementation requirements.
- **Compliance Status**: See [API_COMPLIANCE_REPORT.md](API_COMPLIANCE_REPORT.md) for current API return format compliance check (55% compliant).
- **User Request First**: If the user request conflicts with these files, follow the user request first.

## Tech Stack

- Backend: Django + DRF (Python)
- Database: MySQL
- Frontend: Vue 3 + Vite
- Frontend UI: Ant Design Vue (primary)
- Scheduler: APScheduler (separate process required)

## Repository Layout

- Backend root: backend/djadmin
- Frontend root: fronted
- Main backend apps: user, role, menu, assets, scheduler, sys_config

## Working Rules

- **Pylance 类型优先**: 修改后端代码时，首先使用 `get_errors` 检查并解决 Pylance 报告的类型问题，再提交最终方案。区分真正的类型错误（需修复代码）和库的误报（加 `# type: ignore`）。
- Do not break existing API contracts unless user explicitly asks for API changes.
- Prefer minimal, targeted edits and keep current code style.
- For frontend UI work, use Ant Design Vue components and patterns by default.
- Do not introduce new Element Plus-based UI changes unless user explicitly requests it.
- Keep button icons globally consistent across all pages: use the same icon mapping/style for the same action (add, edit, delete, refresh, save, details) and avoid mixing multiple icon styles for identical actions.
- Time handling rule: backend timestamps should be stored/returned in UTC, and frontend must convert and display date/time using the current user's timezone.
- For backend changes, include migration impact notes when models are changed.
- For scheduler-related issues, always verify whether APScheduler process is running.
- For bug fixes, provide quick verification steps (commands + expected result).

## Common Run Commands

### Backend

**First, activate virtual environment (Windows PowerShell):**

```powershell
(Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned) ; (& c:\workspace\python\djadmin\backend\.venv\Scripts\Activate.ps1)
cd backend/djadmin
python manage.py runserver 0.0.0.0:8000
```

**Or use these commands:**

- `cd backend/djadmin`
- Activate venv: `(Set-ExecutionPolicy -Scope Process -ExecutionPolicy RemoteSigned) ; (& .\.venv\Scripts\Activate.ps1)`
- `pip install -r requirements.txt` (if needed)
- `python manage.py runserver`

### Frontend

- cd fronted
- npm install
- npm run dev
- npm run build

### Scheduler

- Windows: ./start_scheduler.ps1
- Or: cd backend/djadmin && python manage.py runapscheduler

## Default Credentials

- **Username:** admin
- **Password:** admin

Login at: http://localhost:8000/sys/login or http://localhost:5173/login

## API Response Format

**All successful API responses return HTTP status 200** (except for authentication/network errors).

Response structure:
```json
{
  "code": 200,
  "msg": "success",
  "data": {
    ...response data...
  }
}
```

Response codes:
- `200` - Success (always use this for all successful operations)
- `300` - Account/password error
- `301` - Token validation failed / login expired
- Other codes - Various errors (msg field describes the error)

Pagination response format (for list endpoints):
```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "results": [...items...],
    "count": 10,
    "pageNumber": 1,
    "pageSize": 10,
    "totalPages": 1,
    "next": null,
    "previous": null
  }
}
```

## Response Preference

- User prefers not to repeat project context in every conversation.
- When possible, infer missing context from repository and this instruction file first.

