package pipeline

// Tool is one check step: a binary plus base arguments. Args may contain
// the literal placeholder "{files}", which is replaced by the file list
// when the step runs; when the placeholder is absent, the files are
// appended to the args instead.
//
// Bin is the executable looked up on PATH to decide whether the step can
// run at all; a missing tool means the step is skipped with a warning,
// never a blocked push.
type Tool struct {
	Name      string   // display name shown in the report ("golangci-lint")
	Bin       string   // executable to check availability for
	Args      []string // base args, may contain the "{files}" placeholder
	FailOnOut bool     // true when a non-zero exit is NOT the failure signal:
	// gofmt and shfmt -d exit 0 while still reporting findings, so a
	// non-empty output is what makes the step fail for them.
	PerDirectory bool // true when the tool can only check files from a single
	// directory in one call. golangci-lint's "named files" mode type-checks
	// one package, so a call naming files from two directories aborts before
	// linting anything; such tools get the pushed files grouped by directory
	// and are run once per directory instead of once with the flat list.
}

// Checks holds the configured formatter + linter for one language.
type Checks struct {
	Language  Language
	Formatter Tool
	Linter    Tool
}

// DefaultChecks is the built-in tool mapping. Any command can be replaced
// through config: pushchecks.tools.<lang>.format / .lint in gitai.json.
//
// The linters run on the pushed files only (not the whole repo) so a push
// is judged on what it actually ships.
var DefaultChecks = []Checks{
	{
		Language:  LangGo,
		Formatter: Tool{Name: "gofmt", Bin: "gofmt", Args: []string{"-l", "{files}"}, FailOnOut: true},
		Linter:    Tool{Name: "golangci-lint", Bin: "golangci-lint", Args: []string{"run", "{files}"}, PerDirectory: true},
	},
	{
		Language:  LangPython,
		Formatter: Tool{Name: "black", Bin: "black", Args: []string{"--check", "--quiet", "{files}"}},
		Linter:    Tool{Name: "ruff", Bin: "ruff", Args: []string{"check", "{files}"}},
	},
	{
		Language:  LangNode,
		Formatter: Tool{Name: "prettier", Bin: "prettier", Args: []string{"--check", "{files}"}},
		Linter:    Tool{Name: "eslint", Bin: "eslint", Args: []string{"{files}"}},
	},
	{
		Language:  LangShell,
		Formatter: Tool{Name: "shfmt", Bin: "shfmt", Args: []string{"-d", "{files}"}, FailOnOut: true},
		Linter:    Tool{Name: "shellcheck", Bin: "shellcheck", Args: []string{"{files}"}},
	},
	{
		Language:  LangHTML,
		Formatter: Tool{Name: "prettier", Bin: "prettier", Args: []string{"--check", "{files}"}},
		Linter:    Tool{Name: "htmlhint", Bin: "htmlhint", Args: []string{"{files}"}},
	},
	{
		Language:  LangYAML,
		Formatter: Tool{Name: "prettier", Bin: "prettier", Args: []string{"--check", "{files}"}},
		Linter:    Tool{Name: "yamllint", Bin: "yamllint", Args: []string{"{files}"}},
	},
}

// checksFor returns the configured checks for lang.
func checksFor(lang Language) (Checks, bool) {
	for _, c := range DefaultChecks {
		if c.Language == lang {
			return c, true
		}
	}
	return Checks{}, false
}

// langExts lists the file extensions for each language, used to attribute
// lint findings back to the files they come from.
var langExts = map[Language][]string{
	LangGo:     {".go"},
	LangPython: {".py"},
	LangNode:   {".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs"},
	LangShell:  {".sh", ".bash"},
	LangHTML:   {".html", ".htm"},
	LangYAML:   {".yml", ".yaml"},
}
