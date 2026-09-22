package catalog

import (
	"errors"
	"testing"
	"testing/fstest"
)

func TestVersionedSelections(t *testing.T) {
	files := fstest.MapFS{
		"modules/tool/module.toml":       {Data: []byte("description = 'Tool'\ndefault = '26'\nversions = ['1.24', '26']\n")},
		"modules/tool/default.nix":       {},
		"modules/tool/versions/1.24.nix": {},
		"modules/tool/versions/26.nix":   {},
	}
	index, err := readModules(files)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &Catalog{files: files, modules: index}
	for selector, entry := range map[string]string{
		"tool":      "default.nix",
		"tool-1.24": "versions/1.24.nix",
		"tool-26":   "versions/26.nix",
	} {
		selected, err := catalog.Module(selector)
		if err != nil || selected.EntryPoint != entry {
			t.Fatalf("%s: entry %q, error %v", selector, selected.EntryPoint, err)
		}
	}
	if _, err := catalog.Module("tool-1.99"); !errors.Is(err, ErrModule) {
		t.Fatalf("unknown version accepted: %v", err)
	}
}

func TestDuplicateSelectorsRejected(t *testing.T) {
	files := fstest.MapFS{
		"modules/tool/module.toml":     {Data: []byte("description = 'Tool'\ndefault = '26'\nversions = ['26']\n")},
		"modules/tool/default.nix":     {},
		"modules/tool/versions/26.nix": {},
		"modules/tool-26/module.toml":  {Data: []byte("description = 'Another module'\n")},
		"modules/tool-26/default.nix":  {},
	}
	if _, err := readModules(files); !errors.Is(err, ErrMetadata) {
		t.Fatalf("duplicate selector accepted: %v", err)
	}
}

func TestInvalidVersionMetadata(t *testing.T) {
	for name, declaration := range map[string]string{
		"missing default":          "versions = ['1.24']",
		"unknown default":          "default = '1.27'\nversions = ['1.24']",
		"default without versions": "default = '1.24'",
		"duplicate":                "default = '1.24'\nversions = ['1.24', '1.24']",
		"unsafe":                   "default = '../escape'\nversions = ['../escape']",
		"missing entry":            "default = '1.24'\nversions = ['1.24']",
		"unknown field":            "defaults = '1.24'",
	} {
		t.Run(name, func(t *testing.T) {
			files := fstest.MapFS{
				"module.toml": {Data: []byte("description = 'Tool'\n" + declaration)},
				"default.nix": {},
			}
			if _, err := readMetadata(files, "."); !errors.Is(err, ErrMetadata) {
				t.Fatalf("invalid metadata accepted: %v", err)
			}
		})
	}
}
