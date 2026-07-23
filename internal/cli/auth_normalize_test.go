package cli

import "testing"

func TestNormalizeAuth(t *testing.T) {
	cases := map[string]string{
		"openai":        "openai",
		"bearer":        "openai",
		"Bearer":        "openai",
		"anthropic":     "anthropic",
		"anthropic-key": "anthropic",
		"ollama":        "ollama",
		"none":          "ollama",
		"":              "openai",
		"custom":        "custom",
	}
	for in, want := range cases {
		if got := normalizeAuth(in); got != want {
			t.Errorf("normalizeAuth(%q)=%q want %q", in, got, want)
		}
	}
}

func TestAuthStyleRoundTrip(t *testing.T) {
	pairs := []struct{ style, typ string }{
		{"bearer", "openai"},
		{"anthropic-key", "anthropic"},
		{"none", "ollama"},
	}
	for _, p := range pairs {
		if got := authStyleToType(p.style); got != p.typ {
			t.Errorf("authStyleToType(%q)=%q want %q", p.style, got, p.typ)
		}
		if got := typeToAuthStyle(p.typ); got != p.style {
			t.Errorf("typeToAuthStyle(%q)=%q want %q", p.typ, got, p.style)
		}
	}
}
