package up

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/docker/docker/api/types/container"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
	"github.com/skevetter/devpod/pkg/compose"
	docker "github.com/skevetter/devpod/pkg/docker"
	provider2 "github.com/skevetter/devpod/pkg/provider"
)

type baseTestContext struct {
	f            *framework.Framework
	dockerHelper *docker.DockerHelper
	initialDir   string
}

func (btc *baseTestContext) execSSH(ctx context.Context, tempDir, command string) (string, error) {
	return btc.f.DevPodSSH(ctx, tempDir, command)
}

func (btc *baseTestContext) inspectContainer(ctx context.Context, ids []string) (*container.InspectResponse, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no container IDs provided")
	}
	var details []container.InspectResponse
	if err := btc.dockerHelper.Inspect(ctx, ids, "container", &details); err != nil {
		return nil, err
	}
	if len(details) == 0 {
		return nil, fmt.Errorf("no container details returned")
	}
	return &details[0], nil
}

type testContext struct {
	baseTestContext
	composeHelper *compose.ComposeHelper
}

func (tc *testContext) setupAndStartWorkspace(ctx context.Context, testDataPath string, upArgs ...string) (string, *provider2.Workspace, error) {
	tempDir, err := setupWorkspace(testDataPath, tc.initialDir, tc.f)
	if err != nil {
		return "", nil, err
	}
	workspace, err := devPodUpAndFindWorkspace(ctx, tc.f, tempDir, upArgs...)
	return tempDir, workspace, err
}

func (tc *testContext) getAppContainer(ctx context.Context, workspace *provider2.Workspace) ([]string, *container.InspectResponse, error) {
	ids, err := findComposeContainer(ctx, tc.dockerHelper, tc.composeHelper, workspace.UID, "app")
	if err != nil || len(ids) == 0 {
		return ids, nil, err
	}
	detail, err := tc.inspectContainer(ctx, ids)
	return ids, detail, err
}

func (tc *testContext) verifyWorkspaceMount(ctx context.Context, workspace *provider2.Workspace, tempDir string) error {
	_, detail, err := tc.getAppContainer(ctx, workspace)
	if err != nil {
		return err
	}
	gomega.Expect(detail.Mounts).To(gomega.HaveLen(1), "1 container volume mount")
	mount := detail.Mounts[0]
	gomega.Expect(mount.Source).To(gomega.Equal(tempDir))
	gomega.Expect(mount.Destination).To(gomega.Equal("/workspaces"))
	gomega.Expect(mount.RW).To(gomega.BeTrue())
	return nil
}

func setupWorkspace(testdataPath, initialDir string, f *framework.Framework) (string, error) {
	tempDir, err := framework.CopyToTempDir(testdataPath)
	if err != nil {
		return "", err
	}
	ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)
	ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, tempDir)
	return tempDir, nil
}

func setupDockerProvider(binDir, dockerPath string) (*framework.Framework, error) {
	f := framework.NewDefaultFramework(binDir)
	_ = f.DevPodProviderDelete(context.Background(), "docker")
	_ = f.DevPodProviderAdd(context.Background(), "docker", "-o", "DOCKER_PATH="+dockerPath)
	return f, f.DevPodProviderUse(context.Background(), "docker")
}

func findComposeContainer(ctx context.Context, dockerHelper *docker.DockerHelper, composeHelper *compose.ComposeHelper, workspaceUID, serviceName string) ([]string, error) {
	return dockerHelper.FindContainer(ctx, []string{
		fmt.Sprintf("%s=%s", compose.ProjectLabel, composeHelper.GetProjectName(workspaceUID)),
		fmt.Sprintf("%s=%s", compose.ServiceLabel, serviceName),
	})
}

func devPodUpAndFindWorkspace(ctx context.Context, f *framework.Framework, tempDir string, args ...string) (*provider2.Workspace, error) {
	if err := f.DevPodUp(ctx, append([]string{tempDir}, args...)...); err != nil {
		return nil, err
	}
	return f.FindWorkspace(ctx, tempDir)
}

func getContainerUID(ctx context.Context, f *framework.Framework, workspaceDir, username string) int {
	out, err := f.DevPodSSH(ctx, workspaceDir, fmt.Sprintf("id -u %s", username))
	framework.ExpectNoError(err)
	uid, err := strconv.Atoi(strings.TrimSpace(out))
	framework.ExpectNoError(err)
	return uid
}

func getContainerGID(ctx context.Context, f *framework.Framework, workspaceDir, username string) int {
	out, err := f.DevPodSSH(ctx, workspaceDir, fmt.Sprintf("id -g %s", username))
	framework.ExpectNoError(err)
	gid, err := strconv.Atoi(strings.TrimSpace(out))
	framework.ExpectNoError(err)
	return gid
}

func verifyContainerUser(ctx context.Context, f *framework.Framework, workspaceDir, expectedUser string) {
	out, err := f.DevPodSSH(ctx, workspaceDir, "whoami")
	framework.ExpectNoError(err)
	ginkgo.By(fmt.Sprintf("container user %s", strings.TrimSpace(out)))
	gomega.Expect(strings.TrimSpace(out)).To(gomega.Equal(expectedUser), fmt.Sprintf("remote container user should be %s", expectedUser))
}

func verifyUIDMapping(containerUID, containerGID, hostUID, hostGID, defaultUID, defaultGID int, username string) {
	ginkgo.By(fmt.Sprintf("container UID mapping: %s=%d, expected=%d", username, containerUID, hostUID))
	ginkgo.By(fmt.Sprintf("container GID mapping: %s=%d, expected=%d", username, containerGID, hostGID))

	if hostUID == 0 {
		ginkgo.By("running as root user on host")
		gomega.Expect(containerUID).To(gomega.Equal(defaultUID), fmt.Sprintf("%s user UID should remain %d when host is root", username, defaultUID))
		gomega.Expect(containerGID).To(gomega.Equal(defaultGID), fmt.Sprintf("%s user GID should remain %d when host is root", username, defaultGID))
	} else {
		ginkgo.By("running as non-root user on host")
		gomega.Expect(containerUID).To(gomega.Equal(hostUID), fmt.Sprintf("%s user UID should match host user UID", username))
		gomega.Expect(containerGID).To(gomega.Equal(hostGID), fmt.Sprintf("%s user GID should match host user GID", username))
	}
}

func verifyHostFileAccess(filePath, expectedContent string) {
	content, err := os.ReadFile(filePath)
	framework.ExpectNoError(err)
	gomega.Expect(string(content)).To(gomega.ContainSubstring(expectedContent), "host file should be accessible to host user")
}

func verifyHostFileOwnership(filePath string, expectedUID, expectedGID int, isRootHost bool) {
	info, err := os.Stat(filePath)
	framework.ExpectNoError(err)
	stat := info.Sys().(*syscall.Stat_t)

	if isRootHost {
		ginkgo.By(fmt.Sprintf("Host file ownership: uid=%d, gid=%d (container user owns files when host is root)", int(stat.Uid), int(stat.Gid)))
	} else {
		gomega.Expect(int(stat.Uid)).To(gomega.Equal(expectedUID), "host file UID should match expected UID")
		gomega.Expect(int(stat.Gid)).To(gomega.Equal(expectedGID), "host file GID should match expected GID")
	}
}
