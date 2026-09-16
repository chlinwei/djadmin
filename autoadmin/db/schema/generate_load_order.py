#!/usr/bin/env python3
"""把 db/schema/postgres/*.sql 拼成一个「可直接灌进空库」的装载脚本。

为什么需要它：schema 文件是按 Django 域切分的 6 个文件（001…006），而 PostgreSQL 建表时要求
被引用的表已存在 —— 跨文件外键与自引用外键让「按文件名顺序执行」必然失败。sqlc 只解析不执行，
所以这个问题在生成期完全看不出来（P5 陷阱 14）。

用法：

    python3 db/schema/generate_load_order.py > /tmp/pgschema_ordered.sql
    psql "postgres://user@host:5432/scratch?sslmode=disable" -f /tmp/pgschema_ordered.sql

输出 = CREATE TABLE（按外键依赖拓扑排序）+ 其余语句（ALTER/COMMENT 等）+ CREATE INDEX（最后，
否则索引会先于它依赖的列不存在）。两类都用「行尾分号」切分语句，与本仓库 schema 的书写风格一致。

用途：P1-7 的迁移复核（从折叠 schema 起、倒序回放 down、正序回放 up，累积效果必须逐条等于
折叠态）与 P1-9 的 EXPLAIN 试验都先需要这个装载脚本。见 docs/architecture/SQL_DESIGN.md §4.7/§2.6。
"""

import re
import sys
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent / "postgres"


def statements(sql: str) -> list[str]:
    chunks: list[str] = []
    buffer = ""
    for line in sql.split("\n"):
        buffer += line + "\n"
        if line.rstrip().endswith(";"):
            if buffer.strip():
                chunks.append(buffer.strip())
            buffer = ""
    if buffer.strip():
        chunks.append(buffer.strip())
    return chunks


def main() -> int:
    sql = "\n".join(path.read_text() for path in sorted(SCHEMA_DIR.glob("*.sql")))
    tables: dict[str, str] = {}
    others: list[str] = []
    for statement in statements(sql):
        match = re.match(r"CREATE TABLE (\w+)", statement)
        if match:
            tables[match.group(1)] = statement
        else:
            others.append(statement)

    references = {
        name: set(re.findall(r"REFERENCES (\w+)", body)) for name, body in tables.items()
    }

    order: list[str] = []
    seen: set[str] = set()

    def visit(name: str) -> None:
        if name in seen:
            return
        seen.add(name)
        # 被引用的表先建；自引用（如 assets_hostgroup.parent_id）因为已经在 seen 里而不会递归。
        for dependency in sorted(references.get(name, ())):
            if dependency in tables:
                visit(dependency)
        order.append(name)

    for name in sorted(tables):
        visit(name)

    indexes = [s for s in others if s.startswith("CREATE INDEX")]
    rest = [s for s in others if not s.startswith("CREATE INDEX")]

    out = [
        "-- 由 db/schema/generate_load_order.py 生成，勿手改。",
        "-- CREATE TABLE 按外键依赖拓扑排序，其余语句随后，CREATE INDEX 最后。",
        "SET client_min_messages = warning;",
    ]
    out += [tables[name] for name in order]
    out += rest
    out += indexes
    print("\n".join(out))
    print(
        "-- tables=%d others=%d indexes=%d" % (len(order), len(rest), len(indexes)),
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
