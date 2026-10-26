//go:build embed

package web

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"
	"net/http"
	"time"

	clientip "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

//go:embed web_access.html
var webAccessHTML string

var webAccessTemplate = template.Must(template.New("web-access").Parse(webAccessHTML))

const privateWebAccessCacheKey = "web_access_private_cache"

// allowWebPage runs before the shared HTML cache, including its 304 fast path.
// Country and allowlist decisions must never enter the shared settings/HTML cache.
func allowWebPage(c *gin.Context, provider WebAccessProvider) bool {
	if provider == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	decision, err := provider.GetWebAccessDecision(ctx, clientip.GetTrustedClientIP(c))
	if err != nil {
		serveWebAccessPage(c, http.StatusServiceUnavailable, webAccessPageData{
			Title:     "暫時無法檢查訪問狀態",
			TitleEn:   "Unable to check access",
			Message:   "請檢查網絡連接，稍後重試。",
			MessageEn: "Please check your connection and try again.",
			Label:     "連接暫時不可用",
			LabelEn:   "CONNECTION UNAVAILABLE",
		})
		return false
	}
	if decision.Enabled {
		c.Set(privateWebAccessCacheKey, true)
	}
	if !decision.Blocked {
		return true
	}
	serveWebAccessPage(c, http.StatusForbidden, webAccessPageData{
		Title:     "本站僅面向海外用戶提供服務",
		TitleEn:   "Exclusively for overseas users",
		Message:   decision.Message,
		MessageEn: "Web access is unavailable in mainland China.",
		Label:     "服務區域",
		LabelEn:   "SERVICE AVAILABILITY",
	})
	return false
}

func setFrontendHTMLCacheHeaders(c *gin.Context) {
	if c.GetBool(privateWebAccessCacheKey) {
		c.Header("Cache-Control", "private, no-store")
		return
	}
	c.Header("Cache-Control", "no-cache")
}

type webAccessPageData struct {
	Title, TitleEn, Message, MessageEn, Label, LabelEn, Nonce string
}

func serveWebAccessPage(c *gin.Context, status int, data webAccessPageData) {
	var content bytes.Buffer
	data.Nonce = middleware.GetNonceFromContext(c)
	err := webAccessTemplate.Execute(&content, data)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	if err != nil {
		c.String(status, "%s / %s\n\n%s\n%s\n\n%s\n%s", data.Label, data.LabelEn, data.Title, data.TitleEn, data.Message, data.MessageEn)
	} else {
		c.Data(status, "text/html; charset=utf-8", content.Bytes())
	}
	c.Abort()
}
