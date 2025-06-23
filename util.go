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

// Topological sort for numbered list output
func topoSort(edges []Edge) ([]string, error) {
	adj := map[string][]string{}
	inDegree := map[string]int{}
	nodes := map[string]struct{}{}
	for _, e := range edges {
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
