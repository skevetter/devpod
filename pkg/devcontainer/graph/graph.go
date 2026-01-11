package graph

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

type Graph[T comparable] struct {
	nodes    map[string]T
	edges    map[string][]string
	inDegree map[string]int
}

func NewGraph[T comparable]() *Graph[T] {
	return &Graph[T]{
		nodes:    make(map[string]T),
		edges:    make(map[string][]string),
		inDegree: make(map[string]int),
	}
}

func (g *Graph[T]) AddNode(id string, data T) error {
	g.nodes[id] = data
	g.inDegree[id] = 0
	if g.edges[id] == nil {
		g.edges[id] = []string{}
	}
	return nil
}

func (g *Graph[T]) AddNodes(nodes map[string]T) error {
	for id, data := range nodes {
		g.nodes[id] = data
		g.inDegree[id] = 0
		if g.edges[id] == nil {
			g.edges[id] = []string{}
		}
	}
	return nil
}

func (g *Graph[T]) AddEdge(from, to string) error {
	if _, exists := g.nodes[from]; !exists {
		return fmt.Errorf("source node %s does not exist", from)
	}
	if _, exists := g.nodes[to]; !exists {
		return fmt.Errorf("target node %s does not exist", to)
	}

	if slices.Contains(g.edges[from], to) {
		return nil
	}

	g.edges[from] = append(g.edges[from], to)
	g.inDegree[to]++
	return nil
}

func (g *Graph[T]) RemoveEdge(from, to string) error {
	if _, exists := g.nodes[from]; !exists {
		return fmt.Errorf("source node %s does not exist", from)
	}
	if _, exists := g.nodes[to]; !exists {
		return fmt.Errorf("target node %s does not exist", to)
	}

	newEdges := []string{}
	edgeFound := false
	for _, child := range g.edges[from] {
		if child != to {
			newEdges = append(newEdges, child)
		} else {
			edgeFound = true
		}
	}

	if edgeFound {
		g.edges[from] = newEdges
		g.inDegree[to]--
	}

	return nil
}

func (g *Graph[T]) GetNode(id string) (T, bool) {
	node, exists := g.nodes[id]
	return node, exists
}

func (g *Graph[T]) UpdateNode(id string, data T) error {
	if _, exists := g.nodes[id]; !exists {
		return fmt.Errorf("node %s does not exist", id)
	}
	g.nodes[id] = data
	return nil
}

func (g *Graph[T]) SetNode(id string, data T) error {
	if g.HasNode(id) {
		return g.UpdateNode(id, data)
	}
	return g.AddNode(id, data)
}

func (g *Graph[T]) RemoveNode(id string) error {
	if _, exists := g.nodes[id]; !exists {
		return fmt.Errorf("node %s does not exist", id)
	}

	for parentID := range g.edges {
		filteredEdges := []string{}
		for _, childID := range g.edges[parentID] {
			if childID != id {
				filteredEdges = append(filteredEdges, childID)
			}
		}
		g.edges[parentID] = filteredEdges
	}

	delete(g.nodes, id)
	delete(g.edges, id)
	delete(g.inDegree, id)

	return nil
}

func (g *Graph[T]) GetChildren(id string) []string {
	return g.edges[id]
}

func (g *Graph[T]) GetParents(id string) []string {
	parents := []string{}
	for nodeID, children := range g.edges {
		if slices.Contains(children, id) {
			parents = append(parents, nodeID)
		}
	}
	sort.Strings(parents)
	return parents
}

func (g *Graph[T]) HasEdge(from, to string) bool {
	return slices.Contains(g.edges[from], to)
}

func (g *Graph[T]) RemoveSubGraph(id string) error {
	if _, exists := g.nodes[id]; !exists {
		return nil
	}

	nodesToRemove := []string{}
	g.collectChildren(id, &nodesToRemove)

	for _, nodeID := range nodesToRemove {
		g.RemoveNode(nodeID)
	}

	return nil
}

func (g *Graph[T]) RemoveChildren(id string) error {
	children := g.GetChildren(id)
	for _, childID := range children {
		err := g.RemoveSubGraph(childID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *Graph[T]) HasNode(id string) bool {
	_, exists := g.nodes[id]
	return exists
}

func (g *Graph[T]) NodeCount() int {
	return len(g.nodes)
}

func (g *Graph[T]) EdgeCount() int {
	totalEdges := 0
	for _, children := range g.edges {
		totalEdges += len(children)
	}
	return totalEdges
}

func (g *Graph[T]) IsEmpty() bool {
	return len(g.nodes) == 0
}

func (g *Graph[T]) GetNodes() map[string]T {
	return g.nodes
}

func (g *Graph[T]) String() string {
	if g.IsEmpty() {
		return "Graph: empty"
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Graph: %d nodes, %d edges\n", g.NodeCount(), g.EdgeCount()))

	sortedNodeIDs := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		sortedNodeIDs = append(sortedNodeIDs, id)
	}
	sort.Strings(sortedNodeIDs)

	for _, id := range sortedNodeIDs {
		children := g.GetChildren(id)
		if len(children) > 0 {
			output.WriteString(fmt.Sprintf("  %s -> %v\n", id, children))
		} else {
			output.WriteString(fmt.Sprintf("  %s\n", id))
		}
	}

	return output.String()
}

func (g *Graph[T]) Sort() ([]T, error) {
	workingInDegree := copyInDegreeMap(g.inDegree)
	zeroInDegreeQueue := initializeQueue(workingInDegree)
	sortedResult := make([]T, 0, len(g.nodes))

	for len(zeroInDegreeQueue) > 0 {
		currentNode := dequeue(&zeroInDegreeQueue)
		sortedResult = append(sortedResult, g.nodes[currentNode])
		processNeighbors(g.edges, currentNode, workingInDegree, &zeroInDegreeQueue)
	}

	if len(sortedResult) != len(g.nodes) {
		remainingNodes := []string{}
		for nodeID, degree := range workingInDegree {
			if degree > 0 {
				remainingNodes = append(remainingNodes, nodeID)
			}
		}
		sort.Strings(remainingNodes)
		return nil, fmt.Errorf("circular dependency detected among nodes: %v", remainingNodes)
	}

	return sortedResult, nil
}

func (g *Graph[T]) SortNodeIDs() ([]string, error) {
	workingInDegree := copyInDegreeMap(g.inDegree)
	zeroInDegreeQueue := initializeQueue(workingInDegree)
	sortedResult := make([]string, 0, len(g.nodes))

	for len(zeroInDegreeQueue) > 0 {
		currentNode := dequeue(&zeroInDegreeQueue)
		sortedResult = append(sortedResult, currentNode)
		processNeighbors(g.edges, currentNode, workingInDegree, &zeroInDegreeQueue)
	}

	if len(sortedResult) != len(g.nodes) {
		remainingNodes := []string{}
		for nodeID, degree := range workingInDegree {
			if degree > 0 {
				remainingNodes = append(remainingNodes, nodeID)
			}
		}
		sort.Strings(remainingNodes)
		return nil, fmt.Errorf("circular dependency detected among nodes: %v", remainingNodes)
	}

	return sortedResult, nil
}

func (g *Graph[T]) HasCircularDependency() bool {
	workingInDegree := copyInDegreeMap(g.inDegree)
	zeroInDegreeQueue := initializeQueue(workingInDegree)
	processedCount := 0

	for len(zeroInDegreeQueue) > 0 {
		currentNode := dequeue(&zeroInDegreeQueue)
		processedCount++
		processNeighbors(g.edges, currentNode, workingInDegree, &zeroInDegreeQueue)
	}

	return processedCount != len(g.nodes)
}

func (g *Graph[T]) collectChildren(id string, nodesToRemove *[]string) {
	*nodesToRemove = append(*nodesToRemove, id)
	for _, childID := range g.edges[id] {
		g.collectChildren(childID, nodesToRemove)
	}
}

func copyInDegreeMap(original map[string]int) map[string]int {
	copied := make(map[string]int, len(original))
	maps.Copy(copied, original)
	return copied
}

func initializeQueue(inDegree map[string]int) []string {
	zeroInDegreeNodes := make([]string, 0)
	for nodeID, degree := range inDegree {
		if degree == 0 {
			zeroInDegreeNodes = append(zeroInDegreeNodes, nodeID)
		}
	}
	sort.Strings(zeroInDegreeNodes)
	return zeroInDegreeNodes
}

func dequeue(queue *[]string) string {
	firstNode := (*queue)[0]
	*queue = (*queue)[1:]
	return firstNode
}

func processNeighbors(edges map[string][]string, currentNode string, inDegree map[string]int, queue *[]string) {
	for _, neighborNode := range edges[currentNode] {
		inDegree[neighborNode]--
		if inDegree[neighborNode] == 0 {
			insertSorted(queue, neighborNode)
		}
	}
}

func insertSorted(queue *[]string, nodeToInsert string) {
	insertPosition := sort.SearchStrings(*queue, nodeToInsert)
	*queue = append((*queue)[:insertPosition], append([]string{nodeToInsert}, (*queue)[insertPosition:]...)...)
}
