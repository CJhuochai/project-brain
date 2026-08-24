package workspace

import (
	"io/fs"
	"path/filepath"
	"sort"
)

var skippedDirectories = map[string]bool{
	".git": true, "target": true, "node_modules": true, "vendor": true, ".idea": true,
}

func Discover(root string) ([]Repository, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var repositories []Repository
	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if skippedDirectories[entry.Name()] {
			if entry.Name() == ".git" {
				repoPath := filepath.Dir(path)
				if !seen[repoPath] {
					repository, discoverErr := discoverRepository(repoPath)
					if discoverErr != nil {
						return discoverErr
					}
					repositories = append(repositories, repository)
					seen[repoPath] = true
				}
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i].Path < repositories[j].Path })
	return repositories, nil
}
