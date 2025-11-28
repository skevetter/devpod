package telemetry

import "github.com/skevetter/devpod/pkg/client"

type noopCollector struct{}

func (n *noopCollector) RecordCLI(err error)                         {}
func (n *noopCollector) SetClient(client client.BaseWorkspaceClient) {}

func (n *noopCollector) Flush() {}
