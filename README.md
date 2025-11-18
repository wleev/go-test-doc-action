# Go Test Documentation Action

A Go tool that generates markdown documentation from Go test files with JUnit XML integration.

## Features

- **AST-based parsing** of Go test files
- **Comment extraction** and association with test functions
- **Parameterized test expansion** (handles for-loop generated subtests)
- **JUnit XML integration** for test status and timing
- **Table-based markdown output** with hierarchical structure
- **GitHub Actions ready** with automated workflows

## Generated Documentation

This repository automatically generates test documentation:

- [**TESTS.md**](TESTS.md) - Complete test documentation

## Usage

### Local Usage

```bash
# Install dependencies
go install gotest.tools/gotestsum@latest

# Build the tool
cd cmd/testdoc
go build -o testdoc .

# Run tests with JUnit output
gotestsum --junitfile junit.xml --format testname -- -v ./...

# Generate documentation
./testdoc -source . -o TESTS.md -junit junit.xml
```

### GitHub Actions

The repository includes three workflows:

1. **test-and-document.yml** - Full repository CI/CD
2. **testdoc.yml** - Tool-specific testing  
3. **generate-docs.yml** - Documentation generation with auto-commit

## Example Output

The tool generates professional markdown tables:

| Test Path | Status | Duration | Description | Failure |
|-----------|--------|----------|-------------|---------|
| TestDocumentationGenerator | ✅ PASS | 0.050s | Tests the test documentation generation functionality |  |
| TestDocumentationGenerator → parse_test_suites | ✅ PASS | 0.030s | Test parsing test suites from the sample file |  |
| TestSelfDocumentation | ✅ PASS | 0.080s | Meta-test that validates the tool can document itself |  |

## Test Features

The tool handles:

- **Simple test functions** with comments
- **Subtests** with `t.Run()` calls
- **Parameterized tests** with loop-generated subtests
- **Nested test hierarchies**
- **JUnit XML status matching**
- **Error scenarios** and edge cases

## Architecture

```
cmd/testdoc/
├── main.go           # CLI tool implementation
├── main_test.go      # Comprehensive test suite
└── TESTS.md          # Auto-generated documentation
```

Key functions:
- `ParseTestSuites()` - AST parsing of Go test files
- `CollectSubtests()` - Recursive subtest collection
- `ExpandTestName()` - Variable expansion for parameterized tests
- `ParseJUnitResults()` - JUnit XML parsing
- `GenerateMarkdownReport()` - Markdown table generation

## License

MIT License