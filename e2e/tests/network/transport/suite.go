package transport

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestTransport(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Network Transport E2E Suite")
}

func DevPodDescribe(text string, body func()) bool {
	return ginkgo.Describe("[network:transport] "+text, ginkgo.Label("network"), body)
}
