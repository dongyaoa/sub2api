package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// appendOpsUpstreamHTTPError captures terminal HTTP responses in branches that
// return early without passing through the usual upstream error handlers.
func appendOpsUpstreamHTTPError(c *gin.Context, account *Account, resp *http.Response, message string) {
	if account == nil || resp == nil {
		return
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:            opsUpstreamProxyID(account),
		ProxyName:          opsUpstreamProxyName(account),
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  resp.Header.Get("x-request-id"),
		Kind:               "http_error",
		Message:            message,
	})
}
