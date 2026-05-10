package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NyanLoli-Network/baka.life/registryctl/parser"
	"github.com/NyanLoli-Network/baka.life/registryctl/validator"
)

type Result struct {
	Registered     bool
	RequiresReview bool
	AutoMerge      bool
	ChangedFiles   []string
	Reasons        []string
}

func Check(baseRoot, headRoot, author string) (Result, error) {
	var result Result

	baseRegistry, err := parser.ParseRegistry(baseRoot)
	if err != nil {
		return result, fmt.Errorf("parse base registry: %w", err)
	}
	if err := validator.Validate(baseRegistry); err != nil {
		return result, fmt.Errorf("validate base registry: %w", err)
	}

	headRegistry, err := parser.ParseRegistry(headRoot)
	if err != nil {
		return result, fmt.Errorf("parse PR registry: %w", err)
	}
	if err := validator.Validate(headRegistry); err != nil {
		return result, fmt.Errorf("validate PR registry: %w", err)
	}

	changedFiles, err := ChangedFiles(baseRoot, headRoot)
	if err != nil {
		return result, err
	}
	result.ChangedFiles = changedFiles
	result.Registered = validator.HasGitHubAuth(baseRegistry, author)

	if len(changedFiles) == 0 {
		result.RequiresReview = true
		result.Reasons = append(result.Reasons, "PR has no file changes")
	}
	if !result.Registered {
		result.RequiresReview = true
		result.Reasons = append(result.Reasons, "GitHub author is not registered in the base registry")
	}

	domainFiles := make([]string, 0, len(changedFiles))
	for _, file := range changedFiles {
		if isDirectDomainFile(file) {
			domainFiles = append(domainFiles, file)
			continue
		}
		result.RequiresReview = true
		result.Reasons = append(result.Reasons, fmt.Sprintf("%s is not an auto-mergeable domain registry file", file))
	}

	if !result.RequiresReview {
		if err := validator.AuthorizeDomainChanges(baseRegistry, headRegistry, domainFiles, author); err != nil {
			return result, err
		}
		result.AutoMerge = true
	}

	return result, nil
}

func ChangedFiles(baseRoot, headRoot string) ([]string, error) {
	baseFiles, err := hashTree(baseRoot)
	if err != nil {
		return nil, err
	}
	headFiles, err := hashTree(headRoot)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	for file := range baseFiles {
		seen[file] = struct{}{}
	}
	for file := range headFiles {
		seen[file] = struct{}{}
	}

	changed := make([]string, 0)
	for file := range seen {
		if baseFiles[file] != headFiles[file] {
			changed = append(changed, file)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func hashTree(root string) (map[string]string, error) {
	files := map[string]string{}
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files[rel] = hashString("symlink:" + target)
			return nil
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		files[rel] = hash
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashString(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func isDirectDomainFile(file string) bool {
	const prefix = "registry/domain/"
	if !strings.HasPrefix(file, prefix) {
		return false
	}
	name := strings.TrimPrefix(file, prefix)
	return name != "" && !strings.Contains(name, "/")
}
