-- dj-agent 安装包管理：上传的二进制及其 sha256/大小/版本，is_active 独占激活，
-- Agent 安装/更新优先使用激活包，未激活时回退构建产物（internal/assets/agent_update.go）。

CREATE TABLE `agent_package` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `version` VARCHAR(64) NOT NULL,
  `file` VARCHAR(512) NOT NULL,
  `sha256` CHAR(64) NOT NULL,
  `size_bytes` BIGINT NOT NULL,
  `is_active` TINYINT(1) NOT NULL DEFAULT 0,
  `create_time` DATETIME(6) NULL,
  PRIMARY KEY (`id`)
);
