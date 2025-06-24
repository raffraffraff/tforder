package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func relOrBase(root, path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Base(path)
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return filepath.Base(absPath)
	}
	if rel == "." {
		return filepath.Base(absPath)
	}
	return rel
}

func escapeDotLabel(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// Topological sort for dependency ordering
func topoSort(edges []Edge, reverse bool) ([]string, error) {
	// If reverse is true, reverse the edges before doing the topological sort
	sortEdges := edges
	if reverse {
		sortEdges = reverseEdges(edges)
	}

	// Build adjacency list and track in-degrees
	adj := map[string][]string{}
	inDegree := map[string]int{}
	nodes := map[string]struct{}{}

	// Add all edges to the adjacency list
	for _, e := range sortEdges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		inDegree[e.Target]++
		nodes[e.Source] = struct{}{}
		nodes[e.Target] = struct{}{}
	}
	var order []string
	queue := []string{}
	for n := range nodes {
		if inDegree[n] == 0 {
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, m := range adj[n] {
			inDegree[m]--
			if inDegree[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, fmt.Errorf("cycle detected in dependency graph")
	}
	return order, nil
}

// reverseEdges creates a new slice of edges with Source and Target swapped
func reverseEdges(edges []Edge) []Edge {
	reversed := make([]Edge, len(edges))
	for i, e := range edges {
		reversed[i] = Edge{
			Source: e.Target,
			Target: e.Source,
		}
	}
	return reversed
}
