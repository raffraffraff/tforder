package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// collectEdges recursively collects dependency edges starting from dir
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

// parseDependencies parses the dependencies block from a main.tf file
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
