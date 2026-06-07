// paserati-analyze clusters Test262 failures by normalized error message.
// Reads a paserati-test262 -json envelope from stdin.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/test262"
)

func main() {
	var output test262.Output
	if err := json.NewDecoder(os.Stdin).Decode(&output); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding JSON from stdin: %v\n", err)
		fmt.Fprintf(os.Stderr, "Usage: paserati-test262 ... -json | paserati-analyze\n")
		os.Exit(1)
	}

	// Group failures by normalized error pattern.
	groups := make(map[string][]string)
	for _, result := range output.Results {
		if !result.Failed && !result.TimedOut {
			continue
		}
		msg := result.Error
		if msg == "" && result.TimedOut {
			msg = "Timeout"
		}
		pattern := normalizeError(msg)
		groups[pattern] = append(groups[pattern], result.Path)
	}

	type group struct {
		Pattern string
		Paths   []string
	}
	sorted := make([]group, 0, len(groups))
	for pattern, paths := range groups {
		sorted = append(sorted, group{Pattern: pattern, Paths: paths})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Paths) > len(sorted[j].Paths)
	})

	failures := len(output.Results) - output.Stats.Passed - output.Stats.Skipped
	fmt.Printf("Analysis of %d failures:\n", failures)
	fmt.Println("================================================================================")

	for _, g := range sorted {
		fmt.Printf("[%d] %s\n", len(g.Paths), g.Pattern)
		for i := 0; i < 3 && i < len(g.Paths); i++ {
			fmt.Printf("  - %s\n", g.Paths[i])
		}
		if len(g.Paths) > 3 {
			fmt.Printf("  ... and %d more\n", len(g.Paths)-3)
		}
		fmt.Println()
	}
}

func normalizeError(msg string) string {
	// Remove "PSXXXX [ERROR]: " prefix
	if idx := strings.Index(msg, "[ERROR]: "); idx != -1 {
		msg = msg[idx+9:]
	}
	// Remove "Uncaught exception: " prefix
	msg = strings.TrimPrefix(msg, "Uncaught exception: ")

	// Remove line numbers/columns: "at line 1, column 1"
	reLoc := regexp.MustCompile(`\s+at line \d+, column \d+`)
	msg = reLoc.ReplaceAllString(msg, "")

	// Truncate if too long
	if len(msg) > 100 {
		msg = msg[:97] + "..."
	}

	return strings.TrimSpace(msg)
}
