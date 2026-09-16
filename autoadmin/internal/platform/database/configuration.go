package database

import "time"

// Configuration 是数据访问层的连接参数。两个方言的 DSN 都放在这里，
// 由 build tag 决定用哪一个（见 open_mysql.go / open_postgres.go）：
// 默认（不带 tag）连 MySQL，`-tags postgres` 连 PostgreSQL。
type Configuration struct {
	MySQLDSN        string
	PostgresDSN     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}
