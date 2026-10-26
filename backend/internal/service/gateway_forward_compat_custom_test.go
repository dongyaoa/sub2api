package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicCompatStreamPreservesMonitorDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.StreamDataIntervalTimeout = 1
	for _, monitor := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "monitor"}[monitor], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if monitor {
				ctx = context.WithValue(ctx, intelligenceGenerationContextKey{}, true)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader(""))}
			defer func() { _ = resp.Body.Close() }()
			stream := newAnthropicCompatStream(cfg, c, resp, "test", "request-test")
			defer stream.stop()
			if monitor {
				require.Greater(t, stream.pump.interval, 30*time.Second)
				require.LessOrEqual(t, stream.pump.interval, time.Minute)
			} else {
				require.Equal(t, time.Second, stream.pump.interval)
			}
		})
	}
}
