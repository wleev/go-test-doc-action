package main_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	main "github.com/wleev/go-test-doc-action/cmd/testdoc"
)

// TestDocumentationGenerator tests the test documentation generation functionality
// This test validates that the tool can parse its own test files and generate proper documentation
func TestDocumentationGenerator(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "sample_test.go")
	junitFile := filepath.Join(tempDir, "junit.xml")
	outputFile := filepath.Join(tempDir, "output.md")

	// Create sample test file with various test patterns
	sampleTestCode := `package testproject_test

import (
	"fmt"
	"testing"
)

// TestBasicFunction tests a basic function
// This test validates basic functionality
func TestBasicFunction(t *testing.T) {
	// Test basic case
	if true != true {
		t.Error("basic test failed")
	}
}

// TestWithSubtests demonstrates subtest functionality
// This test shows how subtests work with different inputs
func TestWithSubtests(t *testing.T) {
	for _, input := range []string{"input1", "input2", "input3"} {
		t.Run("subtest_for_"+input, func(t *testing.T) {
			// Validate the input
			if input == "" {
				t.Error("empty input")
			}
		})
	}
	
	// Additional direct subtest
	t.Run("direct_subtest", func(t *testing.T) {
		// Direct subtest logic
		if false {
			t.Error("direct subtest failed")
		}
	})
}

// TestComplexLoop tests complex loop patterns
// This validates loop-based test generation with multiple variables
func TestComplexLoop(t *testing.T) {
	values := []int{1, 2, 3}
	names := []string{"a", "b", "c"}
	
	for i, val := range values {
		for j, name := range names {
			testName := fmt.Sprintf("test_%s_%d", name, val)
			t.Run(testName, func(t *testing.T) {
				// Complex test logic
				if i != j && val > 0 {
					// Test passes
				}
			})
		}
	}
}
`

	// Create sample JUnit XML
	sampleJUnit := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
	<testsuite name="testproject" tests="8" failures="1" time="0.123">
		<testcase name="TestBasicFunction" time="0.001" status="PASS"/>
		<testcase name="TestWithSubtests/subtest_for_input1" time="0.002" status="PASS"/>
		<testcase name="TestWithSubtests/subtest_for_input2" time="0.003" status="FAIL">
			<failure message="test failed" type="assertion"/>
		</testcase>
		<testcase name="TestWithSubtests/subtest_for_input3" time="0.001" status="PASS"/>
		<testcase name="TestWithSubtests/direct_subtest" time="0.001" status="PASS"/>
		<testcase name="TestComplexLoop/test_a_1" time="0.001" status="PASS"/>
		<testcase name="TestComplexLoop/test_b_2" time="0.002" status="PASS"/>
		<testcase name="TestComplexLoop/test_c_3" time="0.001" status="PASS"/>
	</testsuite>
</testsuites>`

	// Write test files
	if err := os.WriteFile(testFile, []byte(sampleTestCode), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create go.mod in temp directory for proper package resolution
	goModContent := `module testproject

go 1.21
`
	goModFile := filepath.Join(tempDir, "go.mod")
	if err := os.WriteFile(goModFile, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to create go.mod file: %v", err)
	}

	if err := os.WriteFile(junitFile, []byte(sampleJUnit), 0644); err != nil {
		t.Fatalf("Failed to create junit file: %v", err)
	}

	t.Run("parse_test_suites", func(t *testing.T) {
		// Test parsing test suites from the sample file
		testSuites, err := main.ParseTestSuites(tempDir)
		if err != nil {
			t.Fatalf("Failed to parse test suites: %v", err)
		}

		// Validate that we found the expected test suites
		if len(testSuites) == 0 {
			t.Error("No test suites found")
		}

		// Find the main test suite
		var mainSuite *main.TestSuite
		for _, suite := range testSuites {
			if suite.PackageName == "testproject" || suite.PackageName == "main" {
				mainSuite = &suite
				break
			}
		}

		if mainSuite == nil {
			t.Fatal("Main test suite not found")
		}

		// Validate test units were parsed
		if len(mainSuite.TestUnits) < 3 {
			t.Errorf("Expected at least 3 test units, got %d", len(mainSuite.TestUnits))
		}

		// Find and validate specific tests
		tests := make(map[string]*main.TestUnit)
		for i := range mainSuite.TestUnits {
			tests[mainSuite.TestUnits[i].MachineTestName] = &mainSuite.TestUnits[i]
		}

		// Validate TestBasicFunction
		if basicTest, exists := tests["TestBasicFunction"]; exists {
			if !strings.Contains(basicTest.CommentHeader, "tests a basic function") {
				t.Error("TestBasicFunction comment not properly parsed")
			}
			if basicTest.TestName != "TestBasicFunction" {
				t.Errorf("Expected TestBasicFunction, got %s", basicTest.TestName)
			}
		} else {
			t.Error("TestBasicFunction not found")
		}

		// Validate TestWithSubtests has subtests
		if subtestTest, exists := tests["TestWithSubtests"]; exists {
			if len(subtestTest.Subtests) < 4 {
				t.Errorf("Expected at least 4 subtests, got %d", len(subtestTest.Subtests))
			} // Check for expanded subtests
			subtestNames := make(map[string]bool)
			for _, subtest := range subtestTest.Subtests {
				subtestNames[subtest.MachineTestName] = true
			}

			expectedSubtests := []string{
				"TestWithSubtests/subtest_for_input1",
				"TestWithSubtests/subtest_for_input2",
				"TestWithSubtests/subtest_for_input3",
				"TestWithSubtests/direct_subtest",
			}

			for _, expected := range expectedSubtests {
				if !subtestNames[expected] {
					t.Errorf("Expected subtest %s not found", expected)
				}
			}
		} else {
			t.Error("TestWithSubtests not found")
		}
	})

	t.Run("parse_junit_results", func(t *testing.T) {
		// Test JUnit XML parsing
		junitResults, err := main.ParseJUnitResults(junitFile)
		if err != nil {
			t.Fatalf("Failed to parse JUnit results: %v", err)
		}

		// Validate test cases were parsed
		if len(junitResults) < 8 {
			t.Errorf("Expected at least 8 test results, got %d", len(junitResults))
		}

		// Check specific test results
		if result, exists := junitResults["testproject::TestBasicFunction"]; exists {
			if result.Status != "PASS" {
				t.Errorf("Expected PASS, got %s", result.Status)
			}
		} else {
			t.Error("TestBasicFunction result not found")
		}

		// Check failed test
		if result, exists := junitResults["testproject::TestWithSubtests/subtest_for_input2"]; exists {
			if result.Status != "FAIL" {
				t.Errorf("Expected FAIL, got %s", result.Status)
			}
			if result.Failure == "" {
				t.Error("Expected failure message for failed test")
			}
		} else {
			t.Error("Failed test result not found")
		}
	})

	t.Run("generate_markdown_tables", func(t *testing.T) {
		// Test full markdown generation
		testSuites, err := main.ParseTestSuites(tempDir)
		if err != nil {
			t.Fatalf("Failed to parse test suites: %v", err)
		}

		junitResults, err := main.ParseJUnitResults(junitFile)
		if err != nil {
			t.Fatalf("Failed to parse JUnit results: %v", err)
		}

		// Generate markdown
		err = main.GenerateMarkdownReport(testSuites, junitResults, outputFile)
		if err != nil {
			t.Fatalf("Failed to generate markdown: %v", err)
		}

		// Read and validate output
		content, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("Failed to read output file: %v", err)
		}

		output := string(content)

		// Validate markdown structure
		if !strings.Contains(output, "# Test Documentation Report") {
			t.Error("Missing main header")
		}

		if !strings.Contains(output, "## Test Suite:") {
			t.Error("Missing package header")
		}

		if !strings.Contains(output, "| Test Path | Status | Duration | Description | Failure |") {
			t.Error("Missing table header")
		}

		// Validate test entries
		expectedTests := []string{
			"TestBasicFunction",
			"TestWithSubtests",
			"TestComplexLoop",
		}

		for _, testName := range expectedTests {
			if !strings.Contains(output, testName) {
				t.Errorf("Missing test %s in output", testName)
			}
		}

		// Validate status indicators
		if !strings.Contains(output, "✅") {
			t.Error("Missing pass indicator")
		}

		if !strings.Contains(output, "❌") {
			t.Error("Missing fail indicator")
		}
	})

	t.Run("comment_parsing", func(t *testing.T) {
		// Test comment parsing functionality
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, testFile, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("Failed to parse file: %v", err)
		}

		// Test FindRelativeComment function
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "Test") {
				comment := main.FindRelativeComment(fn.Pos(), file.Comments, fset.File(fn.Pos()), testFile)
				if comment == "" {
					t.Errorf("No comment found for function %s", fn.Name.Name)
				} else {
					// Validate comment structure
					lines := strings.Split(comment, "\n")
					if len(lines) < 2 {
						t.Errorf("Comment for %s should have at least 2 lines", fn.Name.Name)
					}

					// First line should be function description
					if !strings.Contains(lines[0], fn.Name.Name) {
						t.Errorf("First comment line should mention function name %s", fn.Name.Name)
					}
				}
			}
		}
	})

	t.Run("loop_expansion", func(t *testing.T) {
		// Test loop variable expansion
		testVars := []main.ExpandedVar{
			{VarName: "input", VarValue: []string{"input1", "input2", "input3"}},
		}

		// Create a mock AST expression that would represent the template
		// For this test, we'll test the actual functionality with a simpler approach
		// since ExpandTestName expects ast.Expr, not string templates

		// Test with variable expansion
		varIdent := &ast.Ident{Name: "input"}
		expandedNames := main.ExpandTestName(varIdent, testVars)
		expectedNames := []string{"input1", "input2", "input3"}
		if len(expandedNames) != len(expectedNames) {
			t.Errorf("Expected %d expanded names, got %d", len(expectedNames), len(expandedNames))
		}

		for i, expected := range expectedNames {
			if i < len(expandedNames) && expandedNames[i] != expected {
				t.Errorf("Expected %s, got %s", expected, expandedNames[i])
			}
		}
	})
}

// TestSelfDocumentation tests that this test suite itself generates proper documentation
// This is a meta-test that validates the tool can document its own test suite
func TestSelfDocumentation(t *testing.T) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Parse this test file itself
	testSuites, err := main.ParseTestSuites(cwd)
	if err != nil {
		t.Fatalf("Failed to parse own test suites: %v", err)
	}

	// Find this test package
	var ownSuite *main.TestSuite
	for _, suite := range testSuites {
		if strings.Contains(suite.Name, "main_test") || strings.Contains(suite.PackageName, "main") {
			ownSuite = &suite
			break
		}
	}

	if ownSuite == nil {
		t.Fatal("Could not find own test suite")
	}

	// Validate that our own tests are properly documented
	testFound := false
	selfTestFound := false

	for _, testUnit := range ownSuite.TestUnits {
		if testUnit.TestName == "TestDocumentationGenerator" {
			testFound = true
			// Should have proper comment
			if !strings.Contains(testUnit.CommentHeader, "tests the test documentation generation") {
				t.Error("TestDocumentationGenerator should have descriptive comment")
			}
			// Should have subtests
			if len(testUnit.Subtests) == 0 {
				t.Error("TestDocumentationGenerator should have subtests")
			}
		}

		if testUnit.TestName == "TestSelfDocumentation" {
			selfTestFound = true
			if !strings.Contains(testUnit.CommentHeader, "meta-test") {
				t.Error("TestSelfDocumentation should mention it's a meta-test")
			}
		}
	}

	if !testFound {
		t.Error("TestDocumentationGenerator not found in own suite")
	}
	if !selfTestFound {
		t.Error("TestSelfDocumentation not found in own suite")
	}
}

// TestStructValidation validates the core data structures
// This test ensures TestSuite and TestUnit have all required fields
func TestStructValidation(t *testing.T) {
	t.Run("test_suite_structure", func(t *testing.T) {
		suite := main.TestSuite{
			PackageName:   "test_package",
			Name:          "test_suite",
			CommentHeader: "Test suite comment",
			TestUnits:     []main.TestUnit{},
		}

		// Validate all fields are accessible
		if suite.PackageName == "" {
			t.Error("PackageName field not properly set")
		}
		if suite.Name == "" {
			t.Error("Name field not properly set")
		}
		if suite.CommentHeader == "" {
			t.Error("CommentHeader field not properly set")
		}
		if suite.TestUnits == nil {
			t.Error("TestUnits field should be initialized")
		}
	})

	t.Run("test_unit_structure", func(t *testing.T) {
		unit := main.TestUnit{
			CommentHeader:   "Test unit comment",
			MachineTestName: "TestExample",
			TestName:        "TestExample",
			Subtests:        []main.TestUnit{},
		}

		// Validate all fields are accessible
		if unit.CommentHeader == "" {
			t.Error("CommentHeader field not properly set")
		}
		if unit.MachineTestName == "" {
			t.Error("MachineTestName field not properly set")
		}
		if unit.TestName == "" {
			t.Error("TestName field not properly set")
		}
		if unit.Subtests == nil {
			t.Error("Subtests field should be initialized")
		}
	})

	t.Run("expanded_var_structure", func(t *testing.T) {
		expVar := main.ExpandedVar{
			VarName:  "testVar",
			VarValue: []string{"val1", "val2"},
		}

		if expVar.VarName == "" {
			t.Error("varName field not properly set")
		}
		if len(expVar.VarValue) != 2 {
			t.Error("varValue field not properly set")
		}
	})
}

// TestErrorHandling tests error conditions and edge cases
// This validates that the tool handles various error scenarios gracefully
func TestErrorHandling(t *testing.T) {
	t.Run("invalid_junit_file", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidJunit := filepath.Join(tempDir, "invalid.xml")

		// Create invalid XML
		err := os.WriteFile(invalidJunit, []byte("invalid xml content"), 0644)
		if err != nil {
			t.Fatalf("Failed to create invalid file: %v", err)
		}

		// Should handle error gracefully
		_, err = main.ParseJUnitResults(invalidJunit)
		if err == nil {
			t.Error("Expected error for invalid XML")
		}
	})

	t.Run("missing_source_directory", func(t *testing.T) {
		// Should handle missing directory gracefully
		_, err := main.ParseTestSuites("/nonexistent/directory")
		if err == nil {
			t.Error("Expected error for missing directory")
		}
	})
}
