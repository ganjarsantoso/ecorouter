package cli

import "strings"

// normalizeAuth maps user-facing --auth values (and legacy --type aliases)
// to the on-disk ProviderConfig.Type values: openai | anthropic | ollama.
func normalizeAuth(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bearer", "openai":
		return "openai"
	case "anthropic-key", "anthropic":
		return "anthropic"
	case "none", "ollama":
		return "ollama"
	case "":
		return "openai" // default when neither flag is provided AND not interactive
	default:
		return s
	}
}

// authStyleToType maps the user-facing choice to the internal ProviderConfig.Type.
// We keep the internal names ("openai" / "anthropic" / "ollama") for wire
// compatibility with the existing fetchModels() and proxy code.
func authStyleToType(style string) string {
	switch style {
	case "bearer":
		return "openai"
	case "anthropic-key":
		return "anthropic"
	case "none":
		return "ollama"
	default:
		return normalizeAuth(style)
	}
}

// typeToAuthStyle is the inverse, for display and re-editing.
func typeToAuthStyle(t string) string {
	switch t {
	case "openai":
		return "bearer"
	case "anthropic":
		return "anthropic-key"
	case "ollama":
		return "none"
	default:
		return "bearer"
	}
}

// authStyleLabel returns a human-readable label for an internal type.
func authStyleLabel(t string) string {
	switch typeToAuthStyle(t) {
	case "bearer":
		return "bearer"
	case "anthropic-key":
		return "anthropic-key"
	case "none":
		return "none"
	default:
		return t
	}
}
