package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// execInOrder runs the given command in each directory in the provided order,
// honoring maxParallel AND dependency relationships
func execInOrder(order []string, edges []Edge, execCmd string, maxParallel int) error {
	// Build actual dependency map from the original edges
	dependencies := make(map[string]map[string]bool)
	dependents := make(map[string]map[string]bool)

	// Initialize maps
	for _, node := range order {
		dependencies[node] = make(map[string]bool)
		dependents[node] = make(map[string]bool)
	}

	// Add dependencies based on the Edge data
	for _, edge := range edges {
		dependencies[edge.Target][edge.Source] = true
		dependents[edge.Source][edge.Target] = true
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channels for coordination
	errCh := make(chan error, len(order))
	completed := make(chan string, len(order))
	ready := make(chan string, len(order))

	// Semaphore to limit concurrency
	sem := make(chan struct{}, maxParallel)

	// Track completed nodes
	completedNodes := make(map[string]bool)
	var completedMutex sync.Mutex

	// First, determine nodes with no dependencies (they can start immediately)
	for _, node := range order {
		if len(dependencies[node]) == 0 {
			ready <- node
		}
	}

	// Start a goroutine to process completions and determine what's ready next
	go func() {
		for node := range completed {
			completedMutex.Lock()
			completedNodes[node] = true

			// Check if any dependents are now ready
			for dependent := range dependents[node] {
				// Check if all dependencies of this dependent are completed
				allDepsComplete := true
				for dep := range dependencies[dependent] {
					if !completedNodes[dep] {
						allDepsComplete = false
						break
					}
				}

				if allDepsComplete {
					// This dependent is ready to execute
					ready <- dependent
				}
			}
			completedMutex.Unlock()
		}
	}()

	// Start workers to execute commands
	for i := 0; i < maxParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case dir, ok := <-ready:
					if !ok {
						return
					}

					// Acquire semaphore slot
					select {
					case sem <- struct{}{}:
						// We got a slot, proceed
					case <-ctx.Done():
						return
					}

					// Run in a separate goroutine to allow this worker to pick up new tasks
					go func(directory string) {
						defer func() { <-sem }() // Release semaphore

						// Check if already completed (could happen with race conditions)
						completedMutex.Lock()
						if completedNodes[directory] {
							completedMutex.Unlock()
							return
						}
						completedMutex.Unlock()

						// Execute the command
						fmt.Printf("[tforder] Running in %s: %s\n", directory, execCmd)
						cmd := exec.CommandContext(ctx, "/bin/sh", "-c", execCmd)
						cmd.Dir = directory
						out, err := cmd.CombinedOutput()
						fmt.Printf("[%s] Output:\n%s", filepath.Base(directory), out)

						if err != nil {
							fmt.Printf("[%s] Error: %v\n", filepath.Base(directory), err)
							errCh <- err
							cancel() // Cancel all ongoing operations
							return
						}

						// Mark as completed and notify
						completed <- directory
					}(dir)
				}
			}
		}()
	}

	// Close channels when appropriate
	go func() {
		for {
			// Exit if cancelled
			if ctx.Err() != nil {
				break
			}

			// Check if all nodes are completed
			completedMutex.Lock()
			allDone := true
			for _, node := range order {
				if !completedNodes[node] {
					allDone = false
					break
				}
			}
			completedMutex.Unlock()

			if allDone {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}

		// Close the ready channel to signal workers to exit
		close(ready)
	}()

	// Wait for all workers to finish
	wg.Wait()
	close(completed)

	// Return the first error that occurred, if any
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}
