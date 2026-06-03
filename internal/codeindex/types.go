package codeindex

type Item struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Language   string `json:"language"`
	Name       string `json:"name"`
	Package    string `json:"package,omitempty"`
	Parent     string `json:"parent,omitempty"`
	Exported   bool   `json:"exported,omitempty"`
	ImportPath string `json:"import_path,omitempty"`
	Signature  string `json:"signature,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
}

type Result struct {
	Items    []Item
	Warnings []string
}

// importLikeKinds are item kinds that carry import/module/namespace wiring
// rather than a definition worth surfacing in the code map.
var importLikeKinds = map[string]bool{
	"go_package":   true,
	"go_import":    true,
	"py_module":    true,
	"py_import":    true,
	"js_import":    true,
	"ts_import":    true,
	"cs_using":     true,
	"cs_namespace": true,
}

// entryPointKinds mark program entry points worth surfacing first.
var entryPointKinds = map[string]bool{
	"go_main": true,
	"py_main": true,
}

// IsImportLike reports whether the item is an import/module/namespace marker
// rather than a definition to surface in the code map.
func (i Item) IsImportLike() bool { return importLikeKinds[i.Kind] }

// IsEntryPoint reports whether the item marks a program entry point.
func (i Item) IsEntryPoint() bool { return entryPointKinds[i.Kind] }

const (
	ModeAuto     = "auto"
	ModeOff      = "off"
	ModeRequired = "required"
)

func SupportedLanguages() []string {
	return []string{"go", "python", "javascript", "typescript", "tsx", "jsx", "csharp"}
}

func NormalizeMode(mode string) string {
	switch mode {
	case ModeOff, ModeRequired:
		return mode
	default:
		return ModeAuto
	}
}

func Enabled(mode string) bool {
	return NormalizeMode(mode) != ModeOff
}
