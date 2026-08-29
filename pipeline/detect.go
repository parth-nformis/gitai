// Package pipeline runs the pre-push check pipeline: it detects which
// languages the files being pushed contain, then runs that language's
// formatter and linter, and reports per-step results.
//
// This is plain local tooling — no LLM involved — so it sits apart from
// the commands/ package, which holds the AI handlers.
package pipeline

import (
	"path/filepath"
	"strings"
)

// Language is a detected language family with configured checks.
type Language string

const (
	// LangGo — .go files: gofmt + golangci-lint.
	LangGo Language = "go"
	// LangPython — .py files: black + ruff.
	LangPython Language = "python"
	// LangNode — .js/.ts family: prettier + eslint.
	LangNode Language = "node"
	// LangShell — .sh/.bash files: shfmt + shellcheck.
	LangShell Language = "shell"
	// LangHTML — .html files: prettier + htmlhint.
	LangHTML Language = "html"
	// LangYAML — .yml/.yaml files: prettier + yamllint.
	LangYAML Language = "yaml"
)

// languages is the fixed check order; it also defines the stable order
// DetectLanguages reports results in, so the terminal readout is
// consistent from push to push.
var languages = []Language{LangGo, LangPython, LangNode, LangShell, LangHTML, LangYAML}

// AllLanguages returns the checked languages in their fixed order.
func AllLanguages() []Language {
	out := make([]Language, len(languages))
	copy(out, languages)
	return out
}

var extToLang = map[string]Language{
	".go": LangGo,
	".py": LangPython,
	".js": LangNode, ".jsx": LangNode, ".ts": LangNode, ".tsx": LangNode,
	".mjs": LangNode, ".cjs": LangNode,
	".sh": LangShell, ".bash": LangShell,
	".html": LangHTML, ".htm": LangHTML,
	".yml": LangYAML, ".yaml": LangYAML,
}

// DetectLanguages returns the distinct languages present in files, in the
// fixed registry order. Files with unrecognized extensions are ignored —
// unknown is not an error, it just means no checks are configured.
func DetectLanguages(files []string) []Language {
	seen := map[Language]bool{}
	for _, f := range files {
		if lang, ok := extToLang[strings.ToLower(filepath.Ext(f))]; ok {
			seen[lang] = true
		}
	}
	var out []Language
	for _, l := range languages {
		if seen[l] {
			out = append(out, l)
		}
	}
	return out
}

// FilesFor returns the subset of files belonging to lang, keeping the
// input order so reports line up with the diff order git produced.
func FilesFor(files []string, lang Language) []string {
	var out []string
	for _, f := range files {
		if l, ok := extToLang[strings.ToLower(filepath.Ext(f))]; ok && l == lang {
			out = append(out, f)
		}
	}
	return out
}
