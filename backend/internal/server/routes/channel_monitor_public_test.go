package routes

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func TestRegisterPublicChannelMonitorRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterPublicChannelMonitorRoutes(v1, &handler.Handlers{
		ChannelMonitor: &handler.ChannelMonitorUserHandler{},
	}, nil)

	want := map[string]bool{
		"GET /api/v1/public/channel-monitors":            false,
		"GET /api/v1/public/channel-monitors/:id/status": false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, registered := range want {
		if !registered {
			t.Fatalf("public channel monitor route not registered: %s", route)
		}
	}
}
