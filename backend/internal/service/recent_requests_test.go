package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildRecentRequestRecordPreservesUnknownAndSnapshotsProxy(t *testing.T) {
	proxyID := int64(3)
	account := &Account{ID: 12, Name: "account", Platform: PlatformOpenAI, ProxyID: &proxyID, Proxy: &Proxy{ID: proxyID, Name: "node A"}}
	result := BuildRecentRequestRecord(account, "gpt-5", false, 0, errors.New("connection reset"))
	account.Proxy.Name = "node B"
	require.Equal(t, 0, result.StatusCode)
	require.False(t, result.Success)
	require.Equal(t, "node A", result.ProxyName)
	require.Equal(t, int64(3), result.ProxyID)
	result = BuildRecentRequestRecord(account, "gpt-5", false, 0, &UpstreamFailoverError{
		StatusCode: 400, ResponseBody: []byte(`{"error":{"message":"level minimal not supported"}}`),
	})
	require.Equal(t, 400, result.StatusCode)
	require.Equal(t, "level minimal not supported", result.ErrorMessage)
}

func TestRecentRequestErrorMessageRedactsCredentialsAndBoundsUnicode(t *testing.T) {
	raw := `failed Authorization: Bearer secret-oauth-token; api_key=secret-key; https://user:password@example.com/path?token=abc; sk-abcdefghijklm`
	message := RecentRequestErrorMessage(raw)
	for _, secret := range []string{"secret-oauth-token", "secret-key", "user:password", "token=abc", "sk-abcdefghijklm"} {
		require.NotContains(t, message, secret)
	}
	require.NotContains(t, RecentRequestErrorMessage(`{"request":{"authorization":"secret"}}`), "secret")
	require.Len(t, []rune(RecentRequestErrorMessage(strings.Repeat("错", 1200))), 1001)
}

func TestRecentRequestErrorMessageRedactsCompleteSecretHeaders(t *testing.T) {
	for _, header := range []string{
		"Cookie: session=secret_a; csrf=secret_b",
		"Set-Cookie: session=secret_a; custom=secret_b; Path=/",
		"Authorization: Custom secret_a secret_b",
		"Proxy-Authorization: Custom secret_a secret_b",
	} {
		t.Run(strings.SplitN(header, ":", 2)[0], func(t *testing.T) {
			message := RecentRequestErrorMessage("upstream rejected headers\n" + header + "\r\nrequest failed")
			require.NotContains(t, message, "secret_a")
			require.NotContains(t, message, "secret_b")
			require.Contains(t, message, "upstream rejected headers")
			require.Contains(t, message, "request failed")
		})
	}
}

type recentRequestCaptureStore struct{ records chan RecentRequestRecord }

func (s *recentRequestCaptureStore) RecordRecentRequest(ctx context.Context, _ int64, record RecentRequestRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.records <- record
	return nil
}
func (s *recentRequestCaptureStore) GetRecentRequests(context.Context, []int64, int) (map[int64][]RecentRequestRecord, error) {
	return nil, nil
}

func TestRecordAccountRecentRequestsSurvivesCancellationAndCopiesBatch(t *testing.T) {
	store := &recentRequestCaptureStore{records: make(chan RecentRequestRecord, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	base := time.Now()
	records := []RecentRequestRecord{{AccountID: 1, CreatedAt: base.Add(time.Second)}, {AccountID: 2, CreatedAt: base}}
	RecordAccountRecentRequests(ctx, store, records)
	records[0].AccountID = 999
	for _, want := range []int64{2, 1} {
		select {
		case record := <-store.records:
			require.Equal(t, want, record.AccountID)
		case <-time.After(2 * time.Second):
			t.Fatal("recent request was not submitted")
		}
	}
}
