package connection

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestConnection(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Network Connection E2E Suite")
}

func DevPodDescribe(text string, body func()) bool {
	return ginkgo.Describe("[network:connection] "+text, body)
}
