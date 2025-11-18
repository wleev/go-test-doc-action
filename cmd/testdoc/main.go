package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

/*** CLI flags ***/
var (
	sourceDir      string
	outPath        string
	junitPath      string
	failSnippetMax int
)

type TestSuite struct {
	PackageName   string
	Name          string
	CommentHeader string
	TestUnits     []TestUnit
}

type TestUnit struct {
	CommentHeader   string
	MachineTestName string
	TestName        string
	Subtests        []TestUnit
}

type ExpandedVar struct {
	VarName  string
	VarValue []string
}

const MAX_GAP_SIZE = 10

func main() {
	flag.StringVar(&sourceDir, "source", ".", "source directory to scan for tests")
	flag.StringVar(&outPath, "o", "TESTS.md", "output markdown file path")
	flag.StringVar(&junitPath, "junit", "", "path to JUnit XML (required)")
	flag.IntVar(&failSnippetMax, "fail-snippet", 300, "max chars of failure message to include (0=hide)")
	flag.Parse()

	if junitPath == "" {
		fmt.Fprintln(os.Stderr, "error: -junit path is required (provide JUnit XML from a previous step)")
		os.Exit(1)
	}

	// 1) Gather static docs from source
	testSuites, err := ParseTestSuites(sourceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing test functions: %v\n", err)
		os.Exit(1)
	}

	// print test units for , recursively
	var printTestFunc func(tu TestUnit, indent string)

	printTestFunc = func(tu TestUnit, indent string) {
		fmt.Printf("%sTest: %s\n", indent, tu.TestName)
		fmt.Printf("%sMachine Name: %s\n", indent, tu.MachineTestName)
		if tu.CommentHeader != "" {
			fmt.Printf("%sComments:\n%s\n", indent, tu.CommentHeader)
		}
		for _, sub := range tu.Subtests {
			printTestFunc(sub, indent+"  ")
		}
	}

	for _, ts := range testSuites {
		fmt.Printf("Package: %s\n", ts.PackageName)
		fmt.Printf("Suite: %s\n", ts.Name)
		if ts.CommentHeader != "" {
			fmt.Printf("Comments:\n%s\n", ts.CommentHeader)
		}
		for _, tu := range ts.TestUnits {
			printTestFunc(tu, "")
		}
	}

	// 2) Read JUnit XML and attach statuses/durations/failures
	jmap, err := ParseJUnitResults(junitPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: reading junit: %v\n", err)
	}

	// 3) Generate markdown report
	err = GenerateMarkdownReport(testSuites, jmap, outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating markdown report: %v\n", err)
		os.Exit(1)
	}
}

/*** Package scanning (AST for summaries/tags/subtests) ***/
func ParseTestSuites(sourceDir string) ([]TestSuite, error) {
	cfg := &packages.Config{
		Dir:        sourceDir,
		Mode:       packages.NeedName | packages.NeedFiles | packages.NeedModule | packages.NeedForTest,
		Tests:      true,
		Env:        nil,
		Fset:       nil,
		BuildFlags: nil,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}

	var all []TestSuite

	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if len(p.Errors) > 0 {
			for _, e := range p.Errors {
				fmt.Fprintf(os.Stderr, "warn: package %s: %v\n", p.PkgPath, e)
			}
		}

		if strings.HasSuffix(p.Name, "_test") {
			for _, filePath := range p.GoFiles {
				fileSet := token.NewFileSet()
				node, err := parser.ParseFile(fileSet, filePath, nil, parser.ParseComments)
				if err != nil {
					fmt.Fprintf(os.Stderr, "parse error %s: %v\n", filePath, err)
					continue
				}

				// Create one test suite per file
				var testUnits []TestUnit

				ast.Inspect(node, func(n ast.Node) bool {
					fd, ok := n.(*ast.FuncDecl)
					if !ok || fd.Recv != nil || fd.Name == nil {
						return true
					}
					name := fd.Name.Name
					if !strings.HasPrefix(name, "Test") {
						return true
					}

					file := fileSet.File(fd.End())

					subs := CollectSubtests(fd.Body, node.Comments, file, filePath, name, nil)

					// Create a test unit for this function
					testUnits = append(testUnits, TestUnit{
						CommentHeader:   FindRelativeComment(fd.Pos(), node.Comments, file, filePath),
						MachineTestName: name,
						TestName:        name,
						Subtests:        subs,
					})

					return true // Continue to find more test functions
				})

				// Only add suite if we found test functions
				if len(testUnits) > 0 {
					all = append(all, TestSuite{
						PackageName:   strings.TrimSuffix(p.PkgPath, "_test"), // to match junit output
						Name:          filepath.Base(filePath),
						CommentHeader: "",
						TestUnits:     testUnits,
					})
				}
			}
		}
	})

	return all, nil
}

func CollectSubtests(testBody *ast.BlockStmt, comments []*ast.CommentGroup, file *token.File, filePath string, parentName string, expandedVariables []ExpandedVar) []TestUnit {
	tests := make([]TestUnit, 0)
	var precedingComments string

	ast.Inspect(testBody, func(n ast.Node) bool {
		loop, ok := n.(*ast.RangeStmt)
		if ok {
			rangeValues := ExtractRangeValues(loop)

			var loopVarName string
			if loop.Value != nil {
				if ident, ok := loop.Value.(*ast.Ident); ok {
					loopVarName = ident.Name
				}
			}

			expandedVariables = append(expandedVariables, ExpandedVar{VarName: loopVarName, VarValue: rangeValues})

			ast.Inspect(loop.Body, func(innerN ast.Node) bool {
				if call, ok := innerN.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel != nil && sel.Sel.Name == "Run" {
						if len(call.Args) >= 2 {
							precedingComments = FindRelativeComment(call.Pos(), comments, file, filePath)

							// Expand test names for each loop value
							expandedNames := ExpandTestName(call.Args[0], expandedVariables)
							if testFunc, ok := call.Args[1].(*ast.FuncLit); ok {
								// Create a test unit for each expanded name
								for _, expandedName := range expandedNames {
									subtests := CollectSubtests(testFunc.Body, comments, file, filePath, fmt.Sprintf("%s/%s", parentName, expandedName), expandedVariables)
									tests = append(tests, TestUnit{
										CommentHeader:   precedingComments,
										MachineTestName: strings.ReplaceAll(fmt.Sprintf("%s/%s", parentName, expandedName), " ", "_"),
										TestName:        expandedName,
										Subtests:        subtests,
									})
								}
							}
						}
						return false
					}
				}
				return true
			})

			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "Run" {
			return true
		}

		if len(call.Args) == 0 {
			return true
		}

		precedingComments = FindRelativeComment(call.Pos(), comments, file, filePath)

		testNames := ExpandTestName(call.Args[0], expandedVariables)

		if testFunc, ok := call.Args[1].(*ast.FuncLit); ok && len(testNames) > 0 {
			for _, testName := range testNames {
				unitName := fmt.Sprintf("%s/%s", parentName, testName)
				subtests := CollectSubtests(testFunc.Body, comments, file, filePath, unitName, expandedVariables)
				tests = append(tests, TestUnit{
					CommentHeader:   precedingComments,
					MachineTestName: strings.ReplaceAll(unitName, " ", "_"),
					TestName:        testName,
					Subtests:        subtests,
				})
			}
		}

		return false
	})
	return tests
}

func ExtractRangeValues(rangeStmt *ast.RangeStmt) []string {
	var values []string

	if comp, ok := rangeStmt.X.(*ast.CompositeLit); ok {
		for _, elt := range comp.Elts {
			if lit, ok := elt.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					values = append(values, s)
				}
			} else {
				// For non-string literals, convert to string representation
				var sb strings.Builder
				if err := format.Node(&sb, token.NewFileSet(), elt); err == nil {
					values = append(values, sb.String())
				}
			}
		}
	}

	return values
}

// ExpandTestName substitutes the loop variable with a specific value
func ExpandTestName(expr ast.Expr, expandedVariables []ExpandedVar) []string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		// Simple string literal - return as is
		if e.Kind == token.STRING {
			if s, err := strconv.Unquote(e.Value); err == nil {
				return []string{s}
			}
		}
		return []string{e.Value}

	case *ast.BinaryExpr:
		// Handle string concatenation like "test"+input
		if e.Op == token.ADD {
			left := ExpandTestName(e.X, expandedVariables)
			right := ExpandTestName(e.Y, expandedVariables)
			// mix all combinations
			var results []string
			for _, l := range left {
				for _, r := range right {
					results = append(results, l+r)
				}
			}
			return results
		}

	case *ast.Ident:
		// Check if this ident matches any expanded variable
		for _, ev := range expandedVariables {
			if e.Name == ev.VarName {
				return ev.VarValue
			}
		}
		return []string{e.Name}
	}

	// Fallback: convert entire expression to string
	var sb strings.Builder
	if err := format.Node(&sb, token.NewFileSet(), expr); err == nil {
		// Simple string replacement as fallback
		return []string{sb.String()}
	}

	return []string{"unknown"}
}

func FindRelativeComment(callPos token.Pos, comments []*ast.CommentGroup, file *token.File, filePath string) string {
	precedingComments := ""
	for _, commentGroup := range comments {
		// Check if comment is immediately before the statement (allowing for line endings)
		if callPos-commentGroup.End() <= MAX_GAP_SIZE {
			funcOffset := file.Position(callPos).Offset
			commentOffset := file.Position(commentGroup.End()).Offset

			gapSize := funcOffset - commentOffset

			if gapSize > 0 {
				b, err := os.ReadFile(filePath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "read error %s: %v\n", filePath, err)
					return ""
				}
				gapBytes := b[commentOffset:funcOffset]
				// confirm no more than 1 newline
				newlineCount := 0
				for i := 0; i < len(gapBytes); i++ {
					if gapBytes[i] == '\n' {
						newlineCount++
					}
					if newlineCount > 1 {
						fmt.Printf("more than 1 newline found in gap; skipping comment association\n")
						return ""
					}
				}
			}
			precedingComments = commentGroup.Text()
			break
		}
	}
	return precedingComments
}

/*** JUnit parsing ***/

// We support both <testsuites> and single <testsuite>.
type junitSuites struct {
	XMLName    xml.Name     `xml:"testsuites"`
	TestSuites []junitSuite `xml:"testsuite"`
}
type junitSuite struct {
	XMLName    xml.Name    `xml:"testsuite"`
	Name       string      `xml:"name,attr"`
	Time       string      `xml:"time,attr"`
	TotalTests int         `xml:"tests,attr"`
	Failures   int         `xml:"failures,attr"`
	TestCases  []junitCase `xml:"testcase"`
	// Some generators put <properties>, <system-out>, etc. which we ignore here.
}
type junitCase struct {
	XMLName xml.Name  `xml:"testcase"`
	Class   string    `xml:"classname,attr"` // often "github.com/your/module/pkg"
	Name    string    `xml:"name,attr"`      // "TestFoo[/Sub]"
	Time    string    `xml:"time,attr"`      // seconds string
	Failure *jFailure `xml:"failure"`
	Skipped *jSkipped `xml:"skipped"`
}
type jFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}
type jSkipped struct {
	Message string `xml:"message,attr"`
}

type junitRecord struct {
	Status   string // PASS/FAIL/SKIP
	Duration string // like "0.13s"
	Failure  string // message or body text
}

func ParseJUnitResults(path string) (map[string]junitRecord, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try <testsuites> first
	var suites junitSuites
	suitesErr := xml.Unmarshal(b, &suites)
	if suitesErr == nil && len(suites.TestSuites) > 0 {
		return collectFromSuites(suites), nil
	}

	// Fallback: maybe it's a single <testsuite>
	var single junitSuite
	singleErr := xml.Unmarshal(b, &single)
	if singleErr == nil && len(single.TestCases) > 0 {
		return collectFromSuites(junitSuites{TestSuites: []junitSuite{single}}), nil
	}

	// If both unmarshaling attempts failed, return an error
	if suitesErr != nil && singleErr != nil {
		return nil, fmt.Errorf("failed to parse JUnit XML as testsuites (%v) or testsuite (%v)", suitesErr, singleErr)
	}

	return map[string]junitRecord{}, nil
}

func collectFromSuites(suites junitSuites) map[string]junitRecord {
	out := map[string]junitRecord{}
	for _, ts := range suites.TestSuites {
		for _, tc := range ts.TestCases {
			pkg := tc.Class
			if pkg == "" {
				// some reporters stuff package into testsuite.name; last resort
				pkg = ts.Name
			}
			test := tc.Name
			status := "PASS"
			failMsg := ""
			if tc.Skipped != nil {
				status = "SKIP"
				if tc.Skipped.Message != "" {
					failMsg = tc.Skipped.Message
				}
			}
			if tc.Failure != nil {
				status = "FAIL"
				if tc.Failure.Message != "" {
					failMsg = tc.Failure.Message
				} else if tc.Failure.Text != "" {
					failMsg = tc.Failure.Text
				}
			}

			duration := ""
			if strings.TrimSpace(tc.Time) != "" {
				duration = fmt.Sprintf("%ss", strings.TrimSpace(tc.Time))
			}
			out[pkgKey(pkg, test)] = junitRecord{
				Status:   status,
				Duration: duration,
				Failure:  strings.TrimSpace(failMsg),
			}
		}
	}
	return out
}

func pkgKey(pkg, test string) string { return strings.TrimSpace(pkg) + "::" + strings.TrimSpace(test) }

func truncate(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func GenerateMarkdownReport(testSuites []TestSuite, jmap map[string]junitRecord, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer f.Close()

	w := func(format string, a ...interface{}) {
		fmt.Fprintf(f, format, a...)
	}

	w("# Test Documentation Report\n\n")

	for _, ts := range testSuites {
		w("## Test Suite: %s\n\n", ts.Name)

		if ts.CommentHeader != "" {
			w("**Suite Description:**\n\n%s\n\n", ts.CommentHeader)
		}

		// Create table header
		w("| Test Path | Status | Duration | Description | Failure |\n")
		w("|-----------|--------|----------|-------------|----------|\n")

		// Add main test and all subtests to the table
		for _, tu := range ts.TestUnits {
			generateTableRowsForTestUnit(w, tu, ts.PackageName, jmap, "")
		}

		w("\n")
	}

	return nil
}

func generateTableRowsForTestUnit(w func(string, ...interface{}), tu TestUnit, pkgName string, jmap map[string]junitRecord, pathPrefix string) {
	// Build the current test path
	currentPath := tu.TestName
	if pathPrefix != "" {
		currentPath = pathPrefix + " → " + tu.TestName
	}

	// Lookup JUnit record
	key := pkgKey(pkgName, tu.MachineTestName)
	status := "NOT RUN"
	duration := "-"
	failure := ""

	if rec, ok := jmap[key]; ok {
		status = rec.Status
		if rec.Duration != "" {
			duration = rec.Duration
		}
		if rec.Status == "FAIL" && rec.Failure != "" {
			failure = truncate(rec.Failure, 100) // Shorter for table
			// Escape pipe characters that would break table
			failure = strings.ReplaceAll(failure, "|", "\\|")
			failure = strings.ReplaceAll(failure, "\n", " ")
		}
	}

	// Extract description from comments
	description := ""
	if tu.CommentHeader != "" {
		description = extractSummaryFromComment(tu.CommentHeader)
		// Escape pipe characters
		description = strings.ReplaceAll(description, "|", "\\|")
		description = strings.ReplaceAll(description, "\n", " ")
	}

	// Add status emoji
	statusIcon := getStatusIcon(status)

	// Write the table row
	w("| %s | %s %s | %s | %s | %s |\n",
		currentPath, statusIcon, status, duration, description, failure)

	// Recursively add subtests
	for _, sub := range tu.Subtests {
		generateTableRowsForTestUnit(w, sub, pkgName, jmap, currentPath)
	}
}

func getStatusIcon(status string) string {
	switch status {
	case "PASS":
		return "✅"
	case "FAIL":
		return "❌"
	case "SKIP":
		return "⏭️"
	default:
		return "⚪"
	}
}

func extractSummaryFromComment(comment string) string {
	if comment == "" {
		return ""
	}
	lines := strings.Split(comment, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and tag lines
		if line != "" && !strings.HasPrefix(line, "@") && !strings.HasPrefix(line, "//") {
			return line
		}
		// Handle @desc: tags specifically
		if strings.HasPrefix(line, "@desc:") {
			return strings.TrimSpace(line[6:])
		}
	}
	return ""
}
