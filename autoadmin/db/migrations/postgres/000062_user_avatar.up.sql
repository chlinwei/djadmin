CREATE TABLE sys_user_avatar (
  user_id integer NOT NULL PRIMARY KEY,
  filename varchar(255) NOT NULL,
  content bytea NOT NULL,
  update_time timestamp(6) NOT NULL,
  CONSTRAINT sys_user_avatar_filename UNIQUE (filename),
  CONSTRAINT sys_user_avatar_user_fk FOREIGN KEY (user_id) REFERENCES sys_user (id) ON DELETE CASCADE,
  CONSTRAINT sys_user_avatar_size CHECK (OCTET_LENGTH(content) BETWEEN 1 AND 1048576)
);
