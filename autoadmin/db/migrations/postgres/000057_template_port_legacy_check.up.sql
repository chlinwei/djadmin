-- PG 侧差异：按表和表达式从 pg_constraint 查出旧非负整数检查，不依赖生成的约束名。
DO $$
DECLARE legacy_check record;
BEGIN
  FOR legacy_check IN
    SELECT con.conname
    FROM pg_constraint con
    WHERE con.conrelid = 'assets_application_port'::regclass
      AND con.contype = 'c'
      AND regexp_replace(pg_get_expr(con.conbin, con.conrelid), '[[:space:]()"]', '', 'g')
        IN ('port>=0', '0<=port')
  LOOP
    EXECUTE format('ALTER TABLE assets_application_port DROP CONSTRAINT %I', legacy_check.conname);
  END LOOP;
END $$;
