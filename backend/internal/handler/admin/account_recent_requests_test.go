package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountRecentRequestsStoreStub struct {
	ids   []int64
	limit int
	err   error
}

func (s *accountRecentRequestsStoreStub) RecordRecentRequest(context.Context, int64, service.RecentRequestRecord) error {
	return nil
}
func (s *accountRecentRequestsStoreStub) GetRecentRequests(_ context.Context, ids []int64, limit int) (map[int64][]service.RecentRequestRecord, error) {
	s.ids, s.limit = ids, limit
	return map[int64][]service.RecentRequestRecord{1: {{AccountID: 1, Success: false, StatusCode: 400, ErrorMessage: "invalid level"}}}, s.err
}

func TestGetBatchRecentRequestsValidatesAndReturnsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &accountRecentRequestsStoreStub{}
	h := &AccountHandler{recentRequestStore: store}
	request := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/accounts/recent-requests/batch", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.GetBatchRecentRequests(c)
		return w
	}
	w := request(`{"account_ids":[1,2],"limit":100}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []int64{1, 2}, store.ids)
	require.Equal(t, 20, store.limit)
	var payload struct {
		Code int                                      `json:"code"`
		Data map[string][]service.RecentRequestRecord `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, 0, payload.Code)
	require.Equal(t, 400, payload.Data["1"][0].StatusCode)
	for _, invalid := range []string{`{}`, `{"account_ids":[]}`, `{"account_ids":[0]}`, `{"account_ids":[-1]}`} {
		require.Equal(t, http.StatusBadRequest, request(invalid).Code)
	}
	store.err = errors.New("redis unavailable")
	require.Equal(t, http.StatusInternalServerError, request(`{"account_ids":[1]}`).Code)
}
