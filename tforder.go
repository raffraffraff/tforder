/*
tforder: Terraform dependency graph generator

Usage:
  tforder -d <start_dir> [-o <file.{txt|.dot|.svg|.png}>] [-r] [-relative-to <base>]

Flags:
  -d, -dir           Directory to start in (default: .)
  -o, -out           Output file (.txt, .dot, .svg, .png). If not specified, output is printed to stdout in numbered list format.
  -r, -recursive     Recursively scan all subdirectories for main.tf files
  -relative-to   Base path for relative node names (default: current working directory)

Examples:
  tforder -d tf/dev/eu-west-1/ew1a/eks
  tforder -d tf/dev/eu-west-1/ew1a/eks -o order.svg
  tforder -d tf -r -o infra.dot
  tforder -d tf -r -o infra.svg -relative-to tf
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Edge struct {
	Source string
	Target string
}

func main() {
	// Preprocess os.Args for short options
	shortToLong := map[string]string{
		"-d": "-dir",
		"-o": "-out",
		"-r": "-recursive",
		"-x": "-execute",
	}
	osArgs := os.Args[:1]
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if long, ok := shortToLong[arg]; ok {
			osArgs = append(osArgs, long)
		} else {
			osArgs = append(osArgs, arg)
		}
	}
	os.Args = osArgs

	dirPtr := flag.String("dir", ".", "-d, -dir  Directory to start in (default: .)")
	outPtr := flag.String("out", "", "-o, -out  Output file (.txt, .dot, .svg, .png). If not specified, output is printed to stdout in numbered list format.")
	relToPtr := flag.String("relative-to", "", "Base path for relative node names (default: current working directory)")
	recursivePtr := flag.Bool("recursive", false, "-r, -recursive  Recursively scan all subdirectories for main.tf files")
	execPtr := flag.String("execute", "", "-x, -execute  Script or command to execute in each dependency directory (optional)")
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

	// Calculate topological order once after edges are built
	order, err := topoSort(edges, *reversePtr)
	if err != nil {
		log.Fatalf("Failed to sort dependencies: %v", err)
	}

	if *execPtr != "" {
		fmt.Printf("Execution order (reverse=%v):\n", *reversePtr)
		for i, n := range order {
			fmt.Printf("%2d. %s\n", i+1, relOrBase(writeBase, n))
		}
		fmt.Printf("Executing '%s' in %s (max parallel: %d)\n", *execPtr, func() string {
			if *reversePtr {
				return "reverse dependency order"
			} else {
				return "dependency order"
			}
		}(), *maxParPtr)
		err = execInOrder(order, *execPtr, *maxParPtr)
		if err != nil {
			log.Fatalf("Execution failed: %v", err)
		}
		fmt.Println("All executions complete.")
		return
	}

	outFile := *outPtr
	pretty := strings.HasSuffix(outFile, ".svg") || strings.HasSuffix(outFile, ".png")
	isTxt := strings.HasSuffix(outFile, ".txt") || (!strings.HasSuffix(outFile, ".dot") && !pretty)

	if outFile == "" {
		err := writeNumberedListWriterOrder(order, os.Stdout, writeBase)
		if err != nil {
			log.Fatalf("%v", err)
		}
	} else if isTxt {
		err := writeNumberedListOrder(order, outFile, writeBase)
		if err != nil {
			log.Fatalf("%v", err)
		}
		fmt.Printf("Numbered list written: %s\n", outFile)
	} else if pretty {
		edgeSet := map[[2]string]struct{}{}
		for i := 0; i < len(order)-1; i++ {
			key := [2]string{order[i], order[i+1]}
			edgeSet[key] = struct{}{}
		}
		if _, err := exec.LookPath("dot"); err != nil {
			log.Fatalf("Error: 'dot' (Graphviz) is required to generate SVG/PNG output.")
		}
		tmpDot, err := os.CreateTemp("", "tforder-*.dot")
		if err != nil {
			log.Fatalf("Failed to create temp .dot file: %v", err)
		}
		tmpDotPath := tmpDot.Name()
		tmpDot.Close()
		err = writeDotFileMap(edgeSet, tmpDotPath, writeBase, pretty)
		if err != nil {
			log.Fatalf("%v", err)
		}
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
		err := writeDotFileMap(edgeSet, outFile, writeBase, pretty)
		if err != nil {
			log.Fatalf("%v", err)
		}
	}

	//fmt.Printf("Done. Edges: %d\n", len(edgeSet))
}
