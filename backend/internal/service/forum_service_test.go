package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type forumValidationRepo struct {
	ForumRepository
	calls   int
	content string
}

func (r *forumValidationRepo) Create(_ context.Context, _ int64, content string) (*ForumMessage, error) {
	r.calls++
	r.content = content
	return &ForumMessage{Content: content}, nil
}
func TestForumContentAndSettingsValidation(t *testing.T) {
	r := &forumValidationRepo{}
	s := NewForumService(r)
	ctx := context.Background()
	for _, body := range []string{"", " \n\t", strings.Repeat("中", 2001), "a\x00b", string([]byte{0xff})} {
		_, err := s.Create(ctx, 1, body)
		require.Error(t, err)
	}
	require.Zero(t, r.calls)
	_, err := s.Create(ctx, 1, "  "+strings.Repeat("中", 2000)+" \n")
	require.NoError(t, err)
	require.Equal(t, 1, r.calls)
	require.Equal(t, strings.Repeat("中", 2000), r.content)
	bad := -1
	require.Error(t, s.UpdateSettings(ctx, 1, &bad, nil))
	require.Error(t, s.UpdateSettings(ctx, 1, nil, nil))
	days := 3651
	require.Error(t, s.UpdateSettings(ctx, 1, nil, &days))
	require.Error(t, s.Mute(ctx, 1, 2, -1))
	require.Empty(t, ForumDisplayName(" email@example.com "))
	require.Equal(t, "昵称", ForumDisplayName(" 昵称 "))
}
