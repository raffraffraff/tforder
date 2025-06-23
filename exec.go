package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
)

// dagExec runs the given command in each directory in dependency order, honoring maxParallel
func dagExec(edges []Edge, execCmd string, maxParallel int) error {
	adj := map[string][]string{}
	inDegree := map[string]int{}
	nodes := map[string]struct{}{}
	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		inDegree[e.Target]++
		nodes[e.Source] = struct{}{}
		nodes[e.Target] = struct{}{}
	}
	ready := []string{}
	for n := range nodes {
		if inDegree[n] == 0 {
			ready = append(ready, n)
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	mu := sync.Mutex{}
	ctx := context.Background()
	done := map[string]struct{}{}
	errCh := make(chan error, 1)

	runNode := func(dir string) {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()
		fmt.Printf("[tforder] Running in %s: %s\n", dir, execCmd)
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", execCmd)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		fmt.Printf("[%s] Output:\n%s", filepath.Base(dir), out)
		if err != nil {
			fmt.Printf("[%s] Error: %v\n", filepath.Base(dir), err)
			errCh <- err
		}
		mu.Lock()
		done[dir] = struct{}{}
		mu.Unlock()
	}

	// Kahn's algorithm with parallel execution
	for len(ready) > 0 {
		next := []string{}
		wg.Add(len(ready))
		for _, node := range ready {
			go runNode(node)
			for _, neighbor := range adj[node] {
				inDegree[neighbor]--
				if inDegree[neighbor] == 0 {
					next = append(next, neighbor)
				}
			}
		}
		ready = next
		wg.Wait()
		select {
		case err := <-errCh:
			return err
		default:
		}
	}
	return nil
}
