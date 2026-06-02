package codeindex

import "sort"

func Extract(path string, language string, source []byte) Result {
	if !IsSupported(language) {
		return Result{}
	}
	result := extractWithTreeSitter(path, language, source)
	SortItems(result.Items)
	sort.Strings(result.Warnings)
	return result
}

func SortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Parent != items[j].Parent {
			return items[i].Parent < items[j].Parent
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ImportPath < items[j].ImportPath
	})
}
