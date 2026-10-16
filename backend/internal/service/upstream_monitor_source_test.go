//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type upstreamMonitorSourceTestRepo struct {
	*upstreamTestRepo
	source *UpstreamTarget
}

func (r *upstreamMonitorSourceTestRepo) GetTarget(ctx context.Context, id int64) (*UpstreamTarget, error) {
	if r.source != nil && r.source.ID == id {
		copy := *r.source
		return &copy, nil
	}
	return r.upstreamTestRepo.GetTarget(ctx, id)
}

func monitorSourceFixture() (*UpstreamCenterService, *upstreamMonitorSourceTestRepo) {
	siteID := int64(9)
	source := validUpstreamTestTarget()
	source.ID, source.SupplierID = 17, &siteID
	source.AccountIDs = []int64{101, 102}
	source.NewAPIUserID, source.NewAPIAccessTokenEncrypted = 42, "encrypted:source-console"
	source.NewAPICredentialsInherited = true
	repo := &upstreamMonitorSourceTestRepo{upstreamTestRepo: &upstreamTestRepo{}, source: source}
	return NewUpstreamCenterService(repo, upstreamTestEncryptor{}, nil, nil), repo
}

func TestUpstreamMonitorSourceImportsGroupWithoutBusinessBindings(t *testing.T) {
	svc, repo := monitorSourceFixture()
	result, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SourceTargetID: &repo.source.ID})
	require.NoError(t, err)
	require.Equal(t, repo.source.Name, result.Name)
	require.Equal(t, repo.source.Provider, result.Provider)
	require.Equal(t, repo.source.Endpoint, result.Endpoint)
	require.Equal(t, repo.source.APIMode, result.APIMode)
	require.Equal(t, repo.source.Models, result.Models)
	require.Equal(t, repo.source.ID, *result.SourceTargetID)
	require.Nil(t, result.SupplierID)
	require.Empty(t, result.AccountIDs)
	require.Equal(t, repo.source.APIKeyEncrypted, repo.saved.APIKeyEncrypted)
	require.Equal(t, repo.source.NewAPIAccessTokenEncrypted, repo.saved.NewAPIAccessTokenEncrypted)
	require.True(t, result.NewAPICredentialsInherited)
	require.True(t, result.NewAPIAccessTokenConfigured)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "source-console")
	require.NotContains(t, string(encoded), "secret-testing-key")

	name, models := "Custom monitor", []string{"different-model"}
	result, err = svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SourceTargetID: &repo.source.ID, Name: &name, Models: &models})
	require.NoError(t, err)
	require.Equal(t, name, result.Name)
	require.Equal(t, models, result.Models)
}

func TestUpstreamMonitorSourceRejectsConflictingOrUnavailableSources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*upstreamMonitorSourceTestRepo, *UpstreamTargetInput)
	}{
		{"independent source", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) { r.source.SupplierID = nil }},
		{"supplier destination", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) { in.SupplierID = json.RawMessage("9") }},
		{"other credential", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) {
			key := "another-key"
			in.APIKey = &key
		}},
		{"account source", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) {
			id := int64(12)
			in.SourceAccountID = &id
		}},
		{"other endpoint", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) {
			endpoint := "https://1.1.1.1"
			in.Endpoint = &endpoint
		}},
		{"other provider", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) {
			provider := "gemini"
			in.Provider = &provider
		}},
		{"negative id", func(r *upstreamMonitorSourceTestRepo, in *UpstreamTargetInput) {
			id := int64(-1)
			in.SourceTargetID = &id
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := monitorSourceFixture()
			in := UpstreamTargetInput{SourceTargetID: &repo.source.ID}
			tc.change(repo, &in)
			_, err := svc.SaveTarget(context.Background(), 0, in)
			require.ErrorIs(t, err, ErrUpstreamInvalid)
			require.Nil(t, repo.saved)
		})
	}
	svc, repo := monitorSourceFixture()
	missing := int64(999)
	_, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SourceTargetID: &missing})
	require.ErrorIs(t, err, ErrUpstreamNotFound)
	require.Nil(t, repo.saved)
}

func TestUpstreamMonitorSourceKeepsCopiedKeyAndFollowsAuthorization(t *testing.T) {
	svc, repo := monitorSourceFixture()
	_, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SourceTargetID: &repo.source.ID})
	require.NoError(t, err)
	repo.target = repo.saved
	repo.target.ID = 7
	repo.source.APIKeyEncrypted = "encrypted:new-group-key"
	repo.source.NewAPIAccessTokenEncrypted = "encrypted:rotated-console"
	result, err := svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{Endpoint: &repo.target.Endpoint, Provider: &repo.target.Provider})
	require.NoError(t, err)
	require.Equal(t, "encrypted:secret-testing-key", repo.saved.APIKeyEncrypted, "editing a schedule must not silently replace the copied inference key")
	require.Equal(t, repo.source.ID, *result.SourceTargetID)
	require.Equal(t, "encrypted:rotated-console", repo.saved.NewAPIAccessTokenEncrypted)
	repo.source.NewAPIUserID, repo.source.NewAPIAccessTokenEncrypted = 0, ""
	result, err = svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{})
	require.NoError(t, err)
	require.False(t, result.NewAPIAccessTokenConfigured)

	zero := int64(0)
	result, err = svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{SourceTargetID: &zero})
	require.NoError(t, err)
	require.Nil(t, result.SourceTargetID)
	require.False(t, result.NewAPICredentialsInherited)
	require.Empty(t, repo.saved.NewAPIAccessTokenEncrypted)
}

func TestUpstreamMonitorSourceManualCredentialsDetachAuthorization(t *testing.T) {
	svc, repo := monitorSourceFixture()
	_, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SourceTargetID: &repo.source.ID})
	require.NoError(t, err)
	repo.target = repo.saved
	repo.target.ID = 7
	endpoint, key := "https://1.1.1.1", "manual-new-key"
	result, err := svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{Endpoint: &endpoint, APIKey: &key})
	require.NoError(t, err)
	require.Nil(t, result.SourceTargetID)
	require.False(t, result.NewAPICredentialsInherited)
	require.Empty(t, repo.saved.NewAPIAccessTokenEncrypted)
}

func TestUpstreamMonitorSourceDetachAcceptsExplicitIndependentAuthorization(t *testing.T) {
	svc, repo := monitorSourceFixture()
	_, err := svc.SaveTarget(context.Background(), 0, UpstreamTargetInput{SourceTargetID: &repo.source.ID})
	require.NoError(t, err)
	repo.target = repo.saved
	repo.target.ID = 7
	zero, userID, token := int64(0), int64(42), "independent-console"
	result, err := svc.SaveTarget(context.Background(), 7, UpstreamTargetInput{SourceTargetID: &zero, NewAPIUserID: &userID, NewAPIAccessToken: &token})
	require.NoError(t, err)
	require.Nil(t, result.SourceTargetID)
	require.False(t, result.NewAPICredentialsInherited)
	require.Equal(t, userID, result.NewAPIUserID)
	require.Equal(t, "encrypted:independent-console", repo.saved.NewAPIAccessTokenEncrypted)
}

func TestUpstreamMonitorSourceModelsUsesSavedGroupWithoutBindings(t *testing.T) {
	svc, repo := monitorSourceFixture()
	repo.source.Provider = MonitorProviderOpenAI
	calls := 0
	svc.modelsClient.Transport = upstreamModelsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https://8.8.8.8/v1/models", r.URL.String())
		require.Equal(t, "Bearer secret-testing-key", r.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"source-model"}]}`))}, nil
	})
	models, err := svc.Models(context.Background(), UpstreamModelsInput{SourceTargetID: &repo.source.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"source-model"}, models)
	require.Nil(t, repo.saved)
	zero, otherID := int64(0), int64(77)
	for _, input := range []UpstreamModelsInput{
		{SourceTargetID: &zero},
		{SourceTargetID: &repo.source.ID, TargetID: &otherID},
		{SourceTargetID: &repo.source.ID, AccountID: &otherID},
		{SourceTargetID: &repo.source.ID, APIKey: "other-key"},
		{SourceTargetID: &repo.source.ID, Endpoint: "https://1.1.1.1"},
		{SourceTargetID: &repo.source.ID, Provider: MonitorProviderGemini},
	} {
		_, err = svc.Models(context.Background(), input)
		require.ErrorIs(t, err, ErrUpstreamInvalid)
	}
	require.Equal(t, 1, calls)
}
