//go:build !windows

package service

// Non-Windows hosts retain the existing process-environment proxy behavior.
func readWebAccessSystemProxy() (webAccessSystemProxySettings, error) {
	return webAccessSystemProxySettings{}, nil
}
