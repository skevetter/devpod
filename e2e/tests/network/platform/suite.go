package platform

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestPlatform(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Network Platform E2E Suite")
}

func DevPodDescribe(text string, body func()) bool {
	return ginkgo.Describe("[network:platform] "+text, body)
}
