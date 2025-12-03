package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("ping", func() {
	ginkgo.Context("devpod binary", ginkgo.Label("ping"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("devpod binary exists and is executable", ginkgo.Label("cli-binary"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-cli-binary"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, name, "test -x /usr/local/bin/devpod && echo 'executable' || echo 'not executable'")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "executable")
		})
	})
})
