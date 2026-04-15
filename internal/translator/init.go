// Package translator adapts the CLIProxyAPI translator SDK for AMP Manager.
package translator

import sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"

// Format constants used inside AMP Manager.
const (
	FormatOpenAI          Format = "openai"
	FormatOpenAIChat      Format = "openai-chat"
	FormatOpenAIResponses Format = "openai-responses"
	FormatClaude          Format = "claude"
	FormatGemini          Format = "gemini"
)

func canonicalSDKFormat(format Format) sdktranslator.Format {
	switch format {
	case FormatOpenAI, FormatOpenAIChat:
		return sdktranslator.FormatOpenAI
	case FormatOpenAIResponses:
		return sdktranslator.FormatOpenAIResponse
	case FormatClaude:
		return sdktranslator.FormatClaude
	case FormatGemini:
		return sdktranslator.FormatGemini
	default:
		return sdktranslator.Format(format)
	}
}

func fromSDKFormat(format sdktranslator.Format) Format {
	switch format {
	case sdktranslator.FormatOpenAI:
		return FormatOpenAIChat
	case sdktranslator.FormatOpenAIResponse:
		return FormatOpenAIResponses
	case sdktranslator.FormatClaude:
		return FormatClaude
	case sdktranslator.FormatGemini:
		return FormatGemini
	default:
		return Format(format)
	}
}
