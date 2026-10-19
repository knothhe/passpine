package i18n

import (
	"embed"
	"io/fs"

	mygoi18n "github.com/egoist/mygo/i18n"
)

// Onevra resolves its internal labels through MyGo's application catalog.
// Keep these overrides alongside the client catalog so language changes also
// update existing component controls without recreating their state.
//
//go:embed components/*.json
var componentMessages embed.FS

func init() {
	files, err := fs.Sub(componentMessages, "components")
	if err != nil {
		panic(err)
	}
	catalog, err := mygoi18n.NewCatalog(files, English)
	if err != nil {
		panic(err)
	}
	mygoi18n.Register(catalog)
}
