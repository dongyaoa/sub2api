package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type antigravityRecentErrorReader struct{}

func (antigravityRecentErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestAntigravityRecentRequestCapturesSSEFailuresWithoutChangingResult(t *testing.T) {
	for _, tt := range []struct {
		name      string
		body      io.Reader
		wantError string
	}{
		{"completed", strings.NewReader("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"), ""},
		{"in band error", strings.NewReader("data: {\"type\":\"error\",\"error\":{\"message\":\"level minimal not supported\"}}\n\n"), "level minimal not supported"},
		{"incomplete EOF", strings.NewReader("data: {\"type\":\"message_start\"}\n\n"), "before a completion event"},
		{"read error", antigravityRecentErrorReader{}, "unexpected EOF"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			observation := &recentRequestObservation{}
			c.Set(recentRequestObserverKey, observation)
			beginUpstreamResponseModelObservation(c)
			s := &AntigravityGatewayService{settingService: &SettingService{}}
			result := s.streamUpstreamResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(tt.body)}, time.Now())
			require.NotNil(t, result, "recent-request telemetry must preserve existing usage result")
			if tt.wantError == "" {
				require.Nil(t, observation.failure)
			} else {
				require.NotNil(t, observation.failure)
				require.Equal(t, 200, observation.failure.StatusCode)
				require.Contains(t, observation.failure.ErrorMessage, tt.wantError)
			}
		})
	}
}

func TestAntigravityRecentRequestObservesWrappedGeminiError(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	observation := &recentRequestObservation{}
	c.Set(recentRequestObserverKey, observation)
	s := &AntigravityGatewayService{}
	s.observeAntigravityGeminiSSELine(c, `data: {"response":{"error":{"message":"quota exceeded"}}}`)
	require.NotNil(t, observation.failure)
	require.Equal(t, "quota exceeded", observation.failure.ErrorMessage)
}
