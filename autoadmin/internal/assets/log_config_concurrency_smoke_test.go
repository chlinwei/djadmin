package assets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

// 使用专用测试库验证写偏斜；只创建并清理本测试命名的表，不操作业务数据。
func TestLogConfigSerializablePreventsWriteSkewRealDatabase(t *testing.T) {
	dsn := os.Getenv("LOG_CONFIG_CONCURRENCY_DSN")
	if dsn == "" {
		t.Skip("LOG_CONFIG_CONCURRENCY_DSN 未设置：跳过真库并发测试（需要专用测试库的建表/删表权限）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	table := fmt.Sprintf("log_config_concurrency_%d", time.Now().UnixNano())
	if _, err := pool.ExecContext(ctx, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, enabled BIGINT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.ExecContext(cleanupCtx, "DROP TABLE "+table); err != nil {
			t.Errorf("cleanup %s: %v", table, err)
		}
	}()
	if _, err := pool.ExecContext(ctx, "INSERT INTO "+table+" (id, enabled) VALUES (1, 0), (2, 0)"); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	outcomes := make(chan error, 2)
	conflict := errors.New("same host log path already enabled")
	for id := 1; id <= 2; id++ {
		go func(id int) {
			tx, err := beginLogConfigWrite(ctx, pool)
			if err != nil {
				outcomes <- err
				return
			}
			defer tx.Rollback()
			// 两条服务记录分别修改，强制同时进入检查：普通快照隔离下双方都会认为仅自己启用。
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET enabled=1 WHERE id=%d", table, id)); err != nil {
				outcomes <- err
				return
			}
			ready <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				outcomes <- ctx.Err()
				return
			}
			var enabled int
			if err := tx.QueryRowContext(ctx, "SELECT SUM(enabled) FROM "+table).Scan(&enabled); err != nil {
				outcomes <- err
				return
			}
			if enabled > 1 {
				outcomes <- conflict
				return
			}
			outcomes <- tx.Commit()
		}(id)
	}
	for range 2 {
		select {
		case <-ready:
		case err := <-outcomes:
			cancel()
			// 另一个事务可能仍在等待放行；取消后必须收尾，才能安全清理测试表。
			<-outcomes
			t.Fatalf("concurrent writers did not reach validation: %v", err)
		case <-ctx.Done():
			<-outcomes
			<-outcomes
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	committed := 0
	for range 2 {
		err := <-outcomes
		if err == nil {
			committed++
			continue
		}
		if errors.Is(err, conflict) {
			continue
		}
		if translated := translate(err); translated.Error() != ErrConcurrentWrite.Error() {
			t.Errorf("want a database concurrency rejection, got %v", err)
		}
	}
	if committed != 1 {
		t.Fatalf("exactly one conflicting writer must commit, got %d", committed)
	}
	var enabled int
	if err := pool.QueryRowContext(ctx, "SELECT SUM(enabled) FROM "+table).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatalf("database contains %d enabled conflicting inputs", enabled)
	}
}
