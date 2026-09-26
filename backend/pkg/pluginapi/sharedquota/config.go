// Package sharedquota 定义全站共享额度插件的声明式配置契约。
package sharedquota

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"time"
)

const Capability = "billing.shared_daily_quota.v1"

const (
	ApplyImmediately = "immediate"
	ApplyNextReset   = "next_reset"
)

type Config struct {
	DailyQuota  float64           `json:"daily_quota"`
	ResetTime   string            `json:"reset_time"`
	ApplyMode   string            `json:"apply_mode,omitempty"`
	GroupLimits map[int64]float64 `json:"group_limits,omitempty"`
}

func Parse(raw []byte) (Config, error) {
	var c Config
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return c, err
	}
	if v, ok := fields["daily_quota"]; !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return c, errors.New("必须设置每日共享额度")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, errors.New("配置必须是单个 JSON 对象")
	}
	if math.IsNaN(c.DailyQuota) || math.IsInf(c.DailyQuota, 0) || c.DailyQuota < 0 || c.DailyQuota > 1e10 {
		return c, errors.New("每日共享额度必须介于 0 和 10000000000 之间")
	}
	if rawGroups, ok := fields["group_limits"]; ok {
		var groups map[string]json.RawMessage
		if err := json.Unmarshal(rawGroups, &groups); err != nil {
			return c, errors.New("分组额度格式无效")
		}
		for key, value := range groups {
			id, err := strconv.ParseInt(key, 10, 64)
			if err != nil || id <= 0 || strconv.FormatInt(id, 10) != key {
				return c, errors.New("分组 ID 无效")
			}
			amount := c.GroupLimits[id]
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > 1e10 {
				return c, errors.New("分组每日上限必须介于 0 和 10000000000 之间")
			}
		}
	}
	t, err := time.Parse("15:04", c.ResetTime)
	if err != nil || t.Format("15:04") != c.ResetTime {
		return c, errors.New("每日刷新时间必须为 HH:mm")
	}
	if c.ApplyMode == "" {
		c.ApplyMode = ApplyImmediately
	}
	if c.ApplyMode != ApplyImmediately && c.ApplyMode != ApplyNextReset {
		return c, errors.New("额度生效方式必须为立即生效或下次刷新时生效")
	}
	return c, nil
}

// Window 使用日历日而非 24 小时偏移，兼容站点时区的夏令时。
func (c Config) Window(now time.Time, loc *time.Location) (time.Time, time.Time) {
	t, _ := time.Parse("15:04", c.ResetTime)
	n := now.In(loc)
	start := time.Date(n.Year(), n.Month(), n.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	if n.Before(start) {
		start = time.Date(n.Year(), n.Month(), n.Day()-1, t.Hour(), t.Minute(), 0, 0, loc)
	}
	end := time.Date(start.Year(), start.Month(), start.Day()+1, t.Hour(), t.Minute(), 0, 0, loc)
	return start.UTC(), end.UTC()
}
