package scheduler

import (
	"strings"
	"testing"
	"time"
)

// 「下次运行时间」必须晚于"现在"（2026-09-20 现场：列表里的下次运行时间比当前时间早）。
//
// 原因是那列只在保存/启停时写过一次、之后没人推进（真正的触发由进程内 gocron 自己算），
// 时间一久就成了过去。现在改为**实时算**（displayNextRunTime），这条用例钉住"永远在未来"，
// 以及另外两种"没有下次"的情况：任务停用、实现未迁移（调度器根本不注册它）。
func TestDisplayNextRunTimeIsAlwaysInTheFuture(t *testing.T) {
	task := Task{ID: 1, Code: "cleanup_login_audit_logs", Name: "登录日志清理", Enabled: true, Supported: true, EffectiveCronExpression: "*/5 * * * *"}

	next := displayNextRunTime(task)
	if next == nil {
		t.Fatal("启用的、已实现的任务应有下次运行时间")
	}
	parsed, err := time.Parse("2006-01-02T15:04:05.999999Z", *next)
	if err != nil {
		t.Fatalf("时间格式不对：%v (%q)", err, *next)
	}
	if !parsed.After(time.Now().UTC().Add(-time.Second)) {
		t.Fatalf("下次运行时间必须在未来，得到 %s（现在 %s）", parsed, time.Now().UTC())
	}
}

func TestDisplayNextRunTimeHasNoValueWhenItWillNotRun(t *testing.T) {
	cases := []struct {
		name string
		task Task
	}{
		{"已停用", Task{Enabled: false, Supported: true, EffectiveCronExpression: "*/5 * * * *"}},
		{"实现未迁移（调度会跳过它）", Task{Enabled: true, Supported: false, EffectiveCronExpression: "*/5 * * * *"}},
		{"没有 cron 表达式", Task{Enabled: true, Supported: true, EffectiveCronExpression: ""}},
		{"cron 非法", Task{Enabled: true, Supported: true, EffectiveCronExpression: "not a cron"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := displayNextRunTime(testCase.task); got != nil {
				t.Fatalf("不该给出下次运行时间，得到 %q", *got)
			}
		})
	}
}

// 展示与落库用的是同一套计算：cron 按 ScheduleLocation 解释、结果存 UTC。
// 钉住"时区只有一处"——以后若改成固定时区，这条会跟着走，不会出现两处各算一套。
func TestNextRunUsesScheduleLocation(t *testing.T) {
	original := ScheduleLocation
	defer func() { ScheduleLocation = original }()

	// 固定到一个没有任何 DST 的时区，断言"算出来的时刻落在这个时区的下一个整点"。
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("时区数据不可用：%v", err)
	}
	ScheduleLocation = shanghai

	next, err := nextRun("0 * * * *", true)
	if err != nil {
		t.Fatalf("nextRun: %v", err)
	}
	if !next.Valid {
		t.Fatal("启用的任务应算出下次时间")
	}
	if minute := next.Time.UTC().Minute(); minute != 0 {
		t.Fatalf("整点任务的分钟应为 0，得到 %d", minute)
	}
	// 按上海时区看，这个时刻必须是整点（而不是 UTC 整点）——说明解释时区生效了。
	if minute := next.Time.In(shanghai).Minute(); minute != 0 {
		t.Fatalf("按调度时区看也应是整点，得到 %d", minute)
	}
	if !next.Time.After(time.Now().UTC()) {
		t.Fatalf("下次时间应在未来：%s", next.Time)
	}
}

func TestNextRunDisabledReturnsNothing(t *testing.T) {
	next, err := nextRun("*/5 * * * *", false)
	if err != nil || next.Valid {
		t.Fatalf("停用任务不应有下次时间：%+v err=%v", next, err)
	}
}

func TestNextRunRejectsMalformedCron(t *testing.T) {
	for _, expression := range []string{"invalid", "* * * *", "* * * * * *", ""} {
		if _, err := nextRun(expression, true); err == nil {
			t.Fatalf("%q 应被判为非法 cron", expression)
		}
	}
}

// 调度时区必须随 DTO 一起给界面：cron 的钟点是"调度时区"的钟点，用户在自己的时区里看到
// 别的钟点很正常，界面得能解释这一点。
func TestTaskCarriesScheduleTimezone(t *testing.T) {
	task := Task{Code: "cleanup_login_audit_logs", Enabled: true, EffectiveCronExpression: "*/5 * * * *"}
	withTaskSupport(&task)
	if strings.TrimSpace(task.ScheduleTimezone) == "" {
		t.Fatal("任务 DTO 应带上调度时区")
	}
}
