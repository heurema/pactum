package codeindex

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func extractWithTreeSitter(path string, language string, source []byte) Result {
	grammar := grammarFor(language)
	if grammar == nil {
		return Result{}
	}

	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(grammar); err != nil {
		return Result{Warnings: []string{fmt.Sprintf("code index parser unavailable for %s: %v", path, err)}}
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		return Result{Warnings: []string{"code index parse failed: " + path}}
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil || root.HasError() {
		return Result{Warnings: []string{"code index parse warning: " + path + " contains syntax errors"}}
	}

	ctx := sourceContext{path: path, language: language, source: source}
	switch language {
	case "go":
		return Result{Items: extractGo(ctx, root)}
	case "python":
		return Result{Items: extractPython(ctx, root)}
	case "javascript", "jsx":
		return Result{Items: extractJavaScript(ctx, root, language)}
	case "typescript", "tsx":
		return Result{Items: extractTypeScript(ctx, root, language)}
	case "csharp":
		return Result{Items: extractCSharp(ctx, root)}
	default:
		return Result{}
	}
}

func grammarFor(language string) *sitter.Language {
	switch language {
	case "go":
		return sitter.NewLanguage(tree_sitter_go.Language())
	case "python":
		return sitter.NewLanguage(tree_sitter_python.Language())
	case "javascript", "jsx":
		return sitter.NewLanguage(tree_sitter_javascript.Language())
	case "typescript":
		return sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	case "tsx":
		return sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	case "csharp":
		return sitter.NewLanguage(tree_sitter_c_sharp.Language())
	default:
		return nil
	}
}

type sourceContext struct {
	path     string
	language string
	source   []byte
}

func (c sourceContext) text(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	start := int(node.StartByte())
	end := int(node.EndByte())
	if start < 0 || end < start || end > len(c.source) {
		return ""
	}
	return string(c.source[start:end])
}

func (c sourceContext) item(kind string, node *sitter.Node, name string) Item {
	startLine, endLine := lineRange(node)
	return Item{
		Path:      c.path,
		Kind:      kind,
		Language:  c.language,
		Name:      name,
		Signature: signature(c.text(node)),
		StartLine: startLine,
		EndLine:   endLine,
	}
}

func extractGo(ctx sourceContext, root *sitter.Node) []Item {
	var items []Item
	packageName := ""

	for _, child := range namedChildren(root) {
		if child.Kind() != "package_clause" {
			continue
		}
		packageName = firstNamedText(ctx, child, "package_identifier", "identifier")
		if packageName != "" {
			item := ctx.item("go_package", child, packageName)
			item.Package = packageName
			items = append(items, item)
		}
	}

	for _, child := range namedChildren(root) {
		switch child.Kind() {
		case "import_declaration":
			for _, importNode := range descendants(child, "interpreted_string_literal", "raw_string_literal") {
				importPath := unquote(ctx.text(importNode))
				if importPath == "" {
					continue
				}
				item := ctx.item("go_import", importNode, importPath)
				item.Package = packageName
				item.ImportPath = importPath
				items = append(items, item)
			}
		case "function_declaration":
			name := nodeName(ctx, child)
			if packageName == "main" && name == "main" {
				item := ctx.item("go_main", child, "main")
				item.Package = packageName
				items = append(items, item)
				continue
			}
			if !isExportedGo(name) {
				continue
			}
			item := ctx.item("go_func", child, name)
			item.Package = packageName
			item.Exported = true
			items = append(items, item)
		case "type_declaration":
			for _, typeSpec := range descendants(child, "type_spec") {
				name := nodeName(ctx, typeSpec)
				if !isExportedGo(name) {
					continue
				}
				item := ctx.item("go_type", typeSpec, name)
				item.Package = packageName
				item.Exported = true
				items = append(items, item)
			}
		case "method_declaration":
			name := nodeName(ctx, child)
			parent := goReceiverName(ctx, child.ChildByFieldName("receiver"))
			if !isExportedGo(name) || !isExportedGo(parent) {
				continue
			}
			item := ctx.item("go_method", child, name)
			item.Package = packageName
			item.Parent = parent
			item.Exported = true
			items = append(items, item)
		}
	}

	return items
}

func extractPython(ctx sourceContext, root *sitter.Node) []Item {
	module := strings.TrimSuffix(filepath.Base(ctx.path), filepath.Ext(ctx.path))
	items := []Item{func() Item {
		item := ctx.item("py_module", root, module)
		item.Package = module
		return item
	}()}

	for _, child := range namedChildren(root) {
		switch child.Kind() {
		case "import_statement", "import_from_statement", "future_import_statement":
			name := compact(ctx.text(child))
			item := ctx.item("py_import", child, name)
			item.Package = module
			item.ImportPath = name
			items = append(items, item)
		case "function_definition":
			name := nodeName(ctx, child)
			if isPrivatePython(name) {
				continue
			}
			item := ctx.item("py_func", child, name)
			item.Package = module
			item.Exported = true
			items = append(items, item)
		case "class_definition":
			className := nodeName(ctx, child)
			if isPrivatePython(className) {
				continue
			}
			item := ctx.item("py_class", child, className)
			item.Package = module
			item.Exported = true
			items = append(items, item)
			for _, method := range descendants(child, "function_definition") {
				methodName := nodeName(ctx, method)
				if isPrivatePython(methodName) {
					continue
				}
				methodItem := ctx.item("py_method", method, methodName)
				methodItem.Package = module
				methodItem.Parent = className
				methodItem.Exported = true
				items = append(items, methodItem)
			}
		case "if_statement":
			text := ctx.text(child)
			if strings.Contains(text, "__name__") && (strings.Contains(text, `"__main__"`) || strings.Contains(text, "'__main__'")) {
				item := ctx.item("py_main", child, "__main__")
				item.Package = module
				items = append(items, item)
			}
		}
	}

	return items
}

func extractJavaScript(ctx sourceContext, root *sitter.Node, language string) []Item {
	ctx.language = language
	prefix := "js"
	return extractJSLike(ctx, root, prefix)
}

func extractTypeScript(ctx sourceContext, root *sitter.Node, language string) []Item {
	ctx.language = language
	return extractJSLike(ctx, root, "ts")
}

func extractJSLike(ctx sourceContext, root *sitter.Node, prefix string) []Item {
	var items []Item
	for _, child := range namedChildren(root) {
		switch child.Kind() {
		case "import_statement":
			name := importPathFromNode(ctx, child)
			if name == "" {
				name = compact(ctx.text(child))
			}
			item := ctx.item(prefix+"_import", child, name)
			item.ImportPath = name
			items = append(items, item)
		case "export_statement":
			before := len(items)
			items = append(items, exportedJSItems(ctx, child, prefix)...)
			if len(items) == before {
				item := ctx.item(prefix+"_export", child, compact(ctx.text(child)))
				items = append(items, item)
			}
		}
	}
	return items
}

func exportedJSItems(ctx sourceContext, exportNode *sitter.Node, prefix string) []Item {
	var items []Item
	for _, node := range descendants(exportNode,
		"function_declaration",
		"generator_function_declaration",
		"class_declaration",
		"abstract_class_declaration",
		"interface_declaration",
		"type_alias_declaration",
		"enum_declaration",
		"variable_declarator",
	) {
		name := nodeName(ctx, node)
		if name == "" {
			continue
		}
		switch node.Kind() {
		case "function_declaration", "generator_function_declaration":
			item := ctx.item(prefix+"_func", node, name)
			item.Exported = true
			items = append(items, item)
		case "class_declaration", "abstract_class_declaration":
			item := ctx.item(prefix+"_class", node, name)
			item.Exported = true
			items = append(items, item)
			items = append(items, jsMethodItems(ctx, node, prefix, name)...)
		case "interface_declaration":
			if prefix == "ts" {
				item := ctx.item("ts_interface", node, name)
				item.Exported = true
				items = append(items, item)
			}
		case "type_alias_declaration":
			if prefix == "ts" {
				item := ctx.item("ts_type", node, name)
				item.Exported = true
				items = append(items, item)
			}
		case "enum_declaration":
			if prefix == "ts" {
				item := ctx.item("ts_enum", node, name)
				item.Exported = true
				items = append(items, item)
			}
		case "variable_declarator":
			if hasDescendant(node, "arrow_function") || hasDescendant(node, "function") {
				item := ctx.item(prefix+"_func", node, name)
				item.Exported = true
				items = append(items, item)
			}
		}
	}
	return dedupeItems(items)
}

func jsMethodItems(ctx sourceContext, classNode *sitter.Node, prefix string, parent string) []Item {
	var items []Item
	for _, method := range descendants(classNode, "method_definition") {
		name := nodeName(ctx, method)
		if name == "" || name == "constructor" || strings.HasPrefix(name, "#") {
			continue
		}
		item := ctx.item(prefix+"_method", method, name)
		item.Parent = parent
		item.Exported = true
		items = append(items, item)
	}
	return items
}

func extractCSharp(ctx sourceContext, root *sitter.Node) []Item {
	var items []Item
	var walk func(node *sitter.Node, namespace string, parentType string, parentPublic bool)

	walk = func(node *sitter.Node, namespace string, parentType string, parentPublic bool) {
		switch node.Kind() {
		case "using_directive":
			name := csharpUsingName(ctx, node)
			item := ctx.item("cs_using", node, name)
			item.ImportPath = name
			item.Package = namespace
			items = append(items, item)
		case "namespace_declaration", "file_scoped_namespace_declaration":
			namespace = csharpName(ctx, node)
			if namespace != "" {
				item := ctx.item("cs_namespace", node, namespace)
				item.Package = namespace
				items = append(items, item)
			}
		case "class_declaration", "interface_declaration", "record_declaration", "struct_declaration", "enum_declaration":
			name := nodeName(ctx, node)
			public := csharpHasPublicModifier(ctx, node)
			if public && name != "" {
				item := ctx.item(csharpKind(node.Kind()), node, name)
				item.Package = namespace
				item.Exported = true
				items = append(items, item)
			}
			parentType = name
			parentPublic = public
		case "method_declaration":
			name := nodeName(ctx, node)
			if parentPublic && csharpHasPublicModifier(ctx, node) && name != "" {
				item := ctx.item("cs_method", node, name)
				item.Package = namespace
				item.Parent = parentType
				item.Exported = true
				items = append(items, item)
			}
		}

		for _, child := range namedChildren(node) {
			walk(child, namespace, parentType, parentPublic)
		}
	}

	walk(root, "", "", false)
	return dedupeItems(items)
}

func csharpKind(kind string) string {
	switch kind {
	case "class_declaration":
		return "cs_class"
	case "interface_declaration":
		return "cs_interface"
	case "record_declaration":
		return "cs_record"
	case "struct_declaration":
		return "cs_struct"
	case "enum_declaration":
		return "cs_enum"
	default:
		return "cs_item"
	}
}

func namedChildren(node *sitter.Node) []*sitter.Node {
	children := make([]*sitter.Node, 0, node.NamedChildCount())
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child != nil {
			children = append(children, child)
		}
	}
	return children
}

func descendants(node *sitter.Node, kinds ...string) []*sitter.Node {
	wanted := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		wanted[kind] = struct{}{}
	}
	var result []*sitter.Node
	var walk func(*sitter.Node)
	walk = func(current *sitter.Node) {
		if current == nil {
			return
		}
		if _, ok := wanted[current.Kind()]; ok {
			result = append(result, current)
		}
		for _, child := range namedChildren(current) {
			walk(child)
		}
	}
	walk(node)
	return result
}

func hasDescendant(node *sitter.Node, kind string) bool {
	return len(descendants(node, kind)) > 0
}

func nodeName(ctx sourceContext, node *sitter.Node) string {
	if node == nil {
		return ""
	}
	if name := ctx.text(node.ChildByFieldName("name")); name != "" {
		return cleanName(name)
	}
	return firstNamedText(ctx, node, "identifier", "field_identifier", "property_identifier", "type_identifier", "package_identifier")
}

func firstNamedText(ctx sourceContext, node *sitter.Node, kinds ...string) string {
	for _, child := range descendants(node, kinds...) {
		text := cleanName(ctx.text(child))
		if text != "" {
			return text
		}
	}
	return ""
}

func importPathFromNode(ctx sourceContext, node *sitter.Node) string {
	for _, child := range descendants(node, "string", "string_fragment", "interpreted_string_literal", "raw_string_literal") {
		value := unquote(ctx.text(child))
		if value != "" {
			return value
		}
	}
	return ""
}

func csharpUsingName(ctx sourceContext, node *sitter.Node) string {
	text := strings.TrimSpace(ctx.text(node))
	text = strings.TrimPrefix(text, "using")
	text = strings.TrimSuffix(text, ";")
	return strings.TrimSpace(text)
}

func csharpName(ctx sourceContext, node *sitter.Node) string {
	text := strings.TrimSpace(ctx.text(node))
	text = strings.TrimPrefix(text, "namespace")
	text = strings.TrimSpace(text)
	for _, marker := range []string{";", "{"} {
		if idx := strings.Index(text, marker); idx >= 0 {
			text = text[:idx]
			break
		}
	}
	if text = strings.TrimSpace(text); text != "" {
		return text
	}
	return nodeName(ctx, node)
}

func csharpHasPublicModifier(ctx sourceContext, node *sitter.Node) bool {
	name := nodeName(ctx, node)
	text := ctx.text(node)
	if name != "" {
		if idx := strings.Index(text, name); idx >= 0 {
			text = text[:idx]
		}
	}
	return strings.Contains(" "+text+" ", " public ")
}

func goReceiverName(ctx sourceContext, receiver *sitter.Node) string {
	text := ctx.text(receiver)
	text = strings.NewReplacer("(", " ", ")", " ", "*", " ", "[", " ", "]", " ").Replace(text)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return cleanName(fields[len(fields)-1])
}

func isExportedGo(name string) bool {
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

func isPrivatePython(name string) bool {
	return name == "" || strings.HasPrefix(name, "_")
}

func lineRange(node *sitter.Node) (int, int) {
	if node == nil {
		return 0, 0
	}
	return int(node.StartPosition().Row) + 1, int(node.EndPosition().Row) + 1
}

func signature(text string) string {
	text = compact(text)
	for _, marker := range []string{" {", "{", ";"} {
		if idx := strings.Index(text, marker); idx >= 0 {
			text = strings.TrimSpace(text[:idx])
			break
		}
	}
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

func compact(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, `"'`)
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "`'\"")
	return name
}

func dedupeItems(items []Item) []Item {
	seen := make(map[string]struct{}, len(items))
	result := make([]Item, 0, len(items))
	for _, item := range items {
		key := item.Path + "\x00" + item.Kind + "\x00" + item.Parent + "\x00" + item.Name + "\x00" + item.ImportPath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}
