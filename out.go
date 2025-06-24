package main

import (
	"fmt"
	"io"
	"os"
)

// Write edges from a deduplicated map
func writeDotFileMap(edgeSet map[[2]string]struct{}, outPath string, root string, pretty bool) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create .dot file: %w", err)
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
	return nil
}

// Write a numbered list to a file, given a precomputed order
func writeNumberedListOrder(order []string, outPath string, root string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()
	for i, n := range order {
		fmt.Fprintf(f, "%2d. %s\n", i+1, relOrBase(root, n))
	}
	return nil
}

// Write a numbered list to an io.Writer, given a precomputed order
func writeNumberedListWriterOrder(order []string, w io.Writer, root string) error {
	for i, n := range order {
		fmt.Fprintf(w, "%2d. %s\n", i+1, relOrBase(root, n))
	}
	return nil
}
