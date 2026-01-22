package up

import (
	"context"
	"os"
	"os/exec"

	"github.com/onsi/ginkgo/v2"
	"github.com/skevetter/devpod/e2e/framework"
)

var _ = ginkgo.Describe("testing up command for podman provider", ginkgo.Label("up-provider-podman"), ginkgo.Ordered, func() {
	var initialDir string

	ginkgo.BeforeEach(func() {
		var err error
		initialDir, err = os.Getwd()
		framework.ExpectNoError(err)
	})

	ginkgo.Context("with rootless podman", ginkgo.Ordered, func() {
		var f *framework.Framework

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			f, err = setupDockerProvider(initialDir+"/bin", "podman")
			framework.ExpectNoError(err)
		})

		ginkgo.It("should start a new workspace with existing image", func(ctx context.Context) {
			tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
			framework.ExpectNoError(err)

			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))
	})

	ginkgo.Context("with rootfull podman", ginkgo.Ordered, func() {
		var f *framework.Framework

		ginkgo.BeforeAll(func(ctx context.Context) {
			wrapper, err := os.Create(initialDir + "/bin/podman-rootful")
			framework.ExpectNoError(err)

			defer func() { _ = wrapper.Close() }()

			_, err = wrapper.WriteString(`#!/bin/sh
				sudo podman "$@"
				`)
			framework.ExpectNoError(err)

			err = wrapper.Close()
			framework.ExpectNoError(err)

			cmd := exec.Command("sudo", "chmod", "+x", initialDir+"/bin/podman-rootful")
			err = cmd.Run()
			framework.ExpectNoError(err)

			err = exec.Command(initialDir+"/bin/podman-rootful", "ps").Run()
			framework.ExpectNoError(err)
		})

		ginkgo.BeforeEach(func(ctx context.Context) {
			var err error
			f, err = setupDockerProvider(initialDir+"/bin", initialDir+"/bin/podman-rootful")
			framework.ExpectNoError(err)
		})

		ginkgo.It("should start a new workspace with existing image", func(ctx context.Context) {
			tempDir, err := setupWorkspace("tests/up/testdata/docker", initialDir, f)
			framework.ExpectNoError(err)

			// Wait for devpod workspace to come online (deadline: 30s)
			err = f.DevPodUp(ctx, tempDir)
			framework.ExpectNoError(err)
		}, ginkgo.SpecTimeout(framework.GetTimeout()))
	})
})
