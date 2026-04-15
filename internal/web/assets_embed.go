//go:build embed_frontend

package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedAssets embed.FS

func loadAssetSource() assetSource {
	distFS, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		return assetSource{}
	}

	source := assetSource{
		dist:  distFS,
		label: "embedded",
	}
	if _, err := fs.Stat(distFS, "index.html"); err == nil {
		source.indexFound = true
	}
	return source
}
