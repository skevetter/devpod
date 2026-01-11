package resolver

import (
	"fmt"
	"maps"
	"sort"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/graph"
	"github.com/skevetter/devpod/pkg/types"
)

func combine(resolvedOptions map[string]config.OptionValue, extraValues map[string]string) map[string]string {
	options := map[string]string{}
	maps.Copy(options, extraValues)
	for k, v := range resolvedOptions {
		options[k] = v.Value
	}
	return options
}

func addDependencies(g *graph.Graph[*types.Option], options config.OptionDefinitions, optionValues map[string]config.OptionValue) error {
	for optionName := range options {
		err := addDependency(g, optionValues, optionName)
		if err != nil {
			return err
		}
	}
	return nil
}

func addDependency(g *graph.Graph[*types.Option], optionValues map[string]config.OptionValue, optionName string) error {
	option, exists := g.GetNode(optionName)
	if !exists {
		return nil
	}

	for _, childName := range optionValues[optionName].Children {
		if !g.HasNode(childName) || childName == optionName {
			continue
		}

		childOption, childExists := g.GetNode(childName)
		if childExists && childOption != nil {
			if !option.Global && childOption.Global {
				return fmt.Errorf("cannot use a global option as a dependency of a non-global option. Option '%s' used in children of option '%s'", childName, optionName)
			}
			if option.Local && !childOption.Local {
				return fmt.Errorf("cannot use a non-local option as a dependency of a local option. Option '%s' used in children of option '%s'", childName, optionName)
			}
		}

		_ = g.AddEdge(optionName, childName)
	}

	for _, dep := range findVariables(option.Default) {
		if !g.HasNode(dep) || dep == optionName {
			continue
		}

		depOption, depExists := g.GetNode(dep)
		if depExists && depOption != nil {
			if option.Global && !depOption.Global {
				return fmt.Errorf("cannot use a non-global option as a dependency of a global option. Option '%s' used in default of option '%s'", dep, optionName)
			}
			if !option.Local && depOption.Local {
				return fmt.Errorf("cannot use a local option as a dependency of a non-local option. Option '%s' used in default of option '%s'", dep, optionName)
			}
		}

		_ = g.AddEdge(dep, optionName)
	}

	for _, dep := range findVariables(option.Command) {
		if !g.HasNode(dep) || dep == optionName {
			continue
		}

		depOption, depExists := g.GetNode(dep)
		if depExists && depOption != nil {
			if option.Global && !depOption.Global {
				return fmt.Errorf("cannot use a non-global option as a dependency of a global option. Option '%s' used in command of option '%s'", dep, optionName)
			}
			if !option.Local && depOption.Local {
				return fmt.Errorf("cannot use a local option as a dependency of a non-local option. Option '%s' used in command of option '%s'", dep, optionName)
			}
		}

		_ = g.AddEdge(dep, optionName)
	}

	return nil
}

func addOptionsToGraph(g *graph.Graph[*types.Option], optionDefinitions config.OptionDefinitions, optionValues map[string]config.OptionValue) error {
	if !g.HasNode(rootID) {
		_ = g.AddNode(rootID, nil)
	}

	for optionName, option := range optionDefinitions {
		_ = g.SetNode(optionName, option)
		_ = g.AddEdge(rootID, optionName)
	}

	err := addDependencies(g, optionDefinitions, optionValues)
	if err != nil {
		return err
	}

	return nil
}

func findVariables(str string) []string {
	retVars := map[string]bool{}
	matches := variableExpression.FindAllStringSubmatch(str, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			retVars[match[1]] = true
		}
	}

	retVarsArr := []string{}
	for k := range retVars {
		retVarsArr = append(retVarsArr, k)
	}

	sort.Strings(retVarsArr)
	return retVarsArr
}

func mergeMaps[K comparable, V any](existing map[K]V, newOpts map[K]V) map[K]V {
	retOpts := map[K]V{}
	maps.Copy(retOpts, existing)
	maps.Copy(retOpts, newOpts)

	return retOpts
}
