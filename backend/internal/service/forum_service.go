package service

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrForumMuted    = infraerrors.Forbidden("FORUM_MUTED", "当前处于禁言期间")
	ErrForumTooFast  = infraerrors.TooManyRequests("FORUM_TOO_FAST", "发言过于频繁，请稍后再试")
	ErrForumAdmin    = infraerrors.Forbidden("FORUM_ADMIN_REQUIRED", "仅管理员可执行此操作")
	ErrForumNotFound = infraerrors.NotFound("FORUM_NOT_FOUND", "记录不存在")
)

type ForumAuthor struct {
	UserID     int64   `json:"user_id"`
	Username   string  `json:"username"`
	AvatarURL  string  `json:"avatar_url"`
	ActualCost float64 `json:"actual_cost"`
}
type ForumMessage struct {
	ID        int64       `json:"id"`
	Author    ForumAuthor `json:"author"`
	Content   string      `json:"content"`
	Rank      int         `json:"rank"`
	Pinned    bool        `json:"pinned"`
	CreatedAt time.Time   `json:"created_at"`
}
type ForumSettings struct {
	MutedUntil    *time.Time `json:"muted_until"`
	RetentionDays int        `json:"retention_days"`
}
type ForumPage struct {
	Messages   []ForumMessage `json:"messages"`
	Pinned     []ForumMessage `json:"pinned"`
	Settings   ForumSettings  `json:"settings"`
	MutedUntil *time.Time     `json:"muted_until"`
	HasMore    bool           `json:"has_more"`
}
type ForumArrival struct {
	ID        int64       `json:"id"`
	Author    ForumAuthor `json:"author"`
	Rank      int         `json:"rank"`
	CreatedAt time.Time   `json:"created_at"`
}
type ForumEvents struct {
	Cursor   int64          `json:"cursor"`
	Arrivals []ForumArrival `json:"arrivals"`
}
type ForumMute struct {
	Author     ForumAuthor `json:"author"`
	MutedUntil time.Time   `json:"muted_until"`
}
type ForumRepository interface {
	Page(context.Context, int64, int64) (*ForumPage, error)
	Create(context.Context, int64, string) (*ForumMessage, error)
	Events(context.Context, *int64) (*ForumEvents, error)
	Arrive(context.Context, int64, string) error
	UpdateSettings(context.Context, int64, *int, *int) error
	Mute(context.Context, int64, int64, int) error
	Mutes(context.Context, int64) ([]ForumMute, error)
	Pin(context.Context, int64, int64, bool) error
	Clear(context.Context, int64) error
	Cleanup(context.Context) error
}
type ForumService struct {
	repo      ForumRepository
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewForumService(repo ForumRepository) *ForumService {
	return &ForumService{repo: repo, stop: make(chan struct{}), done: make(chan struct{})}
}
func (s *ForumService) Page(ctx context.Context, user, before int64) (*ForumPage, error) {
	return s.repo.Page(ctx, user, before)
}
func (s *ForumService) Create(ctx context.Context, user int64, content string) (*ForumMessage, error) {
	content = strings.TrimSpace(content)
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) || utf8.RuneCountInString(content) < 1 || utf8.RuneCountInString(content) > 2000 {
		return nil, infraerrors.BadRequest("FORUM_CONTENT_INVALID", "发言须为 1 至 2000 个字符")
	}
	return s.repo.Create(ctx, user, content)
}
func (s *ForumService) Events(ctx context.Context, after *int64) (*ForumEvents, error) {
	return s.repo.Events(ctx, after)
}
func (s *ForumService) Arrive(ctx context.Context, user int64, session string) error {
	// 旧版无 sid 的令牌可正常使用网站，但不能生成无法稳定去重的上线通报。
	if session == "" {
		return nil
	}
	return s.repo.Arrive(ctx, user, session)
}
func (s *ForumService) UpdateSettings(ctx context.Context, actor int64, minutes, days *int) error {
	if minutes == nil && days == nil || minutes != nil && (*minutes < 0 || *minutes > 525600) || days != nil && (*days < 0 || *days > 3650) {
		return infraerrors.BadRequest("FORUM_SETTINGS_INVALID", "禁言时长或保留天数无效")
	}
	return s.repo.UpdateSettings(ctx, actor, minutes, days)
}
func (s *ForumService) Mute(ctx context.Context, actor, user int64, minutes int) error {
	if user <= 0 || minutes < 0 || minutes > 525600 {
		return infraerrors.BadRequest("FORUM_MUTE_INVALID", "禁言时长无效")
	}
	return s.repo.Mute(ctx, actor, user, minutes)
}
func (s *ForumService) Mutes(ctx context.Context, actor int64) ([]ForumMute, error) {
	return s.repo.Mutes(ctx, actor)
}
func (s *ForumService) Pin(ctx context.Context, actor, message int64, pinned bool) error {
	return s.repo.Pin(ctx, actor, message, pinned)
}
func (s *ForumService) Clear(ctx context.Context, actor int64) error { return s.repo.Clear(ctx, actor) }

// 昵称可能由 OAuth 邮箱初始化，公开讨论中保持与排行榜相同的隐私规则。
func ForumDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if strings.Contains(name, "@") {
		return ""
	}
	return name
}
func (s *ForumService) Start() {
	s.startOnce.Do(func() {
		go func() {
			defer close(s.done)
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := s.repo.Cleanup(ctx)
				cancel()
				if err != nil {
					logger.LegacyPrintf("service.forum", "论坛定时清理失败: %v", err)
				}
				select {
				case <-s.stop:
					return
				case <-ticker.C:
				}
			}
		}()
	})
}
func (s *ForumService) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	s.startOnce.Do(func() { close(s.done) })
	<-s.done
}
func ProvideForumService(repo ForumRepository) *ForumService {
	s := NewForumService(repo)
	s.Start()
	return s
}
