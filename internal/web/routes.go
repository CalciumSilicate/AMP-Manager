package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

const fallbackHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AMP Manager</title>
  <style>
    body { font-family: sans-serif; margin: 0; padding: 2rem; background: #0f172a; color: #e2e8f0; }
    main { max-width: 42rem; margin: 10vh auto; background: rgba(15, 23, 42, 0.9); border: 1px solid #334155; border-radius: 16px; padding: 2rem; }
    h1 { margin-top: 0; font-size: 1.75rem; }
    p { line-height: 1.6; color: #cbd5e1; }
    code { background: #111827; padding: 0.1rem 0.35rem; border-radius: 6px; }
  </style>
</head>
<body>
  <main>
    <h1>Frontend assets are missing</h1>
    <p>The backend is running, but the frontend build output was not found.</p>
    <p>Build the frontend into <code>web/dist</code> for development, or use the <code>embed_frontend</code> build tag for release binaries.</p>
  </main>
</body>
</html>
`

type assetSource struct {
	dist       fs.FS
	label      string
	indexFound bool
}

// RegisterStaticRoutes registers routes to serve the frontend.
func RegisterStaticRoutes(r *gin.Engine) {
	registerStaticRoutes(r, loadAssetSource())
}

func registerStaticRoutes(r *gin.Engine, source assetSource) {
	r.GET("/assets/*filepath", serveStaticDir(source, "assets"))
	r.HEAD("/assets/*filepath", serveStaticDir(source, "assets"))
	r.GET("/fonts/*filepath", serveStaticDir(source, "fonts"))
	r.HEAD("/fonts/*filepath", serveStaticDir(source, "fonts"))

	r.NoRoute(func(c *gin.Context) {
		if isAPIRoute(c.Request.URL.Path) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		if source.indexFound {
			indexHTML, err := fs.ReadFile(source.dist, "index.html")
			if err == nil {
				c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
				return
			}
		}

		if looksLikeAssetPath(c.Request.URL.Path) {
			c.Status(http.StatusNotFound)
			return
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fallbackHTML))
	})
}

func serveStaticDir(source assetSource, dir string) gin.HandlerFunc {
	sub, ok := subFS(source.dist, dir)
	if !ok {
		return func(c *gin.Context) {
			c.Status(http.StatusNotFound)
		}
	}

	fileServer := http.StripPrefix("/"+dir, http.FileServer(http.FS(sub)))
	return func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func subFS(root fs.FS, dir string) (fs.FS, bool) {
	if root == nil {
		return nil, false
	}
	sub, err := fs.Sub(root, dir)
	if err != nil {
		return nil, false
	}
	return sub, true
}

func isAPIRoute(route string) bool {
	return strings.HasPrefix(route, "/api") || strings.HasPrefix(route, "/v1")
}

func looksLikeAssetPath(route string) bool {
	base := path.Base(route)
	return strings.Contains(base, ".")
}
