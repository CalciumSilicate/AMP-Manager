//go:build !embed_frontend

package web

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterStaticRoutesFallsBackWithoutDist(t *testing.T) {
	t.Setenv("AMP_WEB_DIST_DIR", filepath.Join(t.TempDir(), "missing-dist"))
	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterStaticRoutes(router)

	resp := performRequest(t, router, "GET", "/dashboard")
	if resp.Code != 200 {
		t.Fatalf("expected fallback page status 200, got %d", resp.Code)
	}
	if body := resp.Body.String(); !contains(body, "Frontend assets are missing") {
		t.Fatalf("expected fallback page body, got %q", body)
	}

	apiResp := performRequest(t, router, "GET", "/api/unknown")
	if apiResp.Code != 404 {
		t.Fatalf("expected api 404, got %d", apiResp.Code)
	}
	if body := apiResp.Body.String(); body != "{\"error\":\"not found\"}" {
		t.Fatalf("expected api not found json, got %q", body)
	}

	assetResp := performRequest(t, router, "GET", "/assets/app.js")
	if assetResp.Code != 404 {
		t.Fatalf("expected asset 404, got %d", assetResp.Code)
	}
}

func TestRegisterStaticRoutesServesDiskDist(t *testing.T) {
	distDir := filepath.Join(t.TempDir(), "dist")
	mustNoError(t, os.MkdirAll(filepath.Join(distDir, "assets"), 0o755))
	mustNoError(t, os.MkdirAll(filepath.Join(distDir, "fonts"), 0o755))
	mustNoError(t, os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html>ok</html>"), 0o644))
	mustNoError(t, os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log('ok');"), 0o644))

	t.Setenv("AMP_WEB_DIST_DIR", distDir)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterStaticRoutes(router)

	indexResp := performRequest(t, router, "GET", "/settings")
	if indexResp.Code != 200 {
		t.Fatalf("expected index status 200, got %d", indexResp.Code)
	}
	if body := indexResp.Body.String(); body != "<html>ok</html>" {
		t.Fatalf("expected index html, got %q", body)
	}

	assetResp := performRequest(t, router, "GET", "/assets/app.js")
	if assetResp.Code != 200 {
		t.Fatalf("expected asset status 200, got %d", assetResp.Code)
	}
	if body := assetResp.Body.String(); !contains(body, "console.log") {
		t.Fatalf("expected asset body, got %q", body)
	}
}

func TestCandidateDistDirsFromIncludesRepoRootForNestedExecutable(t *testing.T) {
	base := filepath.Join("/tmp", "ampmanager", "package", "dist")
	got := candidateDistDirsFrom("", "", base)

	wantPrefix := []string{
		filepath.Join(base, "web", "dist"),
		filepath.Join(base, "internal", "web", "dist"),
		filepath.Join("/tmp", "ampmanager", "package", "web", "dist"),
		filepath.Join("/tmp", "ampmanager", "package", "internal", "web", "dist"),
		filepath.Join("/tmp", "ampmanager", "web", "dist"),
		filepath.Join("/tmp", "ampmanager", "internal", "web", "dist"),
		filepath.Join("/tmp", "web", "dist"),
		filepath.Join("/tmp", "internal", "web", "dist"),
	}

	if !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("unexpected candidate prefix:\nwant: %#v\ngot:  %#v", wantPrefix, got[:len(wantPrefix)])
	}
}
