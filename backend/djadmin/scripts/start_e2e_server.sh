#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${PROJECT_DIR}"

python - <<'PY'
import os

db_name = os.getenv('DJADMIN_E2E_DB_NAME', 'djadmin_e2e')
db_user = os.getenv('DJADMIN_E2E_DB_USER', 'root')
db_password = os.getenv('DJADMIN_E2E_DB_PASSWORD', '1qazXSW@')
db_host = os.getenv('DJADMIN_E2E_DB_HOST', '10.25.66.150')
db_port = int(os.getenv('DJADMIN_E2E_DB_PORT', '3400'))

connection = None
last_error = None

try:
	import MySQLdb  # type: ignore

	connection = MySQLdb.connect(
		host=db_host,
		user=db_user,
		passwd=db_password,
		port=db_port,
		charset='utf8mb4',
	)
except Exception as exc:  # pragma: no cover
	last_error = exc
	try:
		import pymysql  # type: ignore

		connection = pymysql.connect(
			host=db_host,
			user=db_user,
			password=db_password,
			port=db_port,
			charset='utf8mb4',
			autocommit=True,
		)
	except Exception as inner_exc:  # pragma: no cover
		raise SystemExit(f"[ERROR] Unable to connect MySQL for e2e DB creation: {inner_exc} (first error: {last_error})")

with connection:
	with connection.cursor() as cursor:
		cursor.execute(
			f"CREATE DATABASE IF NOT EXISTS `{db_name}` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
		)

print(f"[OK] ensured e2e database exists: {db_name}")
PY

python manage.py migrate --settings=djadmin.e2e_settings --noinput
python scripts/seed_e2e_data.py

exec python manage.py runserver 127.0.0.1:19000 --settings=djadmin.e2e_settings
