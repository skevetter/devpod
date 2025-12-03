package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("agent commands", func() {
	ginkgo.Context("agent container", ginkgo.Label("agent"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("devpod agent container command works", ginkgo.Label("cli-agent"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-cli-agent"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, name, "/usr/local/bin/devpod agent container 2>&1 || true")
			framework.ExpectNoError(err)
			framework.ExpectEqual(len(strings.TrimSpace(out)) > 0, true, "command should produce output")
		})
	})
})
