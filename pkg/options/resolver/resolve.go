package resolver

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log/survey"
	"github.com/skevetter/log/terminal"
)

func (r *Resolver) resolveOptions(
	ctx context.Context,
	optionValues map[string]config.OptionValue,
) (map[string]config.OptionValue, error) {
	resolvedOptionValues := map[string]config.OptionValue{}
	maps.Copy(resolvedOptionValues, optionValues)

	sortedOptionNames, err := r.graph.SortNodeIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to sort options: %w", err)
	}

	for _, optionName := range sortedOptionNames {
		if optionName == rootID {
			continue
		}

		if !r.graph.HasNode(optionName) {
			continue
		}

		err := r.resolveOption(ctx, optionName, resolvedOptionValues)
		if err != nil {
			return nil, fmt.Errorf("resolve option %s %w", optionName, err)
		}

		err = r.refreshSubOptions(ctx, optionName, resolvedOptionValues)
		if err != nil {
			return nil, fmt.Errorf("refresh sub options for %s %w", optionName, err)
		}
	}

	return resolvedOptionValues, nil
}

func (r *Resolver) resolveOption(
	ctx context.Context,
	optionName string,
	resolvedOptionValues map[string]config.OptionValue,
) error {
	option, exists := r.graph.GetNode(optionName)
	if !exists {
		return fmt.Errorf("option %s not found in graph", optionName)
	}

	// get existing values
	userValue, userValueOk, beforeValue, beforeValueOk, err := r.getValue(optionName, option, resolvedOptionValues)
	if err != nil {
		return err
	}

	// find out options we need to resolve
	if !userValueOk {
		// check if value is already filled
		if beforeValueOk {
			if beforeValue.UserProvided || option.Cache == "" {
				return nil
			} else if option.Cache != "" {
				duration, err := time.ParseDuration(option.Cache)
				if err != nil {
					return fmt.Errorf("parse cache duration of option %s %w", optionName, err)
				}

				// has value expired?
				if beforeValue.Filled != nil && beforeValue.Filled.Add(duration).After(time.Now()) {
					return nil
				}
			}
		}

		// make sure required is always resolved
		if !option.Required {
			// skip if global
			if !r.resolveGlobal && option.Global {
				return nil
			} else if !r.resolveLocal && option.Local {
				return nil
			}
		}
	}

	// resolve option
	if userValueOk {
		resolvedOptionValues[optionName] = config.OptionValue{
			Value:        userValue,
			Children:     beforeValue.Children,
			UserProvided: true,
		}
	} else if option.Default != "" {
		resolvedOptionValues[optionName] = config.OptionValue{
			Children: beforeValue.Children,
			Value:    ResolveDefaultValue(option.Default, combine(resolvedOptionValues, r.extraValues)),
		}
	} else if option.Command != "" {
		optionValue, err := resolveFromCommand(ctx, option, resolvedOptionValues, r.extraValues)
		if err != nil {
			return err
		}

		optionValue.Children = beforeValue.Children
		resolvedOptionValues[optionName] = optionValue
	} else if len(option.Enum) == 1 {
		resolvedOptionValues[optionName] = config.OptionValue{
			Children: beforeValue.Children,
			Value:    option.Enum[0].Value,
		}
	} else {
		resolvedOptionValues[optionName] = config.OptionValue{
			Children: beforeValue.Children,
		}
	}

	// is required?
	if !userValueOk && option.Required && resolvedOptionValues[optionName].Value == "" && !resolvedOptionValues[optionName].UserProvided {
		if r.skipRequired {
			delete(resolvedOptionValues, optionName)
			return r.graph.RemoveChildren(optionName)
		}

		// check if we can ask a question
		if !terminal.IsTerminalIn {
			return fmt.Errorf("option %s is required, but no value provided", optionName)
		}

		questionOpts := []string{}
		for _, enumOpt := range option.Enum {
			questionOpts = append(questionOpts, enumOpt.Value)
		}

		// check if there is only one option
		r.log.Info(option.Description)
		answer, err := r.log.Question(&survey.QuestionOptions{
			Question:               fmt.Sprintf("Please enter a value for %s", optionName),
			Options:                questionOpts,
			ValidationRegexPattern: option.ValidationPattern,
			ValidationMessage:      option.ValidationMessage,
			IsPassword:             option.Password,
		})
		if err != nil {
			return err
		}

		resolvedOptionValues[optionName] = config.OptionValue{
			Value:        answer,
			UserProvided: true,
		}
	}

	// check if value has changed
	if beforeValue.Value != resolvedOptionValues[optionName].Value {
		children := r.graph.GetChildren(optionName)
		for _, childID := range children {
			optionValue, ok := resolvedOptionValues[childID]
			if ok && !optionValue.UserProvided {
				delete(resolvedOptionValues, childID)
			}
		}
	}

	return nil
}

func (r *Resolver) getValue(optionName string, option *types.Option, resolvedOptionValues map[string]config.OptionValue) (string, bool, config.OptionValue, bool, error) {
	// check if user value exists
	userValue, userValueOk := r.userOptions[optionName]

	// get before value
	beforeValue, beforeValueOk := resolvedOptionValues[optionName]

	// validate user value if we have one
	if userValueOk {
		err := validateUserValue(optionName, userValue, option)
		if err != nil {
			return "", false, config.OptionValue{}, false, err
		}
	}

	// validate existing value
	if beforeValueOk {
		err := validateUserValue(optionName, beforeValue.Value, option)
		if err != nil {
			// strip before value
			delete(resolvedOptionValues, optionName)
			beforeValue = config.OptionValue{}
			beforeValueOk = false
		}
	}

	return userValue, userValueOk, beforeValue, beforeValueOk, nil
}

func (r *Resolver) refreshSubOptions(
	ctx context.Context,
	optionName string,
	resolvedOptionValues map[string]config.OptionValue,
) error {
	option, ok := r.graph.GetNode(optionName)
	if !ok {
		return nil
	}

	if !r.resolveSubOptions || option.SubOptionsCommand == "" {
		return nil
	}

	_, ok = resolvedOptionValues[optionName]
	if !ok {
		return nil
	}

	// execute the command
	newDynamicOptions, err := resolveSubOptions(ctx, option, resolvedOptionValues, r.extraValues)
	if err != nil {
		return err
	}

	for childID := range r.getChangedOptions(r.dynamicOptionsForNode(resolvedOptionValues[optionName].Children), newDynamicOptions, resolvedOptionValues) {
		delete(resolvedOptionValues, childID)
		_ = r.graph.RemoveNode(childID)
	}

	// remove invalid existing user values
	for newOptionName, newOption := range newDynamicOptions {
		userValue, ok := r.userOptions[newOptionName]
		if !ok {
			continue
		}

		err := validateUserValue(newOptionName, userValue, newOption)
		if err != nil {
			delete(r.userOptions, newOptionName)
		}
	}

	// set children on value
	val := resolvedOptionValues[optionName]
	val.Children = []string{}
	for k := range newDynamicOptions {
		val.Children = append(val.Children, k)
	}
	resolvedOptionValues[optionName] = val

	// add options to graph
	err = addOptionsToGraph(r.graph, newDynamicOptions, resolvedOptionValues)
	if err != nil {
		return fmt.Errorf("add sub options %w", err)
	}

	err = resolveDynamicOptions(ctx, newDynamicOptions, r, resolvedOptionValues)
	if err != nil {
		return fmt.Errorf("resolve dynamic sub options %w", err)
	}

	return nil
}

type queue struct {
	items []string
	head  int
}

func newQueue(capacity int) *queue {
	return &queue{
		items: make([]string, 0, capacity),
		head:  0,
	}
}

func (q *queue) enqueue(item string) {
	q.items = append(q.items, item)
}

func (q *queue) dequeue() string {
	if q.head >= len(q.items) {
		return ""
	}
	item := q.items[q.head]
	q.head++
	return item
}

func (q *queue) isEmpty() bool {
	return q.head >= len(q.items)
}

func resolveDynamicOptions(ctx context.Context, options config.OptionDefinitions, r *Resolver, optionValues map[string]config.OptionValue) error {
	q := newQueue(len(options))
	processed := make(map[string]bool)

	for optionName := range options {
		q.enqueue(optionName)
	}

	for !q.isEmpty() {
		opt := q.dequeue()

		if processed[opt] {
			continue
		}
		processed[opt] = true

		if !r.graph.HasNode(opt) {
			continue
		}

		err := r.resolveOption(ctx, opt, optionValues)
		if err != nil {
			return fmt.Errorf("resolve dynamic option %s %w", opt, err)
		}

		subOptions, err := r.retrieveSubOptions(ctx, opt, optionValues)
		if err != nil {
			return fmt.Errorf("get sub options for %s %w", opt, err)
		}

		for optionName := range subOptions {
			if !processed[optionName] {
				q.enqueue(optionName)
			}
		}
	}
	return nil
}

func (r *Resolver) retrieveSubOptions(ctx context.Context, optionName string, options map[string]config.OptionValue) (config.OptionDefinitions, error) {
	option, ok := r.graph.GetNode(optionName)
	if !ok || !r.resolveSubOptions || option.SubOptionsCommand == "" {
		return nil, nil
	}

	_, ok = options[optionName]
	if !ok {
		return nil, nil
	}

	suboptions, err := resolveSubOptions(ctx, option, options, r.extraValues)
	if err != nil {
		return nil, err
	}

	for childID := range r.getChangedOptions(r.dynamicOptionsForNode(options[optionName].Children), suboptions, options) {
		delete(options, childID)
		_ = r.graph.RemoveNode(childID)
	}

	for name, option := range suboptions {
		userValue, ok := r.userOptions[name]
		if !ok {
			continue
		}
		err := validateUserValue(name, userValue, option)
		if err != nil {
			delete(r.userOptions, name)
		}
	}

	val := options[optionName]
	val.Children = []string{}
	for k := range suboptions {
		val.Children = append(val.Children, k)
	}
	options[optionName] = val

	err = addOptionsToGraph(r.graph, suboptions, options)
	if err != nil {
		return nil, fmt.Errorf("add sub options %w", err)
	}

	return suboptions, nil
}

func (r *Resolver) getChangedOptions(oldOptions config.OptionDefinitions, newOptions config.OptionDefinitions, resolvedOptionValues map[string]config.OptionValue) config.OptionDefinitions {
	changedOptions := config.OptionDefinitions{}
	for oldK, oldV := range oldOptions {
		_, ok := newOptions[oldK]
		if !ok {
			changedOptions[oldK] = oldV
			continue
		}
	}

	for newK, newV := range newOptions {
		oldV, ok := oldOptions[newK]
		if !ok {
			changedOptions[newK] = newV
			continue
		}

		oldValue, oldValueOk := resolvedOptionValues[newK]
		if !oldValueOk {
			changedOptions[newK] = newV
			continue
		}

		enumValues := []string{}
		for _, o := range newV.Enum {
			enumValues = append(enumValues, o.Value)
		}

		// check if value still valid
		if len(newV.Enum) > 0 && !contains(enumValues, oldValue.Value) {
			changedOptions[newK] = newV
			continue
		}

		// check if default has changed
		if !oldValue.UserProvided && oldV.Default != newV.Default {
			changedOptions[newK] = newV
			continue
		}
	}

	return changedOptions
}

func (r *Resolver) dynamicOptionsForNode(children []string) config.OptionDefinitions {
	retValues := config.OptionDefinitions{}
	for _, childID := range children {
		if option, ok := r.graph.GetNode(childID); ok {
			retValues[childID] = option
		}
	}

	return retValues
}

func contains(stack []string, k string) bool {
	return slices.Contains(stack, k)
}
