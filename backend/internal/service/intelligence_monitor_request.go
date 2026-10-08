package service

// Only local artwork can use an administrator-configured prompt captured when
// the run was queued. Candy and non-local tests retain their fixed definitions.
func intelligenceTestRequestDefinition(run *IntelligenceMonitorRun) (prompt string, limit int, valid bool) {
	if run == nil {
		return
	}
	switch run.TestKind {
	case "", IntelligenceMonitorTestPelican:
		if run.SourceType == "local_group" {
			prompt, valid := intelligenceLocalArtworkPrompt(run.Prompt)
			return prompt, 24000, valid
		}
		return IntelligenceMonitorPrompt, 24000, true
	case IntelligenceMonitorTestCandy:
		return IntelligenceMonitorCandyPrompt, IntelligenceMonitorCandyMaxOutputTokens, true
	default:
		return
	}
}
