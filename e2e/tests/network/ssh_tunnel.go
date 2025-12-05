package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("SSH tunnel traffic", func() {
	ginkgo.Context("SSH tunnel data transfer", ginkgo.Label("network", "ssh-tunnel"), func() {
		ginkgo.It("transfers large data through SSH", ginkgo.Label("large-data"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "dd if=/dev/zero of=/tmp/largefile bs=1024 count=1024 2>/dev/null")
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "stat -c %s /tmp/largefile")
			framework.ExpectNoError(err)
			framework.ExpectEqual(strings.TrimSpace(out), "1048576")
		})

		ginkgo.It("handles binary data transfer", ginkgo.Label("binary-data"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			_, err = f.DevPodSSH(ctx, tempDir, "dd if=/dev/urandom of=/tmp/binary bs=1024 count=10 2>/dev/null")
			framework.ExpectNoError(err)

			out, err := f.DevPodSSH(ctx, tempDir, "md5sum /tmp/binary | awk '{print $1}'")
			framework.ExpectNoError(err)
			checksum1 := strings.TrimSpace(out)

			_, err = f.DevPodSSH(ctx, tempDir, "cp /tmp/binary /tmp/binary2")
			framework.ExpectNoError(err)

			out, err = f.DevPodSSH(ctx, tempDir, "md5sum /tmp/binary2 | awk '{print $1}'")
			framework.ExpectNoError(err)
			checksum2 := strings.TrimSpace(out)

			framework.ExpectEqual(checksum1, checksum2)
		})

		ginkgo.It("maintains connection stability under load", ginkgo.Label("stability"), func() {
			ctx := context.Background()
			f := framework.NewDefaultFramework(initialDir + "/bin")
			_ = f.DevPodProviderAdd(ctx, "docker", "-o", "DOCKER_PATH=docker")
			_ = f.DevPodProviderUse(ctx, "docker")

			tempDir, err := framework.CopyToTempDir(initialDir + "/tests/network/testdata/simple-app")
			framework.ExpectNoError(err)
			ginkgo.DeferCleanup(framework.CleanupTempDir, initialDir, tempDir)

			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)

			for i := range 10 {
				out, err := f.DevPodSSH(ctx, tempDir, fmt.Sprintf("echo 'iteration-%d'", i))
				framework.ExpectNoError(err)
				framework.ExpectEqual(strings.Contains(out, fmt.Sprintf("iteration-%d", i)), true)
			}
		})
	})
})
