package sharedquota

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"reset_time":"00:00"}`, `{"daily_quota":null,"reset_time":"00:00"}`, `{"daily_quota":-1,"reset_time":"00:00"}`, `{"daily_quota":1,"reset_time":"24:00"}`, `{"daily_quota":1,"reset_time":"0:00"}`, `{"daily_quota":1,"reset_time":"00:00","extra":1}`, `{"daily_quota":1,"reset_time":"00:00"} {}`} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("应拒绝配置 %s", raw)
		}
	}
	if _, err := Parse([]byte(`{"daily_quota":0,"reset_time":"00:00"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestDailyResetBoundaryAndDST(t *testing.T) {
	for _, tt := range []struct{ zone, now, start, end string }{
		{"Asia/Shanghai", "2026-09-25T01:59:59+08:00", "2026-09-24T02:00:00+08:00", "2026-09-25T02:00:00+08:00"},
		{"Asia/Shanghai", "2026-09-25T02:00:00+08:00", "2026-09-25T02:00:00+08:00", "2026-09-26T02:00:00+08:00"},
	} {
		loc, err := time.LoadLocation(tt.zone)
		if err != nil {
			t.Fatal(err)
		}
		now, _ := time.Parse(time.RFC3339, tt.now)
		wantStart, _ := time.Parse(time.RFC3339, tt.start)
		wantEnd, _ := time.Parse(time.RFC3339, tt.end)
		a, b := (Config{ResetTime: "02:00"}).Window(now, loc)
		if !a.Equal(wantStart) || !b.Equal(wantEnd) {
			t.Fatalf("账期错误: %s %s", a, b)
		}
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2026-03-08T12:00:00-04:00")
	a, b := (Config{ResetTime: "00:00"}).Window(now, loc)
	if b.Sub(a) != 23*time.Hour {
		t.Fatalf("夏令时账期长度应为23小时，得到 %s", b.Sub(a))
	}
}
