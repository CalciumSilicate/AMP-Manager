package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var staticFS embed.FS

// RegisterStaticRoutes registers routes to serve the embedded frontend
func RegisterStaticRoutes(r *gin.Engine) {
	// Get the dist subdirectory
	distFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic("failed to get dist subdirectory: " + err.Error())
	}
	httpFS := http.FS(distFS)

	// Serve static assets
	r.StaticFS("/assets", http.FS(mustSub(distFS, "assets")))

	// Serve index.html for all non-API routes (SPA fallback)
	r.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		if len(c.Request.URL.Path) >= 3 && c.Request.URL.Path[:3] == "/v1" {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}

		requestPath := path.Clean("/" + c.Request.URL.Path)
		if requestPath != "/" {
			distPath := strings.TrimPrefix(requestPath, "/")
			if fileExists(distFS, distPath) {
				c.FileFromFS(distPath, httpFS)
				return
			}

			// Avoid returning index.html for missing static files such as fonts.
			if strings.Contains(path.Base(distPath), ".") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
		}

		indexHTML, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			c.String(500, "failed to read index.html")
			return
		}
		c.Data(200, "text/html; charset=utf-8", indexHTML)
	})
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("failed to get subdirectory: " + err.Error())
	}
	return sub
}

func fileExists(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
