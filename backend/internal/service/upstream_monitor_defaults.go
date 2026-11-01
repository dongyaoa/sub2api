package service

const defaultUpstreamMonitorIntervalSeconds = 60

func defaultUpstreamMonitorModel(provider string) string {
	switch provider {
	case MonitorProviderAnthropic:
		return "claude-sonnet-5"
	case MonitorProviderGemini:
		return "gemini-2.5-flash"
	default:
		return "gpt-6.1-sol"
	}
}
