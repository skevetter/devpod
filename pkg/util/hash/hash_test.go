package hash

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HashTestSuite struct {
	suite.Suite
	tempDir string
}

func (s *HashTestSuite) SetupTest() {
	s.tempDir = s.T().TempDir()
}

func TestHashSuite(t *testing.T) {
	suite.Run(t, new(HashTestSuite))
}

func (s *HashTestSuite) TestDirectoryHash_EmptyDirectory() {
	hash, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	// dirhash.Hash1 returns a hash even for empty directories (hash of empty input)
	assert.NotEmpty(s.T(), hash)
	assert.Equal(s.T(), "h1:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=", hash)
}

func (s *HashTestSuite) TestDirectoryHash_SingleFile() {
	s.createFile("file.txt", "content")

	hash1, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash1)

	hash2, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_MultipleFiles() {
	s.createFile("file1.txt", "content1")
	s.createFile("file2.txt", "content2")

	hash1, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash1)

	// Change content
	s.createFile("file1.txt", "changed")
	hash2, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_NestedDirectories() {
	s.createFile("dir1/file1.txt", "content1")
	s.createFile("dir1/subdir/file2.txt", "content2")
	s.createFile("dir2/file3.txt", "content3")

	hash, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_ExcludePatterns_Basic() {
	s.createFile("file.txt", "content")
	s.createFile("file.log", "log content")

	hash1, err := DirectoryHash(s.tempDir, []string{"*.log"}, nil)
	require.NoError(s.T(), err)

	// Change excluded file - hash should not change
	s.createFile("file.log", "changed log")
	hash2, err := DirectoryHash(s.tempDir, []string{"*.log"}, nil)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change included file - hash should change
	s.createFile("file.txt", "changed")
	hash3, err := DirectoryHash(s.tempDir, []string{"*.log"}, nil)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) TestDirectoryHash_ExcludePatterns_Directory() {
	s.createFile("src/main.go", "package main")
	s.createFile("node_modules/lib/index.js", "module.exports = {}")

	hash1, err := DirectoryHash(s.tempDir, []string{"node_modules"}, nil)
	require.NoError(s.T(), err)

	// Change excluded directory - hash should not change
	s.createFile("node_modules/lib/index.js", "changed")
	hash2, err := DirectoryHash(s.tempDir, []string{"node_modules"}, nil)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_ExcludePatterns_Precedence() {
	s.createFile("scripts/build.sh", "build")
	s.createFile("scripts/install.sh", "install")

	// Exclude takes precedence over include
	hash1, err := DirectoryHash(s.tempDir, []string{"scripts/install.sh"}, []string{"scripts"})
	require.NoError(s.T(), err)

	// Change excluded file - hash should not change
	s.createFile("scripts/install.sh", "changed")
	hash2, err := DirectoryHash(s.tempDir, []string{"scripts/install.sh"}, []string{"scripts"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change included file - hash should change
	s.createFile("scripts/build.sh", "changed")
	hash3, err := DirectoryHash(s.tempDir, []string{"scripts/install.sh"}, []string{"scripts"})
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) TestDirectoryHash_IncludeFiles_Empty() {
	s.createFile("file1.txt", "content1")
	s.createFile("file2.txt", "content2")

	// Empty include list should include all files
	hash, err := DirectoryHash(s.tempDir, nil, []string{})
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_IncludeFiles_ExactMatch() {
	s.createFile("test", "test content")
	s.createFile("testing", "testing content")

	// Should only match "test", not "testing"
	hash1, err := DirectoryHash(s.tempDir, nil, []string{"test"})
	require.NoError(s.T(), err)

	// Change "testing" - hash should not change (not included)
	s.createFile("testing", "changed")
	hash2, err := DirectoryHash(s.tempDir, nil, []string{"test"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change "test" - hash should change
	s.createFile("test", "changed")
	hash3, err := DirectoryHash(s.tempDir, nil, []string{"test"})
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) TestDirectoryHash_IncludeFiles_DirectoryPrefix() {
	s.createFile("test/file.go", "test")
	s.createFile("testing/file.go", "testing")

	// Should only match "test/" directory, not "testing/"
	hash1, err := DirectoryHash(s.tempDir, nil, []string{"test"})
	require.NoError(s.T(), err)

	// Change "testing/" - hash should not change
	s.createFile("testing/file.go", "changed")
	hash2, err := DirectoryHash(s.tempDir, nil, []string{"test"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change "test/" - hash should change
	s.createFile("test/file.go", "changed")
	hash3, err := DirectoryHash(s.tempDir, nil, []string{"test"})
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) TestDirectoryHash_IncludeFiles_TrailingSeparator() {
	s.createFile("src/main.go", "package main")

	hash1, err := DirectoryHash(s.tempDir, nil, []string{"src/"})
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash1)

	// Should work the same without trailing separator
	hash2, err := DirectoryHash(s.tempDir, nil, []string{"src"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_IncludeFiles_Multiple() {
	s.createFile("src/main.go", "main")
	s.createFile("test/test.go", "test")
	s.createFile("docs/readme.md", "docs")

	hash1, err := DirectoryHash(s.tempDir, nil, []string{"src", "test"})
	require.NoError(s.T(), err)

	// Change excluded directory - hash should not change
	s.createFile("docs/readme.md", "changed")
	hash2, err := DirectoryHash(s.tempDir, nil, []string{"src", "test"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change included directory - hash should change
	s.createFile("src/main.go", "changed")
	hash3, err := DirectoryHash(s.tempDir, nil, []string{"src", "test"})
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) TestDirectoryHash_FileLimitExceeded() {
	// Create 5001 files
	for i := range 5001 {
		s.createFile(filepath.Join("files", fmt.Sprintf("file%d.txt", i)), "content")
	}

	hash, err := DirectoryHash(s.tempDir, nil, nil)

	// Should return partial hash
	assert.NotEmpty(s.T(), hash, "hash should not be empty, got error: %v", err)

	// Should return error
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "exceeded limit")
}

func (s *HashTestSuite) TestDirectoryHash_FileLimitExact() {
	// Create exactly 5000 files
	for i := range 5000 {
		s.createFile(filepath.Join("files", fmt.Sprintf("file%d.txt", i)), "content")
	}

	hash, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_FileLimitUnder() {
	// Create 100 files
	for i := range 100 {
		s.createFile(filepath.Join("files", fmt.Sprintf("file%d.txt", i)), "content")
	}

	hash, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_SymlinkDirectory() {
	s.createFile("target/file.txt", "content")
	s.createSymlink("link", "target")

	hash, err := DirectoryHash(filepath.Join(s.tempDir, "link"), nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_SymlinkFile() {
	s.createFile("target.txt", "content")
	s.createSymlink("link.txt", "target.txt")

	hash, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_NonExistentPath() {
	hash, err := DirectoryHash(filepath.Join(s.tempDir, "nonexistent"), nil, nil)
	require.Error(s.T(), err)
	assert.Empty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_FileNotDirectory() {
	s.createFile("file.txt", "content")

	hash, err := DirectoryHash(filepath.Join(s.tempDir, "file.txt"), nil, nil)
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "not a directory")
	assert.Empty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_InvalidExcludePattern() {
	s.createFile("file.txt", "content")

	hash, err := DirectoryHash(s.tempDir, []string{"["}, nil)
	require.Error(s.T(), err)
	assert.Empty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_Deterministic() {
	s.createFile("file1.txt", "content1")
	s.createFile("file2.txt", "content2")

	hash1, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)

	hash2, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)

	assert.Equal(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_OrderIndependent() {
	// Create files in one order
	tempDir1 := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(filepath.Join(tempDir1, "a.txt"), []byte("a"), 0600))
	require.NoError(s.T(), os.WriteFile(filepath.Join(tempDir1, "b.txt"), []byte("b"), 0600))
	require.NoError(s.T(), os.WriteFile(filepath.Join(tempDir1, "c.txt"), []byte("c"), 0600))

	// Create files in different order
	tempDir2 := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(filepath.Join(tempDir2, "c.txt"), []byte("c"), 0600))
	require.NoError(s.T(), os.WriteFile(filepath.Join(tempDir2, "a.txt"), []byte("a"), 0600))
	require.NoError(s.T(), os.WriteFile(filepath.Join(tempDir2, "b.txt"), []byte("b"), 0600))

	hash1, err := DirectoryHash(tempDir1, nil, nil)
	require.NoError(s.T(), err)

	hash2, err := DirectoryHash(tempDir2, nil, nil)
	require.NoError(s.T(), err)

	assert.Equal(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_ContentSensitive() {
	s.createFile("file.txt", "A")

	hash1, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)

	s.createFile("file.txt", "B")

	hash2, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)

	assert.NotEqual(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_SpecialCharacters() {
	s.createFile("file (1).txt", "content")
	s.createFile("file[2].txt", "content")
	s.createFile("file{3}.txt", "content")

	hash, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_DotFiles() {
	s.createFile(".hidden", "content")
	s.createFile("visible.txt", "content")

	hash1, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)

	// Change hidden file - hash should change
	s.createFile(".hidden", "changed")
	hash2, err := DirectoryHash(s.tempDir, nil, nil)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash2)
}

func (s *HashTestSuite) TestDirectoryHash_RelativePath() {
	s.createFile("file.txt", "content")

	// Test that relative paths work by using a subdirectory
	subDir := filepath.Join(s.tempDir, "subdir")
	require.NoError(s.T(), os.MkdirAll(subDir, 0750))
	require.NoError(s.T(), os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0600))

	// Use relative path from tempDir
	relPath, err := filepath.Rel(s.tempDir, subDir)
	require.NoError(s.T(), err)

	// DirectoryHash will convert relative to absolute internally
	hash, err := DirectoryHash(filepath.Join(s.tempDir, relPath), nil, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), hash)
}

func (s *HashTestSuite) TestDirectoryHash_RealWorldScenario_NodeProject() {
	s.createFile("src/index.js", "console.log('hello')")
	s.createFile("src/utils.js", "module.exports = {}")
	s.createFile("package.json", "{}")
	s.createFile("node_modules/lib/index.js", "module.exports = {}")
	s.createFile("build.log", "build output")

	hash1, err := DirectoryHash(s.tempDir, []string{"node_modules", "*.log"}, []string{"src", "package.json"})
	require.NoError(s.T(), err)

	// Change excluded files - hash should not change
	s.createFile("node_modules/lib/index.js", "changed")
	s.createFile("build.log", "changed")
	hash2, err := DirectoryHash(s.tempDir, []string{"node_modules", "*.log"}, []string{"src", "package.json"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change included file - hash should change
	s.createFile("src/index.js", "changed")
	hash3, err := DirectoryHash(s.tempDir, []string{"node_modules", "*.log"}, []string{"src", "package.json"})
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) TestDirectoryHash_RealWorldScenario_GoProject() {
	s.createFile("main.go", "package main")
	s.createFile("pkg/util/helper.go", "package util")
	s.createFile("vendor/lib/lib.go", "package lib")
	s.createFile("main.test", "test binary")

	hash1, err := DirectoryHash(s.tempDir, []string{"vendor", "*.test"}, nil)
	require.NoError(s.T(), err)

	// Change excluded files - hash should not change
	s.createFile("vendor/lib/lib.go", "changed")
	s.createFile("main.test", "changed")
	hash2, err := DirectoryHash(s.tempDir, []string{"vendor", "*.test"}, nil)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), hash1, hash2)

	// Change included file - hash should change
	s.createFile("main.go", "changed")
	hash3, err := DirectoryHash(s.tempDir, []string{"vendor", "*.test"}, nil)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), hash1, hash3)
}

func (s *HashTestSuite) createFile(relPath, content string) {
	fullPath := filepath.Join(s.tempDir, relPath)
	dir := filepath.Dir(fullPath)
	require.NoError(s.T(), os.MkdirAll(dir, 0750))
	require.NoError(s.T(), os.WriteFile(fullPath, []byte(content), 0600))
}

func (s *HashTestSuite) createSymlink(link, target string) {
	linkPath := filepath.Join(s.tempDir, link)
	targetPath := filepath.Join(s.tempDir, target)
	require.NoError(s.T(), os.Symlink(targetPath, linkPath))
}
