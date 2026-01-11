package graph

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type GraphTestSuite struct {
	suite.Suite
	graph *Graph[string]
}

func (suite *GraphTestSuite) SetupTest() {
	suite.graph = NewGraph[string]()
}

func (suite *GraphTestSuite) TestAddNode() {
	err := suite.graph.AddNode("node1", "data1")
	suite.NoError(err)
	suite.Equal("data1", suite.graph.nodes["node1"])

	// Test duplicate node error
	err = suite.graph.AddNode("node1", "data2")
	suite.Error(err)
	suite.Contains(err.Error(), "node1 already exists")
	suite.Equal("data1", suite.graph.nodes["node1"])
}

func (suite *GraphTestSuite) TestAddNodes() {
	nodes := map[string]string{
		"node1": "data1",
		"node2": "data2",
		"node3": "data3",
	}

	err := suite.graph.AddNodes(nodes)
	suite.NoError(err)

	for id, expectedData := range nodes {
		suite.Equal(expectedData, suite.graph.nodes[id])
	}

	// Test duplicate node error
	duplicateNodes := map[string]string{
		"node1": "new_data1",
		"node4": "data4",
	}
	err = suite.graph.AddNodes(duplicateNodes)
	suite.Error(err)
	suite.Contains(err.Error(), "node1 already exists")
	suite.Equal("data1", suite.graph.nodes["node1"])
	suite.NotContains(suite.graph.nodes, "node4")
}

func (suite *GraphTestSuite) TestAddEdge() {
	suite.Require().NoError(suite.graph.AddNode("parent", "parent_data"))
	suite.Require().NoError(suite.graph.AddNode("child", "child_data"))

	err := suite.graph.AddEdge("parent", "child")
	suite.NoError(err)
	suite.Len(suite.graph.edges["parent"], 1)
	suite.Equal("child", suite.graph.edges["parent"][0])
}

func (suite *GraphTestSuite) TestTopologicalSort() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Require().NoError(suite.graph.AddEdge("A", "B"))

	result, err := suite.graph.Sort()
	suite.NoError(err)
	suite.Len(result, 2)
	suite.Equal("dataA", result[0])
	suite.Equal("dataB", result[1])
}

func (suite *GraphTestSuite) TestSortNodeIDs() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Require().NoError(suite.graph.AddEdge("A", "B"))

	result, err := suite.graph.SortNodeIDs()
	suite.NoError(err)
	suite.Len(result, 2)
	suite.Equal("A", result[0])
	suite.Equal("B", result[1])
}

func (suite *GraphTestSuite) TestCircularDependency() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Require().NoError(suite.graph.AddEdge("A", "B"))
	suite.Require().NoError(suite.graph.AddEdge("B", "A"))

	_, err := suite.graph.Sort()
	suite.Error(err)
}

func (suite *GraphTestSuite) TestRemoveNode() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Require().NoError(suite.graph.AddEdge("A", "B"))

	err := suite.graph.RemoveNode("B")
	suite.NoError(err)
	suite.False(suite.graph.HasNode("B"))
	suite.Len(suite.graph.GetChildren("A"), 0)
}

func (suite *GraphTestSuite) TestRemoveSubGraph() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Require().NoError(suite.graph.AddNode("C", "dataC"))
	suite.Require().NoError(suite.graph.AddEdge("A", "B"))
	suite.Require().NoError(suite.graph.AddEdge("B", "C"))

	err := suite.graph.RemoveSubGraph("B")
	suite.NoError(err)
	suite.False(suite.graph.HasNode("B"))
	suite.False(suite.graph.HasNode("C"))
	suite.True(suite.graph.HasNode("A"))
}

func (suite *GraphTestSuite) TestRemoveEdge() {
	testCases := []struct {
		name     string
		setup    func()
		from, to string
		wantErr  bool
		verify   func() bool
	}{
		{
			name: "remove existing edge",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
			},
			from: "A", to: "B",
			wantErr: false,
			verify: func() bool {
				return !suite.graph.HasEdge("A", "B") && suite.graph.EdgeCount() == 0
			},
		},
		{
			name: "remove non-existing edge",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
			},
			from: "A", to: "B",
			wantErr: false,
			verify: func() bool {
				return suite.graph.EdgeCount() == 0
			},
		},
		{
			name: "remove edge from non-existing source",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
			},
			from: "A", to: "B",
			wantErr: true,
			verify:  func() bool { return true },
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.setup()

			err := suite.graph.RemoveEdge(tc.from, tc.to)
			if tc.wantErr {
				suite.Error(err)
			} else {
				suite.NoError(err)
				suite.True(tc.verify())
			}
		})
	}
}

func (suite *GraphTestSuite) TestUpdateNode() {
	testCases := []struct {
		name    string
		setup   func()
		nodeID  string
		newData string
		wantErr bool
	}{
		{
			name: "update existing node",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("node1", "original"))
			},
			nodeID:  "node1",
			newData: "updated",
			wantErr: false,
		},
		{
			name:    "update non-existing node",
			setup:   func() {},
			nodeID:  "missing",
			newData: "data",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.setup()

			err := suite.graph.UpdateNode(tc.nodeID, tc.newData)
			if tc.wantErr {
				suite.Error(err)
			} else {
				suite.NoError(err)
				data, exists := suite.graph.GetNode(tc.nodeID)
				suite.True(exists)
				suite.Equal(tc.newData, data)
			}
		})
	}
}

func TestGraphSuite(t *testing.T) {
	suite.Run(t, new(GraphTestSuite))
}
func (suite *GraphTestSuite) TestEdgeCount() {
	testCases := []struct {
		name          string
		setup         func()
		expectedCount int
	}{
		{
			name:          "empty graph",
			setup:         func() {},
			expectedCount: 0,
		},
		{
			name: "nodes without edges",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
			},
			expectedCount: 0,
		},
		{
			name: "single edge",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
			},
			expectedCount: 1,
		},
		{
			name: "multiple edges",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddNode("C", "dataC"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
				suite.Require().NoError(suite.graph.AddEdge("A", "C"))
				suite.Require().NoError(suite.graph.AddEdge("B", "C"))
			},
			expectedCount: 3,
		},
		{
			name: "edges after removal",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddNode("C", "dataC"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
				suite.Require().NoError(suite.graph.AddEdge("A", "C"))
				suite.Require().NoError(suite.graph.RemoveEdge("A", "B"))
			},
			expectedCount: 1,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.setup()

			count := suite.graph.EdgeCount()
			suite.Equal(tc.expectedCount, count)
		})
	}
}
func (suite *GraphTestSuite) TestIsEmpty() {
	suite.True(suite.graph.IsEmpty())

	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.False(suite.graph.IsEmpty())

	suite.Require().NoError(suite.graph.RemoveNode("A"))
	suite.True(suite.graph.IsEmpty())
}

func (suite *GraphTestSuite) TestString() {
	suite.Equal("Graph: empty", suite.graph.String())

	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Require().NoError(suite.graph.AddEdge("A", "B"))

	output := suite.graph.String()
	suite.Contains(output, "Graph: 2 nodes, 1 edges")
	suite.Contains(output, "A -> [B]")
}
func (suite *GraphTestSuite) TestHasCircularDependency() {
	suite.False(suite.graph.HasCircularDependency())

	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.False(suite.graph.HasCircularDependency())

	suite.Require().NoError(suite.graph.AddEdge("A", "B"))
	suite.False(suite.graph.HasCircularDependency())

	suite.Require().NoError(suite.graph.AddEdge("B", "A"))
	suite.True(suite.graph.HasCircularDependency())
}
func (suite *GraphTestSuite) TestSetNode() {
	err := suite.graph.SetNode("new", "data")
	suite.NoError(err)
	suite.True(suite.graph.HasNode("new"))

	err = suite.graph.SetNode("new", "updated")
	suite.NoError(err)
	data, _ := suite.graph.GetNode("new")
	suite.Equal("updated", data)
}

func (suite *GraphTestSuite) TestGetChildren() {
	suite.Require().NoError(suite.graph.AddNode("parent", "p"))
	suite.Require().NoError(suite.graph.AddNode("child1", "c1"))
	suite.Require().NoError(suite.graph.AddNode("child2", "c2"))
	suite.Require().NoError(suite.graph.AddEdge("parent", "child1"))
	suite.Require().NoError(suite.graph.AddEdge("parent", "child2"))

	children := suite.graph.GetChildren("parent")
	suite.Len(children, 2)
	suite.Contains(children, "child1")
	suite.Contains(children, "child2")
}

func (suite *GraphTestSuite) TestGetParents() {
	suite.Require().NoError(suite.graph.AddNode("parent1", "p1"))
	suite.Require().NoError(suite.graph.AddNode("parent2", "p2"))
	suite.Require().NoError(suite.graph.AddNode("child", "c"))
	suite.Require().NoError(suite.graph.AddEdge("parent1", "child"))
	suite.Require().NoError(suite.graph.AddEdge("parent2", "child"))

	parents := suite.graph.GetParents("child")
	suite.Len(parents, 2)
	suite.Equal([]string{"parent1", "parent2"}, parents)
}

func (suite *GraphTestSuite) TestHasEdge() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))

	suite.False(suite.graph.HasEdge("A", "B"))

	suite.Require().NoError(suite.graph.AddEdge("A", "B"))
	suite.True(suite.graph.HasEdge("A", "B"))
	suite.False(suite.graph.HasEdge("B", "A"))
}

func (suite *GraphTestSuite) TestRemoveChildren() {
	suite.Require().NoError(suite.graph.AddNode("parent", "p"))
	suite.Require().NoError(suite.graph.AddNode("child1", "c1"))
	suite.Require().NoError(suite.graph.AddNode("child2", "c2"))
	suite.Require().NoError(suite.graph.AddNode("grandchild", "gc"))

	suite.Require().NoError(suite.graph.AddEdge("parent", "child1"))
	suite.Require().NoError(suite.graph.AddEdge("parent", "child2"))
	suite.Require().NoError(suite.graph.AddEdge("child1", "grandchild"))

	err := suite.graph.RemoveChildren("parent")
	suite.NoError(err)
	suite.True(suite.graph.HasNode("parent"))
	suite.False(suite.graph.HasNode("child1"))
	suite.False(suite.graph.HasNode("child2"))
	suite.False(suite.graph.HasNode("grandchild"))
}

func (suite *GraphTestSuite) TestNodeCount() {
	suite.Equal(0, suite.graph.NodeCount())

	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Equal(1, suite.graph.NodeCount())

	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
	suite.Equal(2, suite.graph.NodeCount())

	suite.Require().NoError(suite.graph.RemoveNode("A"))
	suite.Equal(1, suite.graph.NodeCount())
}

func (suite *GraphTestSuite) TestGetAllNodes() {
	suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
	suite.Require().NoError(suite.graph.AddNode("B", "dataB"))

	nodes := suite.graph.GetNodes()
	suite.Len(nodes, 2)
	suite.Equal("dataA", nodes["A"])
	suite.Equal("dataB", nodes["B"])
}
func (suite *GraphTestSuite) TestEdgeCases() {
	testCases := []struct {
		name string
		test func()
	}{
		{
			name: "duplicate edge prevention",
			test: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B")) // duplicate
				suite.Equal(1, suite.graph.EdgeCount())
			},
		},
		{
			name: "remove non-existing node",
			test: func() {
				err := suite.graph.RemoveNode("missing")
				suite.Error(err)
			},
		},
		{
			name: "get children of non-existing node",
			test: func() {
				children := suite.graph.GetChildren("missing")
				suite.Empty(children)
			},
		},
		{
			name: "get parents of non-existing node",
			test: func() {
				parents := suite.graph.GetParents("missing")
				suite.Empty(parents)
			},
		},
		{
			name: "remove edge to non-existing target",
			test: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				err := suite.graph.RemoveEdge("A", "missing")
				suite.Error(err)
			},
		},
		{
			name: "complex circular dependency",
			test: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddNode("C", "dataC"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
				suite.Require().NoError(suite.graph.AddEdge("B", "C"))
				suite.Require().NoError(suite.graph.AddEdge("C", "A")) // creates cycle
				suite.True(suite.graph.HasCircularDependency())
			},
		},
		{
			name: "self loop",
			test: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddEdge("A", "A"))
				suite.True(suite.graph.HasCircularDependency())
			},
		},
		{
			name: "remove subgraph of non-existing node",
			test: func() {
				err := suite.graph.RemoveSubGraph("missing")
				suite.NoError(err) // should not error
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.test()
		})
	}
}

func (suite *GraphTestSuite) TestLargeGraph() {
	nodeCount := 100

	for i := range nodeCount {
		suite.Require().NoError(suite.graph.AddNode(fmt.Sprintf("node%d", i), fmt.Sprintf("data%d", i)))
	}

	for i := 0; i < nodeCount-1; i++ {
		suite.Require().NoError(suite.graph.AddEdge(fmt.Sprintf("node%d", i), fmt.Sprintf("node%d", i+1)))
	}

	suite.Equal(nodeCount, suite.graph.NodeCount())
	suite.Equal(nodeCount-1, suite.graph.EdgeCount())
	suite.False(suite.graph.HasCircularDependency())

	result, err := suite.graph.Sort()
	suite.NoError(err)
	suite.Len(result, nodeCount)
}
func (suite *GraphTestSuite) TestTopologicalSortAdvanced() {
	testCases := []struct {
		name    string
		setup   func()
		wantErr bool
		verify  func([]string)
	}{
		{
			name:    "empty graph",
			setup:   func() {},
			wantErr: false,
			verify: func(result []string) {
				suite.Empty(result)
			},
		},
		{
			name: "diamond dependency",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddNode("C", "dataC"))
				suite.Require().NoError(suite.graph.AddNode("D", "dataD"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
				suite.Require().NoError(suite.graph.AddEdge("A", "C"))
				suite.Require().NoError(suite.graph.AddEdge("B", "D"))
				suite.Require().NoError(suite.graph.AddEdge("C", "D"))
			},
			wantErr: false,
			verify: func(result []string) {
				suite.Len(result, 4)
				suite.Equal("dataA", result[0])
				suite.Equal("dataD", result[3])
			},
		},
		{
			name: "three node cycle",
			setup: func() {
				suite.Require().NoError(suite.graph.AddNode("A", "dataA"))
				suite.Require().NoError(suite.graph.AddNode("B", "dataB"))
				suite.Require().NoError(suite.graph.AddNode("C", "dataC"))
				suite.Require().NoError(suite.graph.AddEdge("A", "B"))
				suite.Require().NoError(suite.graph.AddEdge("B", "C"))
				suite.Require().NoError(suite.graph.AddEdge("C", "A"))
			},
			wantErr: true,
			verify: func(result []string) {
				suite.Nil(result)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.setup()

			result, err := suite.graph.Sort()
			if tc.wantErr {
				suite.Error(err)
			} else {
				suite.NoError(err)
			}
			tc.verify(result)
		})
	}
}
