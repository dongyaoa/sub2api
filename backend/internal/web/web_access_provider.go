package web

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// WebAccessProvider evaluates website access only. API handlers never use this gate.
type WebAccessProvider interface {
	GetWebAccessDecision(context.Context, string) (service.WebAccessDecision, error)
}
