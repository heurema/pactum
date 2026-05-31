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
