package resolver

import (
	"context"
	"testing"

	"github.com/skevetter/devpod/pkg/config"
	"github.com/skevetter/devpod/pkg/devcontainer/graph"
	"github.com/skevetter/devpod/pkg/types"
	"github.com/skevetter/log"
	"github.com/stretchr/testify/suite"
)

type ResolveTestSuite struct {
	suite.Suite
	resolver *Resolver
}

func (suite *ResolveTestSuite) SetupTest() {
	suite.resolver = &Resolver{
		graph:       graph.NewGraph[*types.Option](),
		userOptions: make(map[string]string),
		extraValues: make(map[string]string),
		log:         log.Default,
	}
}

func TestResolveTestSuite(t *testing.T) {
	suite.Run(t, new(ResolveTestSuite))
}

func (suite *ResolveTestSuite) TestResolveOptions_EmptyGraph() {
	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Empty(result)
}

func (suite *ResolveTestSuite) TestResolveOptions_NodeExistenceCheck() {
	suite.Require().NoError(suite.resolver.graph.AddNode("test", &types.Option{}))
	suite.Require().NoError(suite.resolver.graph.RemoveNode("test"))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Empty(result)
}

func (suite *ResolveTestSuite) TestResolveOptions_WithDependencies() {
	option1 := &types.Option{Default: "value1"}
	option2 := &types.Option{Default: "value2"}

	suite.Require().NoError(suite.resolver.graph.AddNode("option1", option1))
	suite.Require().NoError(suite.resolver.graph.AddNode("option2", option2))
	suite.Require().NoError(suite.resolver.graph.AddEdge("option1", "option2"))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Len(result, 2)
	suite.Equal("value1", result["option1"].Value)
	suite.Equal("value2", result["option2"].Value)
}
func (suite *ResolveTestSuite) TestResolveOptions_MultipleNodes() {
	nodes := map[string]*types.Option{
		"option1": {Default: "value1"},
		"option2": {Default: "value2"},
		"option3": {Default: "value3"},
	}

	suite.Require().NoError(suite.resolver.graph.AddNodes(nodes))

	result, err := suite.resolver.resolveOptions(context.Background(), map[string]config.OptionValue{})
	suite.NoError(err)
	suite.Len(result, 3)
	suite.Equal("value1", result["option1"].Value)
	suite.Equal("value2", result["option2"].Value)
	suite.Equal("value3", result["option3"].Value)
}
