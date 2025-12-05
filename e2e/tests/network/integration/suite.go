package integration

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Network Integration E2E Suite")
}

func DevPodDescribe(text string, body func()) bool {
	return ginkgo.Describe("[network:integration] "+text, ginkgo.Label("network"), body)
}
