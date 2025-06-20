/*
tforder: Terraform dependency graph generator

Usage:
  tforder -dir <start_dir> [-out <file.dot|file.svg|file.png>] [-relative-to <base>] [-recursive]

Flags:
  -dir           Directory to start in (default: .)
  -out           Output file (.dot, .svg, .png; default: tforder.dot)
  -relative-to   Base path for relative node names (default: current working directory)
  -recursive     Recursively scan all subdirectories for main.tf files

Examples:
  tforder -dir tf/dev/eu-west-1/ew1a/eks -out graph.svg
  tforder -dir tf -recursive -out infra.dot
  tforder -dir tf -recursive -out infra.svg -relative-to tf
*/

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type Edge struct {
	Source string
	Target string
}

func main() {
	dirPtr := flag.String("dir", ".", "Directory to start in")
	outPtr := flag.String("out", "tforder.dot", "Output .dot/.svg/.png file")
	relToPtr := flag.String("relative-to", "", "Base path for relative node names (default: current working directory)")
	recursivePtr := flag.Bool("recursive", false, "Recursively scan all subdirectories for main.tf files")
	execPtr := flag.String("execute", "", "Script or command to execute in each dependency directory (optional)")
	maxParPtr := flag.Int("maxparallel", 2, "Maximum number of parallel executions (default 2)")
	reversePtr := flag.Bool("reverse", false, "Reverse dependency order (for destroy operations)")
	flag.Parse()

	startDir, _ := filepath.Abs(*dirPtr)
	info, err := os.Stat(startDir)
	if err != nil {
		log.Fatalf("Start directory does not exist: %v", err)
	}
	if *recursivePtr && !info.IsDir() {
		log.Fatalf("-recursive can only be used with a directory path")
	}

	relBase := *relToPtr
	if relBase == "" {
		relBase, _ = os.Getwd()
	}
	writeBase, _ := filepath.Abs(relBase)

	edgeSet := map[[2]string]struct{}{}
	edges := []Edge{}
	mainTfCount := 0

	if *recursivePtr {
		err := filepath.Walk(startDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				mainTf := filepath.Join(path, "main.tf")
				if _, err := os.Stat(mainTf); err == nil {
					mainTfAbs, _ := filepath.Abs(path)
					mainTfCount++
					deps := parseDependencies(mainTf)
					for _, relPath := range deps {
						targetDir, _ := filepath.Abs(filepath.Join(path, relPath))
						key := [2]string{targetDir, mainTfAbs}
						if _, exists := edgeSet[key]; !exists {
							edgeSet[key] = struct{}{}
							edges = append(edges, Edge{Source: targetDir, Target: mainTfAbs})
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			log.Fatalf("Error walking directory: %v", err)
		}
		if mainTfCount == 0 {
			log.Fatalf("No main.tf files found under %s", startDir)
		}
	} else {
		visited := map[string]bool{}
		collectEdges(startDir, &edges, visited)
		for _, e := range edges {
			key := [2]string{e.Source, e.Target}
			if _, exists := edgeSet[key]; !exists {
				edgeSet[key] = struct{}{}
			}
		}
	}

	if *reversePtr {
		for i := range edges {
			edges[i].Source, edges[i].Target = edges[i].Target, edges[i].Source
		}
	}

	if *execPtr != "" {
		order := "dependency order"
		if *reversePtr {
			order = "reverse dependency order"
		}
		fmt.Printf("Executing '%s' in %s (max parallel: %d)\n", *execPtr, order, *maxParPtr)
		err := dagExec(edges, *execPtr, *maxParPtr)
		if err != nil {
			log.Fatalf("Execution failed: %v", err)
		}
		fmt.Println("All executions complete.")
		return
	}

	outFile := *outPtr
	pretty := strings.HasSuffix(outFile, ".svg") || strings.HasSuffix(outFile, ".png")

	if pretty {
		if _, err := exec.LookPath("dot"); err != nil {
			log.Fatalf("Error: 'dot' (Graphviz) is required to generate SVG/PNG output.")
		}
		tmpDot, err := ioutil.TempFile("", "tforder-*.dot")
		if err != nil {
			log.Fatalf("Failed to create temp .dot file: %v", err)
		}
		tmpDotPath := tmpDot.Name()
		// tmpDot.Close() and defer os.Remove(tmpDotPath) removed for debugging
		tmpDot.Close()
		writeDotFileMap(edgeSet, tmpDotPath, writeBase, pretty)
		format := "svg"
		if strings.HasSuffix(outFile, ".png") {
			format = "png"
		}
		cmd := exec.Command("dot", "-T"+format, tmpDotPath, "-o", outFile)
		if err := cmd.Run(); err != nil {
			log.Fatalf("Failed to generate %s: %v\nTemp .dot file: %s", format, err, tmpDotPath)
		}
		fmt.Printf("%s generated: %s\nTemp .dot file: %s\n", strings.ToUpper(format), outFile, tmpDotPath)
	} else {
		writeDotFileMap(edgeSet, outFile, writeBase, pretty)
	}

	fmt.Printf("Done. Edges: %d\n", len(edgeSet))
}

// uniqueNodes returns all unique directories from edges
func uniqueNodes(edges []Edge) []string {
	nodes := map[string]struct{}{}
	for _, e := range edges {
		nodes[e.Source] = struct{}{}
		nodes[e.Target] = struct{}{}
	}
	res := make([]string, 0, len(nodes))
	for n := range nodes {
		res = append(res, n)
	}
	return res
}

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

func collectEdges(dir string, edges *[]Edge, visited map[string]bool) {
	if visited[dir] {
		return
	}
	visited[dir] = true

	mainTf := filepath.Join(dir, "main.tf")
	deps := parseDependencies(mainTf)
	for _, relPath := range deps {
		targetDir := filepath.Clean(filepath.Join(dir, relPath))
		*edges = append(*edges, Edge{Source: targetDir, Target: dir}) // keep absolute
		collectEdges(targetDir, edges, visited)
	}
}

func parseDependencies(tfPath string) map[string]string {
	file, err := os.Open(tfPath)
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()

	deps := map[string]string{}
	inLocals := false
	inDeps := false
	depRe := regexp.MustCompile(`(?i)\s*([a-zA-Z0-9_\-]+)\s*=\s*"([^"]+)"`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "locals") && strings.Contains(line, "{") {
			inLocals = true
			continue
		}
		if inLocals && strings.HasPrefix(line, "dependencies") && strings.Contains(line, "{") {
			inDeps = true
			continue
		}
		if inDeps {
			if strings.Contains(line, "}") {
				inDeps = false
				continue
			}
			matches := depRe.FindStringSubmatch(line)
			if len(matches) == 3 {
				deps[matches[1]] = matches[2]
			}
		}
		if inLocals && !inDeps && strings.Contains(line, "}") {
			inLocals = false
		}
	}
	return deps
}

func writeDotFile(edges []Edge, outPath string, root string) {
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create .dot file: %v\n", err)
		return
	}
	defer f.Close()

	pretty := strings.HasSuffix(outPath, ".svg") || strings.HasSuffix(outPath, ".png")

	fmt.Fprintln(f, "digraph tforder {")
	if pretty {
		fmt.Fprintln(f, `  rankdir=LR;`)
		fmt.Fprintln(f, `  node [shape=box, style=filled, fillcolor="#e3f2fd", fontname=Helvetica, color="#1976d2"];`)
		fmt.Fprintln(f, `  edge [color="#1976d2", penwidth=2, arrowsize=0.8];`)
		fmt.Fprintln(f, `  graph [splines=true, bgcolor="#fafafa"];`)
	}
	for _, e := range edges {
		src := relOrBase(root, e.Source)
		tgt := relOrBase(root, e.Target)
		src = escapeDotLabel(src)
		tgt = escapeDotLabel(tgt)
		fmt.Fprintf(f, "  \"%s\" -> \"%s\";\n", src, tgt)
	}
	fmt.Fprintln(f, "}")
}

// Write edges from a deduplicated map
func writeDotFileMap(edgeSet map[[2]string]struct{}, outPath string, root string, pretty bool) {
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("Failed to create .dot file: %v", err)
	}
	defer f.Close()
	fmt.Fprintln(f, "digraph tforder {")
	if pretty {
		fmt.Fprintln(f, `  rankdir=LR;`)
		fmt.Fprintln(f, `  node [shape=box, style=filled, fillcolor="#e3f2fd", fontname=Helvetica, color="#1976d2"];`)
		fmt.Fprintln(f, `  edge [color="#1976d2", penwidth=2, arrowsize=0.8];`)
		fmt.Fprintln(f, `  graph [splines=true, bgcolor="#fafafa"];`)
	}
	for k := range edgeSet {
		src := relOrBase(root, k[0])
		tgt := relOrBase(root, k[1])
		fmt.Fprintf(f, "  \"%s\" -> \"%s\";\n", escapeDotLabel(src), escapeDotLabel(tgt))
	}
	fmt.Fprintln(f, "}")
}

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
