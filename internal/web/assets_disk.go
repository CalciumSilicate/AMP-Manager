//go:build !embed_frontend

package web

import (
	"io/fs"
	"os"
	"path/filepath"
)

func loadAssetSource() assetSource {
	for _, candidate := range candidateDistDirs() {
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}

		source := assetSource{
			dist:  os.DirFS(candidate),
			label: candidate,
		}
		if _, err := fs.Stat(source.dist, "index.html"); err == nil {
			source.indexFound = true
		}
		return source
	}

	return assetSource{}
}

func candidateDistDirs() []string {
	candidates := make([]string, 0, 5)
	if override := os.Getenv("AMP_WEB_DIST_DIR"); override != "" {
		candidates = append(candidates, override)
	}

	cwd, err := os.Getwd()
	if err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "web", "dist"),
			filepath.Join(cwd, "internal", "web", "dist"),
		)
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "web", "dist"),
			filepath.Join(exeDir, "..", "web", "dist"),
			filepath.Join(exeDir, "internal", "web", "dist"),
		)
	}

	return uniquePaths(candidates)
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		cleaned := filepath.Clean(p)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}
