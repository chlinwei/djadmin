CREATE TABLE `sys_user_avatar` (
  `user_id` int NOT NULL,
  `filename` varchar(255) NOT NULL,
  `content` mediumblob NOT NULL,
  `update_time` datetime(6) NOT NULL,
  PRIMARY KEY (`user_id`),
  CONSTRAINT `sys_user_avatar_filename` UNIQUE (`filename`),
  CONSTRAINT `sys_user_avatar_user_fk` FOREIGN KEY (`user_id`) REFERENCES `sys_user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `sys_user_avatar_size` CHECK (OCTET_LENGTH(`content`) BETWEEN 1 AND 1048576)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
