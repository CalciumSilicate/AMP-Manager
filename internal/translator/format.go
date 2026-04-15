package translator

// Format identifies a request/response schema used inside the proxy.
type Format string

// FromString converts an arbitrary identifier to a translator format.
func FromString(v string) Format {
	return Format(v)
}

// String returns the raw schema identifier.
func (f Format) String() string {
	return string(f)
}

// Platform returns the platform family of the format.
func (f Format) Platform() string {
	switch canonicalSDKFormat(f) {
	case canonicalSDKFormat(FormatOpenAIChat), canonicalSDKFormat(FormatOpenAIResponses):
		return "openai"
	case canonicalSDKFormat(FormatClaude):
		return "claude"
	case canonicalSDKFormat(FormatGemini):
		return "gemini"
	default:
		return string(f)
	}
}

// IsSamePlatform checks if two formats belong to the same platform.
func IsSamePlatform(from, to Format) bool {
	return from.Platform() == to.Platform()
}

// Equivalent reports whether two formats are identical after SDK normalization.
func Equivalent(from, to Format) bool {
	return canonicalSDKFormat(from) == canonicalSDKFormat(to)
}

// SupportsTranslation reports whether the SDK can translate a request/response pair.
func SupportsTranslation(from, to Format) bool {
	if Equivalent(from, to) {
		return true
	}
	return HasResponseTransformer(from, to)
}
