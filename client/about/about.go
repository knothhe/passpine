// Package about embeds the project's direct dependency information.
package about

import (
	_ "embed"
	"encoding/json"
)

const ProjectURL = "https://github.com/knothhe/passpine"

type Dependency struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Module  string `json:"module"`
	Group   string `json:"group"`
	Version string `json:"version"`
	License string `json:"license"`
	URL     string `json:"url"`
	Source  string `json:"source"`
}

//go:embed dependencies.json
var catalog []byte

// Dependencies returns a fresh copy so UI state cannot modify the catalog.
func Dependencies() []Dependency {
	var deps []Dependency
	if err := json.Unmarshal(catalog, &deps); err != nil {
		panic(err)
	}
	return deps
}
