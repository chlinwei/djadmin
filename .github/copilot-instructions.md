# Copilot Instructions for djadmin

## Project Context Source

Use [PROJECT_CONTEXT.md](../PROJECT_CONTEXT.md) as the primary context source for this repository.
If the user request conflicts with this file, follow the user request first.

## Tech Stack

- Backend: Django + DRF (Python)
- Frontend: Vue 3 + Vite
- Frontend UI: Ant Design Vue (primary)
- Scheduler: APScheduler (separate process required)

## Repository Layout

- Backend root: backend/djadmin
- Frontend root: fronted
- Main backend apps: user, role, menu, assets, scheduler, sys_config

## Working Rules

- Do not break existing API contracts unless user explicitly asks for API changes.
- Prefer minimal, targeted edits and keep current code style.
- For frontend UI work, use Ant Design Vue components and patterns by default.
- Do not introduce new Element Plus-based UI changes unless user explicitly requests it.
- For backend changes, include migration impact notes when models are changed.
- For scheduler-related issues, always verify whether APScheduler process is running.
- For bug fixes, provide quick verification steps (commands + expected result).

## Common Run Commands

### Backend

- cd backend/djadmin
- pip install -r requirements.txt
- python manage.py runserver

### Frontend

- cd fronted
- npm install
- npm run dev
- npm run build

### Scheduler

- Windows: ./start_scheduler.ps1
- Or: cd backend/djadmin && python manage.py runapscheduler

## Response Preference

- User prefers not to repeat project context in every conversation.
- When possible, infer missing context from repository and this instruction file first.
