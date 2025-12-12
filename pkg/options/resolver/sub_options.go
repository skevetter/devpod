package resolver

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/shell"
	"github.com/skevetter/devpod/pkg/types"
)

func execOptionCommand(ctx context.Context, command string, resolvedOptions map[string]config.OptionValue, extraValues map[string]string) (*bytes.Buffer, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	env := os.Environ()
	for k, v := range combine(resolvedOptions, extraValues) {
		env = append(env, k+"="+v)
	}

	err := shell.RunEmulatedShell(ctx, command, nil, stdout, stderr, env)
	if err != nil {
		return nil, fmt.Errorf("exec command: %s%s %w", stdout.String(), stderr.String(), err)
	}

	return stdout, nil
}

func resolveFromCommand(ctx context.Context, option *types.Option, resolvedOptions map[string]config.OptionValue, extraValues map[string]string) (config.OptionValue, error) {
	cmdOut, err := execOptionCommand(ctx, option.Command, resolvedOptions, extraValues)
	if err != nil {
		return config.OptionValue{}, fmt.Errorf("run command %w", err)
	}
	optionValue := config.OptionValue{Value: strings.TrimSpace(cmdOut.String())}
	expire := types.NewTime(time.Now())
	optionValue.Filled = &expire
	return optionValue, nil
}

func resolveSubOptions(ctx context.Context, option *types.Option, resolvedOptions map[string]config.OptionValue, extraValues map[string]string) (config.OptionDefinitions, error) {
	cmdOut, err := execOptionCommand(ctx, option.SubOptionsCommand, resolvedOptions, extraValues)
	if err != nil {
		return nil, fmt.Errorf("run subOptionsCommand %w", err)
	}
	subOptions := provider.SubOptions{}
	err = yaml.Unmarshal(cmdOut.Bytes(), &subOptions)
	if err != nil {
		return nil, fmt.Errorf("parse subOptionsCommand: %s %w", cmdOut.String(), err)
	}

	// prepare new options
	// need to look for option in graph. should be rather easy because we don't need to traverse the whole graph
	retOpts := config.OptionDefinitions{}
	maps.Copy(retOpts, subOptions.Options)

	return retOpts, nil
}
