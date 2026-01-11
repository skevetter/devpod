package resolver

import (
	"context"
	"testing"
	"time"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/graph"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
	"github.com/stretchr/testify/suite"
)

type ResolverTestSuite struct {
	suite.Suite
	resolver *Resolver
	logger   log.Logger
}

func (suite *ResolverTestSuite) SetupTest() {
	suite.logger = log.Default
	suite.resolver = New(map[string]string{}, map[string]string{}, suite.logger)
}

func TestResolverSuite(t *testing.T) {
	suite.Run(t, new(ResolverTestSuite))
}

func (suite *ResolverTestSuite) TestBasicFunctionality() {
	optionDefs := map[string]*types.Option{
		"option1": {
			Description: "Test option 1",
			Default:     "default1",
		},
		"option2": {
			Description: "Test option 2",
			Default:     "default2",
		},
	}

	optionValues := map[string]config.OptionValue{
		"option1": {Value: "value1"},
	}

	resolved, newDefs, err := suite.resolver.Resolve(context.Background(), nil, optionDefs, optionValues)
	suite.NoError(err)
	suite.NotNil(resolved)
	suite.NotNil(newDefs)
	suite.Equal("value1", resolved["option1"].Value)
	suite.Equal("default2", resolved["option2"].Value)
}

func (suite *ResolverTestSuite) TestGraphFunctionality() {
	optionDefs := map[string]*types.Option{
		"test": {
			Description: "Test option",
			Default:     "test_default",
		},
	}

	optionValues := map[string]config.OptionValue{}

	_, _, err := suite.resolver.Resolve(context.Background(), nil, optionDefs, optionValues)
	suite.NoError(err)

	suite.NotNil(suite.resolver.graph)
	suite.NotEmpty(suite.resolver.graph.GetNodes())
}

func (suite *ResolverTestSuite) TestResolveOptions_EmptyGraph() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Empty(result)
}

func (suite *ResolverTestSuite) TestResolveOptions_SingleOption() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()
	option := &types.Option{
		Description: "Test option",
		Default:     "default_value",
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("test_option", option))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Len(result, 1)
	suite.Equal("default_value", result["test_option"].Value)
}

func (suite *ResolverTestSuite) TestResolveOptions_UserProvidedValue() {
	userOptions := map[string]string{"test_option": "user_value"}
	suite.resolver = New(userOptions, map[string]string{}, suite.logger)
	suite.resolver.graph = graph.NewGraph[*types.Option]()

	option := &types.Option{
		Description: "Test option",
		Default:     "default_value",
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("test_option", option))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Equal("user_value", result["test_option"].Value)
	suite.True(result["test_option"].UserProvided)
}

func (suite *ResolverTestSuite) TestResolveOptions_EnumValue() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()
	option := &types.Option{
		Description: "Test enum option",
		Enum:        []types.OptionEnum{{Value: "only_choice"}},
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("enum_option", option))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Equal("only_choice", result["enum_option"].Value)
}

func (suite *ResolverTestSuite) TestResolveOptions_SkipGlobalOption() {
	suite.resolver.resolveGlobal = false
	suite.resolver.graph = graph.NewGraph[*types.Option]()
	option := &types.Option{
		Description: "Global option",
		Global:      true,
		Default:     "global_default",
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("global_option", option))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Empty(result)
}

func (suite *ResolverTestSuite) TestResolveOptions_SkipLocalOption() {
	suite.resolver.resolveLocal = false
	suite.resolver.graph = graph.NewGraph[*types.Option]()
	option := &types.Option{
		Description: "Local option",
		Local:       true,
		Default:     "local_default",
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("local_option", option))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Empty(result)
}

func (suite *ResolverTestSuite) TestResolveOptions_CachedValue() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()
	option := &types.Option{
		Description: "Cached option",
		Cache:       "1h",
		Default:     "new_default",
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("cached_option", option))

	now := types.NewTime(time.Now())
	existingValue := map[string]config.OptionValue{
		"cached_option": {
			Value:  "cached_value",
			Filled: &now,
		},
	}

	result, err := suite.resolver.resolveOptions(context.Background(), existingValue)
	suite.NoError(err)
	suite.Equal("cached_value", result["cached_option"].Value)
}

func (suite *ResolverTestSuite) TestResolveOptions_ExpiredCache() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()
	option := &types.Option{
		Description: "Cached option",
		Cache:       "1ns",
		Default:     "new_default",
	}
	suite.Require().NoError(suite.resolver.graph.AddNode("cached_option", option))

	pastTime := types.NewTime(time.Now().Add(-time.Hour))
	existingValue := map[string]config.OptionValue{
		"cached_option": {
			Value:  "old_cached_value",
			Filled: &pastTime,
		},
	}

	result, err := suite.resolver.resolveOptions(context.Background(), existingValue)
	suite.NoError(err)
	suite.Equal("new_default", result["cached_option"].Value)
}

func (suite *ResolverTestSuite) TestResolveOptions_PreserveChildWhenParentUnchanged() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()

	parentOption := &types.Option{Description: "Parent", Default: "new_parent_value"}
	childOption := &types.Option{Description: "Child", Default: "child_default"}

	suite.Require().NoError(suite.resolver.graph.AddNode("parent", parentOption))
	suite.Require().NoError(suite.resolver.graph.AddNode("child", childOption))
	suite.Require().NoError(suite.resolver.graph.AddEdge("parent", "child"))

	existingValues := map[string]config.OptionValue{
		"parent": {Value: "old_parent_value", UserProvided: true},
		"child":  {Value: "old_child_value", UserProvided: false},
	}

	result, err := suite.resolver.resolveOptions(context.Background(), existingValues)
	suite.NoError(err)

	suite.Equal("old_parent_value", result["parent"].Value)
	suite.Equal("old_child_value", result["child"].Value)
}

func (suite *ResolverTestSuite) TestResolveOptions_PreserveUserProvidedChild() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()

	parentOption := &types.Option{Description: "Parent", Default: "new_parent_value"}
	childOption := &types.Option{Description: "Child", Default: "child_default"}

	suite.Require().NoError(suite.resolver.graph.AddNode("parent", parentOption))
	suite.Require().NoError(suite.resolver.graph.AddNode("child", childOption))
	suite.Require().NoError(suite.resolver.graph.AddEdge("parent", "child"))

	existingValues := map[string]config.OptionValue{
		"parent": {Value: "old_parent_value", UserProvided: true},
		"child":  {Value: "user_child_value", UserProvided: true},
	}

	result, err := suite.resolver.resolveOptions(context.Background(), existingValues)
	suite.NoError(err)

	suite.Equal("old_parent_value", result["parent"].Value)
	suite.Equal("user_child_value", result["child"].Value)
}

func (suite *ResolverTestSuite) TestResolveOptions_CircularDependency() {
	suite.resolver.graph = graph.NewGraph[*types.Option]()

	option1 := &types.Option{Description: "Option 1"}
	option2 := &types.Option{Description: "Option 2"}

	suite.Require().NoError(suite.resolver.graph.AddNode("opt1", option1))
	suite.Require().NoError(suite.resolver.graph.AddNode("opt2", option2))
	suite.Require().NoError(suite.resolver.graph.AddEdge("opt1", "opt2"))
	suite.Require().NoError(suite.resolver.graph.AddEdge("opt2", "opt1"))

	_, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.Error(err)
	suite.Contains(err.Error(), "circular dependency")
}

func (suite *ResolverTestSuite) TestCombineFunction() {
	existing := map[string]config.OptionValue{
		"key1": {Value: "existing_value1"},
		"key2": {Value: "existing_value2"},
	}

	extra := map[string]string{
		"key2": "extra_value2",
		"key3": "extra_value3",
	}

	result := combine(existing, extra)

	suite.Equal("existing_value1", result["key1"])
	suite.Equal("existing_value2", result["key2"], "existing should override extra")
	suite.Equal("extra_value3", result["key3"])
}

func (suite *ResolverTestSuite) TestSubOptionsBasicFunctionality() {
	logger := log.Default
	resolver := New(map[string]string{}, map[string]string{}, logger, WithResolveSubOptions())

	optionDefs := map[string]*types.Option{
		"parent": {
			Default: "parent_value",
		},
	}

	optionValues := map[string]config.OptionValue{}

	resolved, _, err := resolver.Resolve(context.Background(), nil, optionDefs, optionValues)
	suite.NoError(err)
	suite.Equal("parent_value", resolved["parent"].Value)
}

func (suite *ResolveTestSuite) TestAddOptionsToGraph_MultipleCalls() {
	g := graph.NewGraph[*types.Option]()

	optionDefs := config.OptionDefinitions{
		"opt1": &types.Option{Description: "Option 1"},
	}

	suite.Require().NoError(addOptionsToGraph(g, optionDefs, map[string]config.OptionValue{}))
	suite.Require().NoError(addOptionsToGraph(g, optionDefs, map[string]config.OptionValue{}))

	nodes := g.GetNodes()
	suite.Len(nodes, 2, "Multiple calls to addOptionsToGraph should not duplicate nodes.")
}
