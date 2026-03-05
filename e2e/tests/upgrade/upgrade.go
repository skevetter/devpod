package upgrade

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("testing upgrade command", ginkgo.Label("upgrade"), ginkgo.Ordered, func() {
	ginkgo.It(
		"should detect correct binary for current OS and architecture using dry-run",
		func(ctx context.Context) {
			initialDir, err := os.Getwd()
			framework.ExpectNoError(err, "getting current working directory should not error")

			f := framework.NewDefaultFramework(initialDir + "/bin")
			output, err := f.ExecCommandOutput(ctx, []string{"upgrade", "--dry-run"})
			framework.ExpectNoError(err, "upgrade --dry-run should not error")

			ginkgo.By("Parsing dry-run key=value output")
			lines := strings.Split(strings.TrimSpace(output), "\n")
			values := make(map[string]string)
			for _, line := range lines {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					values[parts[0]] = parts[1]
				}
			}

			expectedOS := runtime.GOOS
			framework.ExpectEqual(values["os"], expectedOS, "OS should match runtime")

			expectedArch := runtime.GOARCH
			framework.ExpectEqual(values["arch"], expectedArch, "Arch should match runtime")

			expectedAssetPattern := "devpod-" + expectedOS + "-" + expectedArch
			gomega.Expect(values["asset_name"]).
				To(gomega.ContainSubstring(expectedAssetPattern), "Asset name should contain OS and Arch")
		},
	)
})
