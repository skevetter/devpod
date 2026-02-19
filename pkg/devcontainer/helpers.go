package devcontainer

import (
	"bufio"
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/skevetter/devpod/pkg/file"
	"github.com/skevetter/devpod/pkg/git"
	"github.com/skevetter/devpod/pkg/image"
	"github.com/skevetter/log"
)

type GetWorkspaceConfigResult struct {
	IsImage         bool     `json:"isImage"`
	IsGitRepository bool     `json:"isGitRepository"`
	IsLocal         bool     `json:"isLocal"`
	ConfigPaths     []string `json:"configPaths"`
}

type gitRepoFindOptions struct {
	gitRepository         string
	gitPRReference        string
	gitBranch             string
	gitCommit             string
	gitSubDir             string
	tmpDirPath            string
	strictHostKeyChecking bool
	maxDepth              int
	log                   log.Logger
}

func FindDevcontainerFiles(ctx context.Context, rawSource, tmpDirPath string, maxDepth int, strictHostKeyChecking bool, log log.Logger) (*GetWorkspaceConfigResult, error) {
	// local path
	isLocalPath, _ := file.IsLocalDir(rawSource)
	if isLocalPath {
		return FindFilesInLocalDir(rawSource, maxDepth, log)
	}

	// git repo
	gitRepository, gitPRReference, gitBranch, gitCommit, gitSubDir := git.NormalizeRepository(rawSource)
	if strings.HasSuffix(rawSource, ".git") || git.PingRepository(gitRepository, git.GetDefaultExtraEnv(strictHostKeyChecking)) {
		log.Debug("Git repository detected")
		opts := gitRepoFindOptions{
			gitRepository:         gitRepository,
			gitPRReference:        gitPRReference,
			gitBranch:             gitBranch,
			gitCommit:             gitCommit,
			gitSubDir:             gitSubDir,
			tmpDirPath:            tmpDirPath,
			strictHostKeyChecking: strictHostKeyChecking,
			maxDepth:              maxDepth,
			log:                   log,
		}
		return findFilesInGitRepo(ctx, opts)
	}

	result := &GetWorkspaceConfigResult{ConfigPaths: []string{}}

	// container image
	_, err := image.GetImage(ctx, rawSource)
	if err == nil {
		log.Debug("Container image detected")
		result.IsImage = true
		// Doesn't matter, we just want to know if it's an image
		// not going to poke around in the image fs
		return result, nil
	}

	return result, nil
}

func FindFilesInLocalDir(rawSource string, maxDepth int, log log.Logger) (*GetWorkspaceConfigResult, error) {
	log.Debug("Local directory detected")
	result := &GetWorkspaceConfigResult{ConfigPaths: []string{}}
	result.IsLocal = true
	initialDepth := strings.Count(rawSource, string(filepath.Separator))
	err := filepath.WalkDir(rawSource, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		depth := strings.Count(path, string(filepath.Separator)) - initialDepth
		if info.IsDir() && depth > maxDepth {
			return filepath.SkipDir
		}

		if isDevcontainerFilename(path) {
			relPath, err := filepath.Rel(rawSource, path)
			if err != nil {
				log.Warnf("Unable to get relative path for %s: %s", path, err.Error())
				return nil
			}
			result.ConfigPaths = append(result.ConfigPaths, relPath)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func findFilesInGitRepo(ctx context.Context, opts gitRepoFindOptions) (*GetWorkspaceConfigResult, error) {
	result := &GetWorkspaceConfigResult{
		ConfigPaths:     []string{},
		IsGitRepository: true,
	}

	gitInfo := git.NewGitInfo(opts.gitRepository, opts.gitBranch, opts.gitCommit, opts.gitPRReference, opts.gitSubDir)
	opts.log.Debugf("Cloning Git repository into %s", opts.tmpDirPath)
	err := git.CloneRepository(ctx, gitInfo, opts.tmpDirPath, "", opts.strictHostKeyChecking, opts.log, git.WithCloneStrategy(git.BareCloneStrategy))
	if err != nil {
		return nil, err
	}

	opts.log.Debug("Listing git file tree")
	ref := "HEAD"
	// checkout on branch if available
	if opts.gitBranch != "" {
		ref = opts.gitBranch
	}
	// git ls-tree -r --full-name --name-only $REF
	lsArgs := []string{"ls-tree", "-r", "--full-name", "--name-only", ref}
	lsCmd := git.CommandContext(ctx, git.GetDefaultExtraEnv(opts.strictHostKeyChecking), lsArgs...)
	lsCmd.Dir = opts.tmpDirPath
	stdout, err := lsCmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = lsCmd.Start()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		path := scanner.Text()
		depth := strings.Count(path, string(filepath.Separator))
		if depth > opts.maxDepth {
			continue
		}
		if isDevcontainerFilename(path) {
			result.ConfigPaths = append(result.ConfigPaths, path)
		}
	}

	err = lsCmd.Wait()
	if err != nil {
		return nil, err
	}

	return result, nil
}

func isDevcontainerFilename(path string) bool {
	return filepath.Base(path) == ".devcontainer.json" || filepath.Base(path) == "devcontainer.json"
}
