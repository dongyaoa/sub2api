package service

// The Composer image-description preflight returns text and token usage rather
// than a forwarding result. Carry its terminal refusal through that interface
// so the outer request can retain the fixed-fee result without another call.
type grokComposerContentPolicyError struct {
	result *OpenAIForwardResult
	err    error
}

func (e *grokComposerContentPolicyError) Error() string { return e.err.Error() }
func (e *grokComposerContentPolicyError) Unwrap() error { return e.err }
