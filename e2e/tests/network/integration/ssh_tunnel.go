package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("SSH tunnel traffic", func() {
	ginkgo.Context("SSH tunnel data transfer", ginkgo.Label("ssh-tunnel"), func() {
		var initialDir string

		ginkgo.BeforeEach(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It("transfers large data through SSH", ginkgo.Label("large-data"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-large-data"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Create a 1MB file
			_, err = f.DevPodSSH(ctx, name, "dd if=/dev/zero of=/tmp/largefile bs=1024 count=1024 2>/dev/null")
			framework.ExpectNoError(err)

			// Verify file size
			out, err := f.DevPodSSH(ctx, name, "stat -c %s /tmp/largefile")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "1048576")
		})

		ginkgo.It("handles binary data transfer", ginkgo.Label("binary-data"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-binary-data"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Create binary file and get checksum
			_, err = f.DevPodSSH(ctx, name, "dd if=/dev/urandom of=/tmp/binary bs=1024 count=10 2>/dev/null")
			framework.ExpectNoError(err)

			// Get checksum
			out, err := f.DevPodSSH(ctx, name, "md5sum /tmp/binary | awk '{print $1}'")
			framework.ExpectNoError(err)
			checksum1 := strings.TrimSpace(out)

			// Copy file and verify checksum matches
			_, err = f.DevPodSSH(ctx, name, "cp /tmp/binary /tmp/binary2")
			framework.ExpectNoError(err)

			out, err = f.DevPodSSH(ctx, name, "md5sum /tmp/binary2 | awk '{print $1}'")
			framework.ExpectNoError(err)
			checksum2 := strings.TrimSpace(out)

			framework.ExpectEqual(checksum1, checksum2)
		})

		ginkgo.It("maintains connection stability under load", ginkgo.Label("stability"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/../../bin")

			_ = f.DevPodProviderDelete(ctx, "docker")
			err := f.DevPodProviderAdd(ctx, "docker")
			framework.ExpectNoError(err)
			err = f.DevPodProviderUse(ctx, "docker")
			framework.ExpectNoError(err)

			testDir := filepath.Join(initialDir, "testdata", "simple-app")
			name := "test-stability"
			ginkgo.DeferCleanup(f.DevPodWorkspaceDelete, context.Background(), name)

			err = f.DevPodUp(ctx, testDir, "--id", name)
			framework.ExpectNoError(err)

			// Make 10 sequential connections
			for i := range 10 {
				out, err := f.DevPodSSH(ctx, name, fmt.Sprintf("echo 'iteration-%d'", i))
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.Contains(out, fmt.Sprintf("iteration-%d", i)), true)
			}
		})
	})
})
