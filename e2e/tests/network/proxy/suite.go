package proxy

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestProxy(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Network Proxy E2E Suite")
}

func DevPodDescribe(text string, body func()) bool {
	return ginkgo.Describe("[network:proxy] "+text, ginkgo.Label("network"), body)
}
