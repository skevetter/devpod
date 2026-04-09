package json

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

// GetCmd retrieves a JSON value by path.
type GetCmd struct {
	File string
	Fail bool
}

// NewGetCmd creates a new ssh command.
func NewGetCmd() *cobra.Command {
	cmd := &GetCmd{}
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Retrieves a JSON value by JSONPath",
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context(), args)
		},
	}

	getCmd.Flags().StringVarP(&cmd.File, "file", "f", "", "Parse this json file instead of STDIN")
	getCmd.Flags().BoolVar(&cmd.Fail, "fail", false, "Fail if value is not found")

	return getCmd
}

// jsonPathToGjson converts a JSONPath expression to a gjson path.
// It handles expressions like $.foo.bar, $.foo[0].bar, $[0], .foo, [0].foo.
var bracketIndex = regexp.MustCompile(`\[(\d+)\]`)

func jsonPathToGjson(path string) (string, error) {
	if path == "" || path == "$" || path == "$." {
		return "@this", nil
	}
	if strings.Contains(path, "..") ||
		strings.Contains(path, "[?(") ||
		strings.Contains(path, "*") {
		return "", fmt.Errorf("unsupported jsonpath expression: %s", path)
	}

	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	path = bracketIndex.ReplaceAllString(path, ".$1")
	path = strings.TrimPrefix(path, ".")

	return path, nil
}

func writeResult(result gjson.Result) {
	switch result.Type {
	case gjson.String:
		_, _ = os.Stdout.WriteString(result.String())
	case gjson.True:
		_, _ = os.Stdout.WriteString("true")
	case gjson.False:
		_, _ = os.Stdout.WriteString("false")
	default:
		_, _ = os.Stdout.WriteString(result.Raw)
	}
}

// Run executes the get command.
func (cmd *GetCmd) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("jsonpath expected")
	}

	jsonBytes, err := cmd.readInput()
	if err != nil {
		return err
	}

	if !gjson.ValidBytes(jsonBytes) {
		return fmt.Errorf("parse json")
	}

	gjsonPath, err := jsonPathToGjson(args[0])
	if err != nil {
		return err
	}
	result := gjson.GetBytes(jsonBytes, gjsonPath)

	if !result.Exists() {
		if cmd.Fail {
			return fmt.Errorf("unknown key %s", args[0])
		}

		return nil
	}

	writeResult(result)

	return nil
}

func (cmd *GetCmd) readInput() ([]byte, error) {
	if cmd.File != "" {
		return os.ReadFile(cmd.File) //nolint:gosec // file path comes from CLI flag
	}
	return io.ReadAll(os.Stdin)
}
