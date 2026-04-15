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
	var override string
	if env := os.Getenv("AMP_WEB_DIST_DIR"); env != "" {
		override = env
	}

	var cwd string
	if dir, err := os.Getwd(); err == nil {
		cwd = dir
	}

	var exeDir string
	if exePath, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exePath)
	}

	return candidateDistDirsFrom(override, cwd, exeDir)
}

func candidateDistDirsFrom(override string, cwd string, exeDir string) []string {
	candidates := make([]string, 0, 16)
	if override != "" {
		candidates = append(candidates, override)
	}

	if cwd != "" {
		candidates = append(candidates,
			filepath.Join(cwd, "web", "dist"),
			filepath.Join(cwd, "internal", "web", "dist"),
		)
	}

	if exeDir != "" {
		for _, base := range parentDirs(exeDir, 3) {
			candidates = append(candidates,
				filepath.Join(base, "web", "dist"),
				filepath.Join(base, "internal", "web", "dist"),
			)
		}
	}

	return uniquePaths(candidates)
}

func parentDirs(root string, maxDepth int) []string {
	dirs := make([]string, 0, maxDepth+1)
	current := filepath.Clean(root)
	for depth := 0; depth <= maxDepth; depth++ {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dirs
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
