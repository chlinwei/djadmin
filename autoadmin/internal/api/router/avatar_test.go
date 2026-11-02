package router

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"autoadmin/internal/identity"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAvatarUploadPreflight(t *testing.T) {
	const allowedOrigin = "http://10.25.66.150:5173"
	for _, origin := range []string{allowedOrigin, "https://not-allowed.example"} {
		t.Run(origin, func(t *testing.T) {
			engine, err := New(nil, identity.NewTokenManager("test-secret", time.Hour), []string{allowedOrigin}, nil, "", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodOptions, "/user/changeAvatar", nil)
			request.Header.Set("Origin", origin)
			request.Header.Set("Access-Control-Request-Method", http.MethodPost)
			request.Header.Set("Access-Control-Request-Headers", "authorization,content-type,x-requested-with")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if origin != allowedOrigin {
				if recorder.Code != http.StatusForbidden || recorder.Header().Get("Access-Control-Allow-Origin") != "" {
					t.Fatalf("disallowed origin was enabled: HTTP %d %v", recorder.Code, recorder.Header())
				}
				return
			}
			if recorder.Code != http.StatusNoContent || recorder.Header().Get("Access-Control-Allow-Origin") != origin {
				t.Fatalf("preflight failed: HTTP %d %v", recorder.Code, recorder.Header())
			}
			allowedHeaders := map[string]bool{}
			for _, header := range strings.Split(recorder.Header().Get("Access-Control-Allow-Headers"), ",") {
				allowedHeaders[strings.ToLower(strings.TrimSpace(header))] = true
			}
			for _, header := range []string{"authorization", "content-type", "x-requested-with"} {
				if !allowedHeaders[header] {
					t.Errorf("upload header %q not allowed: %v", header, recorder.Header())
				}
			}
		})
	}
}

type processedAvatarArgument struct{}

func (processedAvatarArgument) Match(value driver.Value) bool {
	content, ok := value.([]byte)
	if !ok || len(content) == 0 || len(content) > 1048576 {
		return false
	}
	_, err := png.Decode(bytes.NewReader(content))
	return err == nil
}

func uploadAvatarRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	// A forged ID must not change the self-service target.
	if err := writer.WriteField("user_id", "99"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/user/changeAvatar", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestAvatarUpload(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 24, 16))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 128})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	exactUploadLimit := make([]byte, 5<<20)
	copy(exactUploadLimit, encoded.Bytes())
	for _, test := range []struct {
		name, filename string
		content        []byte
		auth, dbFail   bool
		code           int
	}{
		{"self-service-without-admin-permissions", "face.PNG", encoded.Bytes(), true, false, 200},
		{"exact-five-MiB-source", "face.png", exactUploadLimit, true, false, 200},
		{"unauthenticated", "face.png", encoded.Bytes(), false, false, 301},
		{"invalid-extension", "face.svg", encoded.Bytes(), true, false, 400},
		{"fake-image", "face.png", []byte("<svg/>"), true, false, 400},
		{"too-large", "face.png", make([]byte, (5<<20)+1), true, false, 400},
		{"database-failure", "face.png", encoded.Bytes(), true, true, 600},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			tokens := identity.NewTokenManager("test-secret", time.Hour)
			engine, err := New(pool, tokens, []string{"*"}, nil, "", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			if test.code == 200 || test.dbFail {
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT avatar FROM sys_user .* FOR UPDATE").WithArgs(int32(7)).
					WillReturnRows(sqlmock.NewRows([]string{"avatar"}).AddRow("old.png"))
				mock.ExpectExec("DELETE FROM sys_user_avatar").WithArgs(int32(7)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				insert := mock.ExpectExec("INSERT INTO sys_user_avatar").
					WithArgs(int32(7), sqlmock.AnyArg(), processedAvatarArgument{}, sqlmock.AnyArg())
				if test.dbFail {
					insert.WillReturnError(errors.New("database unavailable"))
					mock.ExpectRollback()
				} else {
					insert.WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("UPDATE sys_user SET avatar").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int32(7)).
						WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
			}
			if test.auth {
				mock.ExpectExec("INSERT INTO audit_operation_log").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			request := uploadAvatarRequest(t, test.filename, test.content)
			if test.auth {
				token, err := tokens.Issue(7, "view-only-user", nil)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", token)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			var envelope struct {
				Code int `json:"code"`
				Data struct {
					Avatar      string `json:"avatar"`
					AvatarURL   string `json:"avatar_url"`
					NewFileName string `json:"new_file_name"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.Code != test.code {
				t.Fatalf("unexpected response: %s (%v)", recorder.Body.String(), err)
			}
			if test.code == 200 {
				if len(envelope.Data.NewFileName) != 36 || !strings.HasSuffix(envelope.Data.NewFileName, ".png") ||
					envelope.Data.Avatar != "userAvatar/"+envelope.Data.NewFileName ||
					envelope.Data.AvatarURL != "/media/"+envelope.Data.Avatar {
					t.Fatalf("invalid avatar URLs: %+v", envelope.Data)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAvatarDatabaseMedia(t *testing.T) {
	for _, test := range []struct {
		name, method, etag string
		found, failed      bool
		status             int
	}{
		{"public-get", http.MethodGet, "", true, false, 200},
		{"head", http.MethodHead, "", true, false, 200},
		{"conditional-get", http.MethodGet, `"avatar.png"`, true, false, 304},
		{"missing-no-disk-fallback", http.MethodGet, "", false, false, 404},
		{"database-unavailable", http.MethodGet, "", false, true, 500},
		{"post-not-a-query", http.MethodPost, "", false, false, 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			engine, err := New(pool, identity.NewTokenManager("test-secret", time.Hour), []string{"*"}, nil, "", "test-secret")
			if err != nil {
				t.Fatal(err)
			}
			if test.method != http.MethodPost {
				query := mock.ExpectQuery("SELECT content, update_time FROM sys_user_avatar").
					WithArgs("avatar.png")
				switch {
				case test.failed:
					query.WillReturnError(errors.New("database unavailable"))
				case test.found:
					query.WillReturnRows(sqlmock.NewRows([]string{"content", "update_time"}).
						AddRow([]byte("image-bytes"), time.Now().UTC()))
				default:
					query.WillReturnRows(sqlmock.NewRows([]string{"content", "update_time"}))
				}
			}
			request := httptest.NewRequest(test.method, "/media/userAvatar/avatar.png", nil)
			if test.etag != "" {
				request.Header.Set("If-None-Match", test.etag)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("HTTP %d: %s", recorder.Code, recorder.Body.String())
			}
			if test.found {
				if recorder.Header().Get("ETag") != `"avatar.png"` ||
					recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf("missing image/cache headers: %v", recorder.Header())
				}
				if test.status == 200 && recorder.Header().Get("Content-Type") != "image/png" {
					t.Fatalf("not an image response: %v", recorder.Header())
				}
				if test.method == http.MethodHead || test.status == 304 {
					if recorder.Body.Len() != 0 {
						t.Fatalf("conditional/HEAD returned a body: %q", recorder.Body.String())
					}
				} else if recorder.Body.String() != "image-bytes" {
					t.Fatalf("image bytes changed: %q", recorder.Body.String())
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
