package network

import (
	"os"

	"github.com/onsi/ginkgo/v2"
)

var initialDir = func() string {
	dir, _ := os.Getwd()
	return dir
}()

// DevPodDescribe annotates the test with the label.
func DevPodDescribe(text string, body func()) bool {
	return ginkgo.Describe("[network] "+text, body)
}
