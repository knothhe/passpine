package cli

import (
	"io"
	"sort"
	"strings"
)

type treeNode struct {
	children map[string]*treeNode
}

// writeTree renders entry paths like pass's `tree -l --noreport` output,
// without the .gpg suffix. Paths are relative to the listed folder.
func writeTree(w io.Writer, title string, paths []string) {
	root := &treeNode{children: map[string]*treeNode{}}
	for _, p := range paths {
		node := root
		for _, part := range strings.Split(p, "/") {
			child, ok := node.children[part]
			if !ok {
				child = &treeNode{children: map[string]*treeNode{}}
				node.children[part] = child
			}
			node = child
		}
	}
	io.WriteString(w, title+"\n")
	writeChildren(w, root, "")
}

func writeChildren(w io.Writer, node *treeNode, prefix string) {
	names := make([]string, 0, len(node.children))
	for name := range node.children {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		left, right := strings.ToLower(names[i]), strings.ToLower(names[j])
		if left == right {
			return names[i] < names[j]
		}
		return left < right
	})
	for i, name := range names {
		branch, next := "├── ", "│   "
		if i == len(names)-1 {
			branch, next = "└── ", "    "
		}
		io.WriteString(w, prefix+branch+name+"\n")
		writeChildren(w, node.children[name], prefix+next)
	}
}

// matchingPaths keeps entries with any path component containing a term,
// matching `tree -P '*term*' --matchdirs --ignore-case` used by pass find.
func matchingPaths(paths, terms []string) []string {
	out := []string{}
	for _, p := range paths {
		lower := strings.ToLower(p)
		for _, term := range terms {
			term = strings.ToLower(term)
			if term != "" && strings.Contains(lower, term) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
