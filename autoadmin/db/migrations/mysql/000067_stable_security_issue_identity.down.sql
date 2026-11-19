-- 合并前的多个问题头无法从统一台账无损还原，禁止伪造可回退结果。
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = '000067 merges issue identities irreversibly; restore a pre-migration backup to downgrade';
