package context

import (
	"context"
	"encoding/json"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/skevetter/devpod/e2e/framework"
)

const ideIntelliJ = "intellij"

var _ = ginkgo.Describe(
	"devpod context test suite",
	ginkgo.Label("context"),
	ginkgo.Ordered,
	func() {
		var initialDir string

		ginkgo.BeforeAll(func() {
			var err error
			initialDir, err = os.Getwd()
			framework.ExpectNoError(err)
		})

		ginkgo.It(
			"create a new context, switch to it and delete afterwards",
			ginkgo.SpecTimeout(framework.GetTimeout()),
			func(ctx context.Context) {
				f := framework.NewDefaultFramework(initialDir + "/bin")

				var err error
				err = f.DevPodContextCreate(ctx, "test-context")
				framework.ExpectNoError(err)

				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					cleanupErr := f.DevPodContextDelete(cleanupCtx, "test-context")
					framework.ExpectNoError(cleanupErr)
				})

				err = f.DevPodContextUse(ctx, "test-context")
				framework.ExpectNoError(err)
			},
		)

		ginkgo.It(
			"should use shared context in IDE commands",
			ginkgo.SpecTimeout(framework.GetTimeout()),
			func(ctx context.Context) {
				f := framework.NewDefaultFramework(initialDir + "/bin")

				contextA := "test-ctx-a-ide"
				contextB := "test-ctx-b-ide"

				var err error
				err = f.DevPodContextCreate(ctx, contextA)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = f.DevPodContextDelete(cleanupCtx, contextA)
				})

				err = f.DevPodContextCreate(ctx, contextB)
				framework.ExpectNoError(err)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					err = f.DevPodContextDelete(cleanupCtx, contextB)
					framework.ExpectNoError(err)
				})

				err = f.DevPodContextUse(ctx, contextA)
				framework.ExpectNoError(err)

				err = f.DevPodIDEUse(ctx, ideIntelliJ, "--context", contextB)
				framework.ExpectNoError(err)

				output, err := f.DevPodIDEList(ctx, "--output", "json")
				framework.ExpectNoError(err)

				var ides []map[string]any
				err = json.Unmarshal([]byte(output), &ides)
				framework.ExpectNoError(err)
				gomega.Expect(ides).NotTo(gomega.BeEmpty(), "IDE list should not be empty")

				for _, ide := range ides {
					if ide["name"] == ideIntelliJ {
						if defaultVal, exists := ide["default"]; exists && defaultVal == true {
							ginkgo.Fail("IDE was incorrectly set in context-a instead of context-b")
						}
					}
				}

				output, err = f.DevPodIDEList(ctx, "--context", contextB, "--output", "json")
				framework.ExpectNoError(err)

				err = json.Unmarshal([]byte(output), &ides)
				framework.ExpectNoError(err)
				gomega.Expect(ides).
					NotTo(gomega.BeEmpty(), "IDE list for context-b should not be empty")

				intellijFound := false
				for _, ide := range ides {
					if ide["name"] == ideIntelliJ {
						if defaultVal, exists := ide["default"]; exists && defaultVal == true {
							intellijFound = true
							break
						}
					}
				}
				gomega.Expect(intellijFound).To(
					gomega.BeTrue(), "IDE should be set as default in context-b",
				)

				ginkgo.GinkgoT().Setenv("DEVPOD_CONTEXT", contextB)

				output, err = f.DevPodIDEList(ctx, "--output", "json")
				framework.ExpectNoError(err)

				err = json.Unmarshal([]byte(output), &ides)
				framework.ExpectNoError(err)
				gomega.Expect(ides).NotTo(
					gomega.BeEmpty(),
					"IDE list via DEVPOD_CONTEXT should not be empty",
				)

				intellijFound = false
				for _, ide := range ides {
					if ide["name"] == ideIntelliJ {
						if defaultVal, exists := ide["default"]; exists && defaultVal == true {
							intellijFound = true
							break
						}
					}
				}
				gomega.Expect(intellijFound).To(
					gomega.BeTrue(),
					"DEVPOD_CONTEXT env var should select context-b with intellij as default IDE",
				)
			},
		)
	},
)
