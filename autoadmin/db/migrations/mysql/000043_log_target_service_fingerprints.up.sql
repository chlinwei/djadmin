-- 服务级"已下发配置指纹"（见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §8.3）。
--
-- 背景：`config_fingerprint` 是**主机级**的，含该主机上**所有**服务的片段。所以共享主机上改服务 B
-- 会让服务 A 的"期望指纹"也跟着变、显示成"待下发"——可 A 自己根本没改（现场：服务 A 被 B 的改动
-- 带成待下发）。哈希不可分解，要按服务判状态就必须另外记住"每个服务上次下发时的子指纹"。
--
-- 存法：主机行本来就有，加一个 JSON 列 `{service_code: subfp}` 即可（不新建表、不加联表）。
-- 子指纹口径 = sha256("output:" + 输出段标识 + 该服务在该主机的全部 inputs.d 片段内容)，
-- **不含** `/var/lib/filebeat/.keep`（它只看整机有没有片段，是主机级信号）。
-- 一次性影响：存量主机该列为空 `{}` → 服务视图先显示"从未下发/待下发"，下发一次后恢复稳定。
ALTER TABLE `monitor_log_collection_target`
  ADD COLUMN `service_fingerprints` json NOT NULL DEFAULT (JSON_OBJECT());
