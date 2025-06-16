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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

