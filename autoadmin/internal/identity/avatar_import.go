package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// ImportDiskAvatars is an explicit, rerunnable migration, not a read-time disk fallback.
// Source files are retained for backup; each successful user is committed independently.
func ImportDiskAvatars(ctx context.Context, pool *sql.DB, mediaRoot string) error {
	repository := NewRepository(pool)
	users, err := repository.queries.ListDiskUserAvatars(ctx)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		slog.Info("avatar import complete", "imported", 0)
		return nil
	}
	root, err := os.OpenRoot(filepath.Join(mediaRoot, avatarDirectory))
	if err != nil {
		return fmt.Errorf("打开旧头像目录：%w", err)
	}
	defer root.Close()
	var failures []error
	imported := 0
	for _, user := range users {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		if err := importDiskAvatar(ctx, repository, root, user.ID, user.Avatar.String); err != nil {
			failures = append(failures, fmt.Errorf("导入用户 %d 头像：%w", user.ID, err))
			continue
		}
		imported++
	}
	slog.Info("avatar import complete", "imported", imported, "failed", len(failures))
	return errors.Join(failures...)
}

func importDiskAvatar(ctx context.Context, repository *Repository, root *os.Root, userID int32, previous string) error {
	name := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(previous), "/"), "media/")
	name = strings.TrimPrefix(name, avatarDirectory+"/")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("旧头像路径不合法：%q", previous)
	}
	// os.Root confines symlinks as well as relative paths to the configured avatar directory.
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("旧头像不是普通文件")
	}
	source, err := io.ReadAll(io.LimitReader(file, avatarUploadLimit+1))
	if err != nil {
		return err
	}
	content, err := processAvatar(source)
	if err != nil {
		return err
	}
	filename, err := newAvatarFilename()
	if err != nil {
		return err
	}
	saved, err := repository.saveAvatar(ctx, userID, filename, content, &previous)
	if err != nil {
		return err
	}
	if !saved {
		return fmt.Errorf("头像已被并发修改，未覆盖；请重新运行导入")
	}
	return nil
}
