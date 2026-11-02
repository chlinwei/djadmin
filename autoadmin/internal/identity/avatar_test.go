package identity

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func avatarPNG(t *testing.T, width, height int, noise bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	random := rand.New(rand.NewSource(42))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := color.NRGBA{R: 80, G: 160, B: 230, A: 128}
			if noise {
				pixel = color.NRGBA{R: uint8(random.Intn(256)), G: uint8(random.Intn(256)), B: uint8(random.Intn(256)), A: uint8(random.Intn(256))}
			}
			img.SetNRGBA(x, y, pixel)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestProcessAvatarWebP(t *testing.T) {
	source, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	extended := make([]byte, len(source)+18)
	copy(extended, source[:12])
	copy(extended[12:16], "VP8X")
	binary.LittleEndian.PutUint32(extended[16:20], 10)
	copy(extended[30:], source[12:])
	binary.LittleEndian.PutUint32(extended[4:8], uint32(len(extended)-8))
	for _, test := range []struct {
		name    string
		source  []byte
		wantErr bool
	}{
		{"simple-webp", source, false},
		{"extended-webp", bytes.Clone(extended), false},
		{"canvas-mismatch", func() []byte {
			b := bytes.Clone(extended)
			b[24] = 1
			return b
		}(), true},
		{"animated-webp", func() []byte {
			b := bytes.Clone(extended)
			b[20] = 2
			return b
		}(), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := processAvatar(test.source)
			if (err != nil) != test.wantErr {
				t.Fatalf("unexpected WebP result: %v", err)
			}
			if !test.wantErr {
				if _, err := png.Decode(bytes.NewReader(result)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestAvatarMediaKeepsOtherFilesAndNeverReadsDiskAvatars(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, avatarDirectory), 0700); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"package.tar.gz", "userAvatar/disk.png"} {
		if err := os.WriteFile(filepath.Join(root, filename), []byte("disk-content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewHandler(NewService(NewRepository(pool), nil))
	engine := gin.New()
	engine.GET("/media/*filepath", handler.ServeMedia(root))
	for _, url := range []string{
		"/media/package.tar.gz", "/media/userAvatar/disk.png", "/media/other/../userAvatar/disk.png",
	} {
		avatar := strings.Contains(url, "disk.png")
		if avatar {
			mock.ExpectQuery("SELECT content, update_time FROM sys_user_avatar").WithArgs("disk.png").
				WillReturnRows(sqlmock.NewRows([]string{"content", "update_time"}))
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
		if avatar {
			if recorder.Code != 404 || bytes.Contains(recorder.Body.Bytes(), []byte("disk-content")) {
				t.Fatalf("disk fallback was used: HTTP %d %q", recorder.Code, recorder.Body.String())
			}
		} else if recorder.Code != 200 || recorder.Body.String() != "disk-content" {
			t.Fatalf("other media behavior changed: HTTP %d %q", recorder.Code, recorder.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminUserUpdateDoesNotOverwriteAvatar(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Now().UTC()
	userRow := func(avatar string) *sqlmock.Rows {
		return sqlmock.NewRows([]string{
			"id", "username", "password", "avatar", "phonenumber", "login_date",
			"status", "create_time", "update_time", "remark", "timezone",
		}).AddRow(7, "user", "hash", avatar, "123", nil, 1, now, now, "", "UTC")
	}
	mock.ExpectQuery("SELECT .* FROM sys_user WHERE id").WithArgs(int32(7)).WillReturnRows(userRow("old.png"))
	mock.ExpectExec("UPDATE sys_user SET phonenumber").
		WithArgs("123", int16(1), "", "UTC", sqlmock.AnyArg(), int32(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM sys_user WHERE id").WithArgs(int32(7)).WillReturnRows(userRow("userAvatar/new.png"))
	user, err := NewService(NewRepository(pool), nil).UpdateUser(context.Background(), 7, UserInput{Status: 1})
	if err != nil || user.Avatar == nil || *user.Avatar != "userAvatar/new.png" {
		t.Fatalf("new avatar lost during admin profile edit: %+v, %v", user, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessAvatar(t *testing.T) {
	t.Run("large-source-final-binary-under-one-MiB", func(t *testing.T) {
		source := avatarPNG(t, 1200, 900, true)
		if len(source) <= avatarStoredLimit || len(source) > avatarUploadLimit {
			t.Fatalf("fixture must be between 1 and 5 MiB: %d", len(source))
		}
		result, err := processAvatar(source)
		if err != nil {
			t.Fatal(err)
		}
		if len(result) == 0 || len(result) > 1048576 {
			t.Fatalf("stored image exceeds exact byte limit: %d", len(result))
		}
		config, err := png.DecodeConfig(bytes.NewReader(result))
		if err != nil || config.Width > 1024 || config.Height > 1024 {
			t.Fatalf("invalid processed image: %+v, %v", config, err)
		}
		if config.Width*3 != config.Height*4 {
			t.Fatalf("aspect ratio changed: %+v", config)
		}
	})
	t.Run("transparency-and-no-upscaling", func(t *testing.T) {
		result, err := processAvatar(avatarPNG(t, 32, 16, false))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(result))
		if err != nil || decoded.Bounds() != image.Rect(0, 0, 32, 16) {
			t.Fatalf("small image changed size: %v, %v", decoded, err)
		}
		_, _, _, alpha := decoded.At(0, 0).RGBA()
		if alpha != 128*257 {
			t.Fatalf("alpha was lost: %d", alpha)
		}
	})
	t.Run("jpeg-and-gif-normalized-to-png", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 24, 24))
		for _, encode := range []func(*bytes.Buffer) error{
			func(buffer *bytes.Buffer) error { return jpeg.Encode(buffer, img, nil) },
			func(buffer *bytes.Buffer) error { return gif.Encode(buffer, img, nil) },
		} {
			var source bytes.Buffer
			if err := encode(&source); err != nil {
				t.Fatal(err)
			}
			result, err := processAvatar(source.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := png.Decode(bytes.NewReader(result)); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestProcessAvatarRejectsInvalidImages(t *testing.T) {
	hugeHeader := func(width, height uint32) []byte {
		source := avatarPNG(t, 1, 1, false)
		binary.BigEndian.PutUint32(source[16:20], width)
		binary.BigEndian.PutUint32(source[20:24], height)
		binary.BigEndian.PutUint32(source[29:33], crc32.ChecksumIEEE(source[12:29]))
		return source
	}
	for name, source := range map[string][]byte{
		"empty": nil, "fake-png": []byte("<script>alert(1)</script>"),
		"over-five-MiB":  make([]byte, avatarUploadLimit+1),
		"dimension-bomb": hugeHeader(8193, 1), "pixel-bomb": hugeHeader(7000, 5000),
		"truncated": avatarPNG(t, 20, 20, false)[:40],
	} {
		t.Run(name, func(t *testing.T) {
			if result, err := processAvatar(source); err == nil || result != nil {
				t.Fatalf("invalid image accepted: %d bytes, %v", len(result), err)
			}
		})
	}
}

func TestSaveAvatarExactStoredLimit(t *testing.T) {
	for _, size := range []int{0, avatarStoredLimit - 1, avatarStoredLimit, avatarStoredLimit + 1} {
		database, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		valid := size > 0 && size <= avatarStoredLimit
		content := make([]byte, size)
		if valid {
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT avatar FROM sys_user .* FOR UPDATE").WithArgs(int32(7)).
				WillReturnRows(sqlmock.NewRows([]string{"avatar"}).AddRow("old.png"))
			mock.ExpectExec("DELETE FROM sys_user_avatar").WithArgs(int32(7)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO sys_user_avatar").WithArgs(int32(7), "new.png", content, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE sys_user SET avatar").WithArgs("userAvatar/new.png", sqlmock.AnyArg(), int32(7)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		saved, err := NewRepository(database).saveAvatar(context.Background(), 7, "new.png", content, nil)
		if saved != valid || (err == nil) != valid {
			t.Fatalf("size=%d saved=%v err=%v", size, saved, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		database.Close()
	}
}

func TestSaveAvatarRollsBackAndProtectsConcurrentImport(t *testing.T) {
	for _, failure := range []string{"lock", "delete", "insert", "update", "commit", "changed"} {
		t.Run(failure, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mock.ExpectBegin()
			lock := mock.ExpectQuery("SELECT avatar FROM sys_user .* FOR UPDATE").WithArgs(int32(7))
			if failure == "lock" {
				lock.WillReturnError(sql.ErrNoRows)
			} else if failure == "changed" {
				lock.WillReturnRows(sqlmock.NewRows([]string{"avatar"}).AddRow("newer.png"))
			} else {
				lock.WillReturnRows(sqlmock.NewRows([]string{"avatar"}).AddRow("old.png"))
				for _, step := range []struct{ name, query string }{
					{"delete", "DELETE FROM sys_user_avatar"},
					{"insert", "INSERT INTO sys_user_avatar"},
					{"update", "UPDATE sys_user SET avatar"},
				} {
					query := mock.ExpectExec(step.query)
					if failure == step.name {
						query.WillReturnError(errors.New("database unavailable"))
						break
					}
					query.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			switch failure {
			case "changed":
				mock.ExpectCommit()
			case "commit":
				mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
			default:
				mock.ExpectRollback()
			}
			previous := "old.png"
			saved, err := NewRepository(database).saveAvatar(context.Background(), 7, "new.png", []byte("image"), &previous)
			if saved || (err == nil) != (failure == "changed") {
				t.Fatalf("saved=%v, err=%v", saved, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestImportDiskAvatars(t *testing.T) {
	for _, sourceFailure := range []string{"", "missing", "corrupt", "traversal", "symlink"} {
		t.Run(sourceFailure, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			mediaRoot := t.TempDir()
			directory := filepath.Join(mediaRoot, avatarDirectory)
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			old := "old.png"
			if sourceFailure == "traversal" {
				old = "../old.png"
			}
			if sourceFailure == "" || sourceFailure == "corrupt" {
				source := avatarPNG(t, 4, 4, false)
				if sourceFailure == "corrupt" {
					source = []byte("invalid image")
				}
				if err := os.WriteFile(filepath.Join(directory, old), source, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if sourceFailure == "symlink" {
				outside := filepath.Join(mediaRoot, "outside.png")
				if err := os.WriteFile(outside, avatarPNG(t, 4, 4, false), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(directory, old)); err != nil {
					t.Fatal(err)
				}
			}
			mock.ExpectQuery("SELECT u.id, u.avatar FROM sys_user").WillReturnRows(
				sqlmock.NewRows([]string{"id", "avatar"}).AddRow(int32(7), old))
			if sourceFailure == "" {
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT avatar FROM sys_user .* FOR UPDATE").WithArgs(int32(7)).
					WillReturnRows(sqlmock.NewRows([]string{"avatar"}).AddRow(old))
				mock.ExpectExec("DELETE FROM sys_user_avatar").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO sys_user_avatar").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE sys_user SET avatar").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = ImportDiskAvatars(context.Background(), database, mediaRoot)
			if (err != nil) != (sourceFailure != "") {
				t.Fatalf("import error: %v", err)
			}
			if sourceFailure == "" {
				if _, err := os.Stat(filepath.Join(directory, old)); err != nil {
					t.Fatalf("source backup was removed: %v", err)
				}
				mock.ExpectQuery("SELECT u.id, u.avatar FROM sys_user").
					WillReturnRows(sqlmock.NewRows([]string{"id", "avatar"}))
				if err := ImportDiskAvatars(context.Background(), database, mediaRoot); err != nil {
					t.Fatalf("rerun failed: %v", err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
