package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelContributionValidation(t *testing.T) {
	valid := func() ChannelContributionInput {
		return ChannelContributionInput{SiteName: " 示例站点 ", BaseURL: "https://api.example.com/v1/", APIKey: "sk-test", NewGroupName: " 共享分组 "}
	}
	input := valid()
	require.NoError(t, validateContribution(&input))
	require.Equal(t, "示例站点", input.SiteName)
	require.Equal(t, "共享分组", input.NewGroupName)
	require.Equal(t, PlatformOpenAI, input.Platform)
	require.Equal(t, "https://api.example.com/v1", input.BaseURL)
	groupID := int64(1)
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini} {
		input := valid()
		input.Platform, input.NewGroupName, input.GroupID = platform, "", &groupID
		require.NoError(t, validateContribution(&input))
	}
	for _, target := range []string{"http://api.example.com", "https://localhost", "https://127.0.0.1", "https://10.0.0.1", "https://[::1]", "https://user:secret@example.com", "https://example.com?key=secret", "https://example.com#secret", "javascript:alert(1)", "https://example.com:99999"} {
		t.Run(target, func(t *testing.T) {
			input := valid()
			input.BaseURL = target
			require.Error(t, validateContribution(&input))
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*ChannelContributionInput)
	}{
		{"两种分组同时填写", func(i *ChannelContributionInput) { i.GroupID = &groupID }},
		{"未填写分组", func(i *ChannelContributionInput) { i.NewGroupName = "" }},
		{"不支持的平台", func(i *ChannelContributionInput) { i.Platform = "unknown" }},
		{"空站名", func(i *ChannelContributionInput) { i.SiteName = " " }},
		{"站名控制字符", func(i *ChannelContributionInput) { i.SiteName = "a\nb" }},
		{"超长组名", func(i *ChannelContributionInput) { i.NewGroupName = strings.Repeat("组", 101) }},
		{"空密钥", func(i *ChannelContributionInput) { i.APIKey = " " }},
		{"密钥换行", func(i *ChannelContributionInput) { i.APIKey = "sk-a\nb" }},
		{"超长密钥", func(i *ChannelContributionInput) { i.APIKey = strings.Repeat("k", 4097) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid()
			tc.edit(&input)
			require.Error(t, validateContribution(&input))
		})
	}
}
