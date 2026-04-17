package web

import (
	"errors"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
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
	return func(c *gin.Context) {
		if source.dist == nil {
			c.Status(http.StatusNotFound)
			return
		}

		requestedPath := strings.TrimPrefix(c.Param("filepath"), "/")
		cleanedPath := path.Clean(requestedPath)
		if requestedPath == "" || cleanedPath == "." || cleanedPath == ".." || strings.HasPrefix(cleanedPath, "../") {
			c.Status(http.StatusNotFound)
			return
		}

		assetPath := path.Join(dir, cleanedPath)
		data, contentType, err := readStaticFile(source.dist, assetPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				c.Status(http.StatusNotFound)
				return
			}
			log.Printf("static asset read failed from %s: %s: %v", source.label, assetPath, err)
			c.Status(http.StatusInternalServerError)
			return
		}

		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Header("Content-Length", strconv.Itoa(len(data)))
		if c.Request.Method == http.MethodHead {
			if contentType != "" {
				c.Header("Content-Type", contentType)
			}
			c.Status(http.StatusOK)
			return
		}

		c.Data(http.StatusOK, contentType, data)
	}
}

func readStaticFile(root fs.FS, assetPath string) ([]byte, string, error) {
	info, err := fs.Stat(root, assetPath)
	if err != nil {
		return nil, "", err
	}
	if info.IsDir() {
		return nil, "", fs.ErrNotExist
	}

	data, err := fs.ReadFile(root, assetPath)
	if err != nil {
		return nil, "", err
	}

	contentType := mime.TypeByExtension(path.Ext(info.Name()))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func isAPIRoute(route string) bool {
	return strings.HasPrefix(route, "/api") || strings.HasPrefix(route, "/v1")
}

func looksLikeAssetPath(route string) bool {
	base := path.Base(route)
	return strings.Contains(base, ".")
}
