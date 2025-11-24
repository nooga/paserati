package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Duplicate structs from paserati-test262
type TestStats struct {
	Total    int
	Passed   int
	Failed   int
	Timeouts int
	Skipped  int
	Duration time.Duration
}

type TestResult struct {
	Path     string        `json:"path"`
	Passed   bool          `json:"passed"`
	Failed   bool          `json:"failed"`
	TimedOut bool          `json:"timedOut"`
	Skipped  bool          `json:"skipped"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

type Output struct {
	Stats   TestStats    `json:"stats"`
	Results []TestResult `json:"results"`
}

func main() {
	var output Output
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&output); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding JSON input: %v\n", err)
		os.Exit(1)
	}

	// Group failures
	groups := make(map[string][]string) // pattern -> list of paths

	for _, result := range output.Results {
		if result.Failed || result.TimedOut {
			msg := result.Error
			if msg == "" && result.TimedOut {
				msg = "Timeout"
			}
			pattern := normalizeError(msg)
			groups[pattern] = append(groups[pattern], result.Path)
		}
	}

	// Sort patterns by count
	type Group struct {
		Pattern string
		Paths   []string
	}
	var sortedGroups []Group
	for pattern, paths := range groups {
		sortedGroups = append(sortedGroups, Group{Pattern: pattern, Paths: paths})
	}
	sort.Slice(sortedGroups, func(i, j int) bool {
		return len(sortedGroups[i].Paths) > len(sortedGroups[j].Paths)
	})

	fmt.Printf("Analysis of %d failures:\n", len(output.Results)-output.Stats.Passed-output.Stats.Skipped)
	fmt.Println("================================================================================")

	for _, g := range sortedGroups {
		fmt.Printf("[%d] %s\n", len(g.Paths), g.Pattern)
		// Print first 3 examples
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
