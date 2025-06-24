package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
)

// execInOrder runs the given command in each directory in the provided order, honoring maxParallel
func execInOrder(order []string, execCmd string, maxParallel int) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)
	errCh := make(chan error, len(order)) // Buffer can hold all errors
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cancel is called to free resources

	for _, dir := range order {
		// If a command has already failed, don't start any new ones.
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // Block until a slot is available

		go func(d string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Double check for cancellation before running
			if ctx.Err() != nil {
				return
			}

			fmt.Printf("[tforder] Running in %s: %s\n", d, execCmd)
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", execCmd)
			cmd.Dir = d
			out, err := cmd.CombinedOutput()
			fmt.Printf("[%s] Output:\n%s", filepath.Base(d), out)
			if err != nil {
				fmt.Printf("[%s] Error: %v\n", filepath.Base(d), err)
				errCh <- err
				cancel() // Cancel the context to stop other commands
			}
		}(dir)
	}

	wg.Wait()
	close(errCh)

	// Return the first error that occurred, if any.
	return <-errCh
}
