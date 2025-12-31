package context

import (
	"context"
	"encoding/json"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = DevPodDescribe("devpod context test suite", func() {
	ginkgo.Context("testing context command", ginkgo.Label("context"), ginkgo.Ordered, func() {
		ctx := context.Background()
		initialDir, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		ginkgo.It("create a new context, switch to it and delete afterwards", func() {
			f := framework.NewDefaultFramework(initialDir + "/bin")

			err = f.DevPodContextCreate(ctx, "test-context")
			framework.ExpectNoError(err)

			err = f.DevPodContextUse(context.Background(), "test-context")
			framework.ExpectNoError(err)

			err = f.DevPodContextDelete(context.Background(), "test-context")
			framework.ExpectNoError(err)
		})

		// written by @skevetter; copied from https://github.com/skevetter/devpod/pull/162
		ginkgo.It("should use shared context in IDE commands", func() {
			f := framework.NewDefaultFramework(initialDir + "/bin")

			contextA := "test-ctx-a-ide"
			contextB := "test-ctx-b-ide"

			err = f.DevPodContextCreate(ctx, contextA)
			framework.ExpectNoError(err)

			err = f.DevPodContextCreate(ctx, contextB)
			framework.ExpectNoError(err)

			ginkgo.DeferCleanup(func() {
				_ = f.DevPodContextDelete(ctx, contextA)
				_ = f.DevPodContextDelete(ctx, contextB)
			})

			err = f.DevPodContextUse(ctx, contextA)
			framework.ExpectNoError(err)

			err = f.DevPodIDEUse(ctx, "intellij", "--context", contextB)
			framework.ExpectNoError(err)

			output, err := f.DevPodIDEList(ctx, "--output", "json")
			framework.ExpectNoError(err)

			var ides []map[string]any
			err = json.Unmarshal([]byte(output), &ides)
			framework.ExpectNoError(err)

			for _, ide := range ides {
				if ide["name"] == "intellij" {
					if defaultVal, exists := ide["default"]; exists && defaultVal == true {
						ginkgo.Fail("IDE was incorrectly set in context-a instead of context-b")
					}
				}
			}

			output, err = f.DevPodIDEList(ctx, "--context", contextB, "--output", "json")
			framework.ExpectNoError(err)

			err = json.Unmarshal([]byte(output), &ides)
			framework.ExpectNoError(err)

			intellijFound := false
			for _, ide := range ides {
				if ide["name"] == "intellij" {
					if defaultVal, exists := ide["default"]; exists && defaultVal == true {
						intellijFound = true
						break
					}
				}
			}
			if !intellijFound {
				ginkgo.Fail("IDE was not set in context-b as expected")
			}

			err = os.Setenv("DEVPOD_CONTEXT", contextB)
			framework.ExpectNoError(err)

			output, err = f.DevPodIDEList(ctx, "--output", "json")
			framework.ExpectNoError(err)

			err = json.Unmarshal([]byte(output), &ides)
			framework.ExpectNoError(err)

			intellijFound = false
			for _, ide := range ides {
				if ide["name"] == "intellij" {
					if defaultVal, exists := ide["default"]; exists && defaultVal == true {
						intellijFound = true
						break
					}
				}
			}
			if !intellijFound {
				ginkgo.Fail("Selecting context-b using environment variable DEVPOD_CONTEXT does not work as expected")
			}
		})

	})
})
