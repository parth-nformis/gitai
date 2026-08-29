package pipeline

import (
	"reflect"
	"testing"
)

func TestDetectLanguages(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []Language
	}{
		{"empty", nil, nil},
		{"single go file", []string{"main.go"}, []Language{LangGo}},
		{"stable order", []string{"b.py", "a.go", "c.sh", "d.js"},
			[]Language{LangGo, LangPython, LangNode, LangShell}},
		{"dedupe", []string{"a.go", "b.go", "c.go"}, []Language{LangGo}},
		{"unknown extensions ignored", []string{"README.md", "Makefile", "x.rs"}, nil},
		{"mixed with unknowns", []string{"README.md", "a.go", "b.yml"},
			[]Language{LangGo, LangYAML}},
		{"case-insensitive extensions", []string{"A.GO", "b.Py"},
			[]Language{LangGo, LangPython}},
		{"directory paths", []string{"src/deep/main.go", "scripts/run.sh"},
			[]Language{LangGo, LangShell}},
		{"all node flavors", []string{"a.js", "b.jsx", "c.ts", "d.tsx", "e.mjs", "f.cjs"},
			[]Language{LangNode}},
		{"yaml both spellings", []string{"a.yml", "b.yaml"}, []Language{LangYAML}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLanguages(tt.files)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DetectLanguages(%v) = %v, want %v", tt.files, got, tt.want)
			}
		})
	}
}

func TestFilesFor(t *testing.T) {
	files := []string{"b.py", "a.go", "c.js", "README.md", "d.go"}
	got := FilesFor(files, LangGo)
	want := []string{"a.go", "d.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilesFor(go) = %v, want %v (input order kept)", got, want)
	}
	if got := FilesFor(files, LangYAML); len(got) != 0 {
		t.Errorf("FilesFor(yaml) = %v, want empty", got)
	}
}
