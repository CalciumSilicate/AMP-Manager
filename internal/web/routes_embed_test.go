//go:build embed_frontend

package web

import (
	"io/fs"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterStaticRoutesServesEmbeddedAssets(t *testing.T) {
	source := loadAssetSource()
	if source.label != "embedded" {
		t.Fatalf("asset source label = %q, want embedded", source.label)
	}
	if !source.indexFound {
		t.Fatal("expected embedded index.html to exist")
	}

	entries, err := fs.ReadDir(source.dist, "assets")
	if err != nil {
		t.Fatalf("ReadDir assets: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one embedded asset file")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerStaticRoutes(router, source)

	indexResp := performRequest(t, router, "GET", "/dashboard")
	if indexResp.Code != 200 {
		t.Fatalf("expected embedded index status 200, got %d", indexResp.Code)
	}
	if body := indexResp.Body.String(); contains(body, "Frontend assets are missing") {
		t.Fatalf("expected embedded index html, got fallback body %q", body)
	}

	assetResp := performRequest(t, router, "GET", "/assets/"+entries[0].Name())
	if assetResp.Code != 200 {
		t.Fatalf("expected embedded asset status 200, got %d", assetResp.Code)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		getResp := performRequest(t, router, "GET", "/assets/"+entry.Name())
		if getResp.Code != 200 {
			t.Fatalf("expected embedded asset %s status 200, got %d", entry.Name(), getResp.Code)
		}

		headResp := performRequest(t, router, "HEAD", "/assets/"+entry.Name())
		if headResp.Code != 200 {
			t.Fatalf("expected embedded asset %s HEAD status 200, got %d", entry.Name(), headResp.Code)
		}
	}
}
