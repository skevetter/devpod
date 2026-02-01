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

// AddNode adds a node to the graph with the given ID and data.
// Returns an error if a node with the same ID already exists.
func (g *Graph[T]) AddNode(id string, data T) error {
	if _, exists := g.nodes[id]; exists {
		return fmt.Errorf("node %s already exists", id)
	}
	g.nodes[id] = data
	g.inDegree[id] = 0
	g.edges[id] = []string{}
	return nil
}

// AddNodes adds multiple nodes to the graph.
// Returns an error if any node with the same ID already exists.
func (g *Graph[T]) AddNodes(nodes map[string]T) error {
	for id := range nodes {
		if _, exists := g.nodes[id]; exists {
			return fmt.Errorf("node %s already exists", id)
		}
	}
	for id, data := range nodes {
		g.nodes[id] = data
		g.inDegree[id] = 0
		g.edges[id] = []string{}
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

	oldLen := len(g.edges[from])
	g.edges[from] = slices.DeleteFunc(g.edges[from], func(child string) bool {
		return child == to
	})
	if len(g.edges[from]) < oldLen && g.inDegree[to] > 0 {
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

	for _, childID := range g.edges[id] {
		if g.inDegree[childID] > 0 {
			g.inDegree[childID]--
		}
	}

	for parentID := range g.edges {
		g.edges[parentID] = slices.DeleteFunc(g.edges[parentID], func(childID string) bool {
			return childID == id
		})
	}

	delete(g.nodes, id)
	delete(g.edges, id)
	delete(g.inDegree, id)

	return nil
}

func (g *Graph[T]) GetChildren(id string) []string {
	children := g.edges[id]
	if children == nil {
		return []string{}
	}
	result := make([]string, len(children))
	copy(result, children)
	return result
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
		if err := g.RemoveNode(nodeID); err != nil {
			return err
		}
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
	result := make(map[string]T, len(g.nodes))
	maps.Copy(result, g.nodes)
	return result
}

func (g *Graph[T]) String() string {
	if g.IsEmpty() {
		return "Graph: empty"
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Graph: %d nodes, %d edges\n", g.NodeCount(), g.EdgeCount())

	sortedNodeIDs := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		sortedNodeIDs = append(sortedNodeIDs, id)
	}
	sort.Strings(sortedNodeIDs)

	for _, id := range sortedNodeIDs {
		children := g.GetChildren(id)
		if len(children) > 0 {
			fmt.Fprintf(&output, "  %s -> %v\n", id, children)
		} else {
			fmt.Fprintf(&output, "  %s\n", id)
		}
	}

	return output.String()
}

func (g *Graph[T]) Sort() ([]T, error) {
	sortedIDs, err := g.sortNodeIDs()
	if err != nil {
		return nil, err
	}
	result := make([]T, len(sortedIDs))
	for i, id := range sortedIDs {
		result[i] = g.nodes[id]
	}
	return result, nil
}

func (g *Graph[T]) SortNodeIDs() ([]string, error) {
	return g.sortNodeIDs()
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

func (g *Graph[T]) sortNodeIDs() ([]string, error) {
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

func (g *Graph[T]) collectChildren(id string, nodesToRemove *[]string) {
	stack := []string{id}
	visited := make(map[string]bool)

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if visited[current] {
			continue
		}
		visited[current] = true

		*nodesToRemove = append(*nodesToRemove, current)
		for _, childID := range g.edges[current] {
			if !visited[childID] {
				stack = append(stack, childID)
			}
		}
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
			insertPosition := sort.SearchStrings(*queue, neighborNode)
			*queue = slices.Insert(*queue, insertPosition, neighborNode)
		}
	}
}
