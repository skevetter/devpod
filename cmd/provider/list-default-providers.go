package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/skevetter/devpod/cmd/flags"
	devpodhttp "github.com/skevetter/devpod/pkg/http"
	"github.com/spf13/cobra"
)

// ListAvailableCmd holds the list cmd flags.
type ListAvailableCmd struct {
	*flags.GlobalFlags
}

// NewListAvailableCmd creates a new command.
func NewListAvailableCmd(flags *flags.GlobalFlags) *cobra.Command {
	cmd := &ListAvailableCmd{
		GlobalFlags: flags,
	}
	listAvailableCmd := &cobra.Command{
		Use:   "list-available",
		Short: "List providers available for installation",
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context())
		},
	}

	return listAvailableCmd
}

// Run runs the command logic.
func (cmd *ListAvailableCmd) Run(ctx context.Context) error {
	jsonResult, err := fetchProviderRepos(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stdout, "List of available providers from skevetter:")
	for _, v := range jsonResult {
		name, ok := v["name"].(string)
		if !ok || name == "" {
			continue
		}
		if after, ok0 := strings.CutPrefix(name, "devpod-provider-"); ok0 {
			_, _ = fmt.Fprintln(os.Stdout, "\t", after)
		}
	}

	return nil
}

func fetchProviderRepos(ctx context.Context) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx,
		"GET",
		"https://api.github.com/users/skevetter/repos?per_page=100",
		nil,
	)
	if err != nil {
		return nil, err
	}
	resp, err := devpodhttp.GetHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(result))
	}

	var jsonResult []map[string]any
	if err := json.Unmarshal(result, &jsonResult); err != nil {
		return nil, err
	}

	return jsonResult, nil
}
