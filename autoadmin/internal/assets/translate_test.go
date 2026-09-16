package assets

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// pgTestError 模拟 PG 驱动的错误：pgx 的 *pgconn.PgError 暴露 SQLState() 与
// 含报文的 Error()，这里只复刻 translate 用到的这两点。
type pgTestError struct {
	state   string
	message string
}

func (err pgTestError) Error() string    { return err.message + " (SQLSTATE " + err.state + ")" }
func (err pgTestError) SQLState() string { return err.state }

func TestTranslateMySQLCodes(t *testing.T) {
	cases := []struct {
		number uint16
		want   error
	}{
		{1062, ErrDuplicate},
		{1451, ErrDeleteProtected},
		{1452, ErrInvalidRelation},
	}
	for _, testCase := range cases {
		err := translate(&mysql.MySQLError{Number: testCase.number, Message: "x"})
		if !errors.Is(err, testCase.want) {
			t.Errorf("MySQL 错误号 %d 应翻译为 %v，实际 %v", testCase.number, testCase.want, err)
		}
	}
}

func TestTranslatePostgresStates(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "唯一约束冲突",
			err:  pgTestError{state: "23505", message: `duplicate key value violates unique constraint "assets_host_instance_name_uniq"`},
			want: ErrDuplicate,
		},
		{
			name: "删除被引用行",
			err:  pgTestError{state: "23503", message: `update or delete on table "assets_project" violates foreign key constraint "x" on table "assets_business_system"`},
			want: ErrDeleteProtected,
		},
		{
			name: "关联行不存在",
			err:  pgTestError{state: "23503", message: `insert or update on table "assets_business_system" violates foreign key constraint "x" on table "assets_project"`},
			want: ErrInvalidRelation,
		},
		{
			name: "报文不匹配时退到关联不存在",
			err:  pgTestError{state: "23503", message: "some other wording"},
			want: ErrInvalidRelation,
		},
		{
			name: "其他 SQLSTATE 原样返回",
			err:  pgTestError{state: "42P01", message: `relation "x" does not exist`},
			want: nil,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			translated := translate(testCase.err)
			if testCase.want == nil {
				if !errors.Is(translated, testCase.err) {
					t.Fatalf("未识别的 PG 错误应原样返回，实际 %v", translated)
				}
				return
			}
			if !errors.Is(translated, testCase.want) {
				t.Fatalf("应翻译为 %v，实际 %v", testCase.want, translated)
			}
		})
	}
}

func TestTranslateKeepsUnknownErrors(t *testing.T) {
	original := errors.New("connection reset by peer")
	if translated := translate(original); !errors.Is(translated, original) {
		t.Fatalf("未知错误应原样返回，实际 %v", translated)
	}
	if translated := translate(nil); translated != nil {
		t.Fatalf("nil 应翻译为 nil，实际 %v", translated)
	}
}
