package upgrade

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Context("testing upgrade command", ginkgo.Label("upgrade"), ginkgo.Ordered, func() {

	ginkgo.It("should detect correct binary for current OS and architecture using dry-run", func(ctx context.Context) {
		initialDir, err := os.Getwd()
		framework.ExpectNoError(err, "getting current working directory should not error")

		f := framework.NewDefaultFramework(initialDir + "/bin")

		// Run upgrade with dry-run flag
		output, err := f.ExecCommandOutput(ctx, []string{"upgrade", "--dry-run"})
		framework.ExpectNoError(err, "upgrade --dry-run should not error")

		ginkgo.By("Parsing dry-run key=value output")

		// Parse key=value output
		lines := strings.Split(strings.TrimSpace(output), "\n")
		values := make(map[string]string)
		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				values[parts[0]] = parts[1]
			}
		}

		// Verify OS matches current runtime
		expectedOS := runtime.GOOS
		framework.ExpectEqual(values["os"], expectedOS, "OS should match runtime")

		// Verify architecture matches current runtime
		expectedArch := runtime.GOARCH
		framework.ExpectEqual(values["arch"], expectedArch, "Arch should match runtime")

		// Verify asset name contains correct OS and arch
		expectedAssetPattern := "devpod-" + expectedOS + "-" + expectedArch
		framework.ExpectEqual(strings.Contains(values["asset_name"], expectedAssetPattern), true, "Asset name should contain OS and Arch")
	})
})
