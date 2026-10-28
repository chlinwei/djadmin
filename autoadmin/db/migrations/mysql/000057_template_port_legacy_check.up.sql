-- Remove only the obsolete nonnegative-integer check. Snapshot-created databases
-- do not have it; discover the generated name rather than assuming a suffix.
SET @port_chk := (
  SELECT cc.CONSTRAINT_NAME
  FROM information_schema.CHECK_CONSTRAINTS cc
  JOIN information_schema.TABLE_CONSTRAINTS tc
    ON tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
   AND tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
  WHERE cc.CONSTRAINT_SCHEMA = DATABASE()
    AND tc.TABLE_NAME = 'assets_application_port'
    AND tc.CONSTRAINT_TYPE = 'CHECK'
    AND REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(cc.CHECK_CLAUSE,
      '`', ''), ' ', ''), '(', ''), ')', ''), CHAR(9), '') IN ('port>=0', '0<=port')
  LIMIT 1
);
SET @drop_port_chk := IF(
  @port_chk IS NULL,
  'SELECT 1',
  CONCAT('ALTER TABLE `assets_application_port` DROP CHECK `', REPLACE(@port_chk, '`', '``'), '`')
);
PREPARE drop_port_chk_stmt FROM @drop_port_chk;
EXECUTE drop_port_chk_stmt;
DEALLOCATE PREPARE drop_port_chk_stmt;
