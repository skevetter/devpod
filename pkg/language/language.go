package language

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/log"
)

type ProgrammingLanguage string

const (
	JavaScript ProgrammingLanguage = "JavaScript"
	TypeScript ProgrammingLanguage = "TypeScript"
	Python     ProgrammingLanguage = "Python"
	Go         ProgrammingLanguage = "Go"
	Cpp        ProgrammingLanguage = "C++"
	C          ProgrammingLanguage = "C"
	DotNet     ProgrammingLanguage = "C#"
	PHP        ProgrammingLanguage = "Php"
	Java       ProgrammingLanguage = "Java"
	Rust       ProgrammingLanguage = "Rust"
	Ruby       ProgrammingLanguage = "Ruby"
	None       ProgrammingLanguage = "None"
)

var SupportedLanguages = map[ProgrammingLanguage]bool{
	JavaScript: true,
	TypeScript: true,
	Python:     true,
	C:          true,
	Cpp:        true,
	DotNet:     true,
	Go:         true,
	PHP:        true,
	Java:       true,
	Rust:       true,
	Ruby:       true,
	None:       true,
}

var MapLanguages = map[ProgrammingLanguage]ProgrammingLanguage{
	TypeScript: JavaScript,
	C:          Cpp,
}

var MapConfig = map[ProgrammingLanguage]*config.DevContainerConfig{
	None: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/base:ubuntu",
		},
	},
	JavaScript: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/javascript-node",
		},
	},
	Python: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/python:3",
		},
	},
	Java: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/java",
		},
	},
	Go: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/go",
		},
	},
	Rust: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/rust:latest",
		},
	},
	Ruby: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/ruby",
		},
	},
	PHP: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/php",
		},
	},
	Cpp: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/cpp",
		},
	},
	DotNet: {
		ImageContainer: config.ImageContainer{
			Image: "mcr.microsoft.com/devcontainers/dotnet",
		},
	},
}

// extensionToLanguage maps file extensions to programming languages.
var extensionToLanguage = map[string]ProgrammingLanguage{
	".js":   JavaScript,
	".ts":   TypeScript,
	".py":   Python,
	".c":    C,
	".cpp":  Cpp,
	".cs":   DotNet,
	".go":   Go,
	".php":  PHP,
	".java": Java,
	".rs":   Rust,
	".rb":   Ruby,
}

// skipDirs contains directory names that should be skipped during language detection.
var skipDirs = map[string]bool{
	"node_modules":     true,
	"vendor":           true,
	"Vendor":           true,
	".git":             true,
	".github":          true,
	".vscode":          true,
	"dist":             true,
	"deps":             true,
	"cache":            true,
	"testdata":         true,
	"Godeps":           true,
	"bower_components": true,
}

func DefaultConfig(startPath string, log log.Logger) *config.DevContainerConfig {
	language, err := DetectLanguage(startPath)
	if err != nil {
		log.Errorf("Error detecting project language: %v", err)
		log.Infof("Couldn't detect project language, fallback to 'None'")
		return MapConfig[None]
	} else if MapConfig[language] == nil {
		log.Infof("Couldn't detect project language, fallback to 'None'")
		return MapConfig[None]
	}

	log.Infof("Detected project language '%s'", language)
	return MapConfig[language]
}

func DetectLanguage(startPath string) (ProgrammingLanguage, error) {
	maxFiles := 5000

	root, err := filepath.Abs(startPath)
	if err != nil {
		return None, err
	}

	fileInfo, err := os.Stat(root)
	if err != nil {
		return None, err
	}

	if fileInfo.Mode().IsRegular() {
		return None, fmt.Errorf("path is a regular file, not a directory: %s", root)
	}

	language := detectLanguageByExtension(root, maxFiles)
	if !SupportedLanguages[language] {
		return None, nil
	}

	if MapLanguages[language] != "" {
		language = MapLanguages[language]
	}

	return language, nil
}

func shouldSkipDir(name string) bool {
	if skipDirs[name] {
		return true
	}
	return name != "." && strings.HasPrefix(name, ".")
}

// countLanguageFiles walks the directory tree and counts files by extension.
func countLanguageFiles(root string, maxFiles int) map[ProgrammingLanguage]int {
	counts := make(map[ProgrammingLanguage]int)
	fileCount := 0

	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if fileCount >= maxFiles {
			return filepath.SkipAll
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if lang, ok := extensionToLanguage[strings.ToLower(filepath.Ext(d.Name()))]; ok {
			counts[lang]++
		}
		fileCount++
		return nil
	})

	return counts
}

func detectLanguageByExtension(root string, maxFiles int) ProgrammingLanguage {
	counts := countLanguageFiles(root, maxFiles)

	best := None
	max := 0
	for lang, count := range counts {
		if count > max || (count == max && lang < best) {
			best = lang
			max = count
		}
	}
	return best
}
