### E2E tests

#### Prerequisites

Make sure you have ginkgo installed on your local machine:
```
go get github.com/onsi/ginkgo/ginkgo
```

To build the binaries locally use the following command from this directory
```
BUILDDIR=bin SRCDIR=".." ../hack/build-e2e.sh
```

#### Kubernetes Tests Setup

For tests that require Kubernetes (labeled with `up-kubernetes` or `build`), you need to set up a kind cluster:

```bash
kind create cluster --image kindest/node:v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a
```

To delete the cluster after testing:
```bash
kind delete cluster
```

#### Run all E2E test
```
# Install ginkgo and run in this directory
ginkgo
```
