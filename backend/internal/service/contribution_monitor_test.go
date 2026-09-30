//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type contributionProbeStub struct {
	contributionMonitorRepository
	account *Account
	err     error
}

func (s *contributionProbeStub) ContributionProbeAccount(context.Context, int64) (*Account, error) {
	return s.account, s.err
}

type contributionProbeTransport func(*http.Request) (*http.Response, error)

func (f contributionProbeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestContributionMonitorRealProbeAndGuards(t *testing.T) {
	ctx := context.Background()
	id := int64(9)
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{ID: 7, ContributionID: 3, AccountID: &id, Enabled: true, PrimaryModel: "alias", APIKey: "linked-contribution", APIMode: MonitorAPIModeChatCompletions}}
	svc := newQuotaModeService(repo)
	source := &contributionProbeStub{account: &Account{ID: id, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Name: "贡献测试", Status: StatusActive, Schedulable: true, Credentials: map[string]any{"base_url": "https://api.example.com/v1", "api_key": "test-current-key", "model_mapping": map[string]any{"alias": "upstream-model"}}}}
	svc.contributions = source
	oldClient, oldPing := monitorHTTPClient, monitorPingHTTPClient
	t.Cleanup(func() { monitorHTTPClient, monitorPingHTTPClient = oldClient, oldPing })
	handler := &openAICaptureHandler{}
	requests := 0
	lastHost := ""
	monitorHTTPClient = &http.Client{Transport: contributionProbeTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		lastHost = r.URL.Host
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Result(), nil
	})}
	monitorPingHTTPClient = &http.Client{Transport: contributionProbeTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		w.WriteHeader(http.StatusOK)
		return w.Result(), nil
	})}
	results, err := svc.RunCheck(ctx, 7)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusOperational, results[0].Status)
	require.Equal(t, "alias", results[0].Model)
	require.NotNil(t, results[0].LatencyMs)
	require.NotNil(t, results[0].PingLatencyMs)
	require.Equal(t, "upstream-model", handler.lastBody["model"])
	require.Equal(t, "Bearer test-current-key", handler.lastHeaders.Get("Authorization"))
	require.Equal(t, "/v1/chat/completions", handler.lastPath)
	source.account.Credentials["api_key"] = "test-rotated-key"
	source.account.Credentials["base_url"] = "https://new.example.com/v1"
	repo.monitor.APIMode = MonitorAPIModeResponses
	results, err = svc.RunCheck(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, MonitorStatusOperational, results[0].Status)
	require.Equal(t, "new.example.com", lastHost)
	require.Equal(t, "Bearer test-rotated-key", handler.lastHeaders.Get("Authorization"))
	require.Equal(t, "/v1/responses", handler.lastPath)
	require.Len(t, repo.history, 2)
	require.Equal(t, "alias", repo.history[0].Model)
	for _, rt := range []ChannelMonitorRuntime{{Enabled: true, Mode: ChannelMonitorModeV2}, {Enabled: false, Mode: ChannelMonitorModeV1}} {
		svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: rt})
		_, err := svc.RunCheck(ctx, 7)
		require.Error(t, err)
	}
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	for _, state := range []string{"disabled", "removed", "private_url", "model_removed"} {
		source.account.Schedulable = true
		source.err = nil
		source.account.Credentials["base_url"] = "https://api.example.com"
		repo.monitor.PrimaryModel = "alias"
		switch state {
		case "disabled":
			source.account.Schedulable = false
		case "removed":
			source.err = ErrContributionNotFound
		case "private_url":
			source.account.Credentials["base_url"] = "https://127.0.0.1"
		case "model_removed":
			repo.monitor.PrimaryModel = "missing"
		}
		_, err := svc.RunCheck(ctx, 7)
		require.Error(t, err, state)
	}
	require.Equal(t, 2, requests)
	require.Len(t, repo.history, 2)
	_, err = svc.GetUserDetail(ctx, 7)
	require.ErrorIs(t, err, ErrChannelMonitorNotFound)
}

func TestContributionMonitorConfigValidation(t *testing.T) {
	in := ContributionAccountInput{ShowInStatus: true, Models: []string{"gpt-5"}}
	require.NoError(t, validateContributionMonitor(&in))
	require.Equal(t, "gpt-5", in.MonitorModel)
	require.Equal(t, MonitorAPIModeChatCompletions, in.MonitorAPIMode)
	for _, model := range []string{"missing", "*", "bad model"} {
		in.MonitorModel = model
		require.Error(t, validateContributionMonitor(&in))
	}
	in.MonitorModel = "gpt-5"
	in.MonitorAPIMode = "invalid"
	require.Error(t, validateContributionMonitor(&in))
}
