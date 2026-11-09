// Command nativetests runs every native Go test package once. The integration
// package runs separately so its timing tests do not compete with other Go
// test packages and its cumulative timeout can cover the full suite.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const integration = "aego/tests/integration"

func packagesFromList(output string) ([]string, error) {
	packages := strings.Fields(output)
	seen := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		if pkg != "aego" && !strings.HasPrefix(pkg, "aego/") {
			return nil, fmt.Errorf("unexpected package %q", pkg)
		}
		if seen[pkg] {
			return nil, fmt.Errorf("duplicate package %q", pkg)
		}
		seen[pkg] = true
	}
	if !seen[integration] || len(packages) < 2 {
		return nil, errors.New("package list must include integration and other packages")
	}
	return packages, nil
}

// runTests attempts both groups, even if one fails. No test selector is used.
func runTests(packages []string, run func(...string) error) error {
	others := make([]string, 0, len(packages)-1)
	for _, pkg := range packages {
		if pkg != integration {
			others = append(others, pkg)
		}
	}
	args := append([]string{"test", "-count=1"}, others...)
	otherErr := run(args...)
	// The default Go timeout applies to the entire package, not each test. The
	// native macOS integration suite exceeded 10 minutes while still making
	// progress. Keep a finite 30-minute package timeout, matching the existing
	// integration race runner's per-group timeout.
	integrationErr := run("test", "-count=1", "-timeout=30m", integration)
	if otherErr != nil {
		otherErr = fmt.Errorf("other packages: %w", otherErr)
	}
	if integrationErr != nil {
		integrationErr = fmt.Errorf("integration package: %w", integrationErr)
	}
	return errors.Join(otherErr, integrationErr)
}

func main() {
	list := exec.Command("go", "list", "./...")
	list.Stderr = os.Stderr
	output, err := list.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "native test package enumeration:", err)
		os.Exit(1)
	}
	packages, err := packagesFromList(string(output))
	if err != nil {
		fmt.Fprintln(os.Stderr, "native test package enumeration:", err)
		os.Exit(1)
	}
	fmt.Printf("Native tests: %d other packages, then %s (30m timeout)\n", len(packages)-1, integration)
	err = runTests(packages, func(args ...string) error {
		cmd := exec.Command("go", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go %s: %w", strings.Join(args[:2], " "), err)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "native tests:", err)
		os.Exit(1)
	}
}
