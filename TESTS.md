# Test Documentation Report

## Test Suite: main_test.go

| Test Path | Status | Duration | Description | Failure |
|-----------|--------|----------|-------------|----------|
| TestDocumentationGenerator | ✅ PASS | 0.050000s | TestDocumentationGenerator tests the test documentation generation functionality |  |
| TestDocumentationGenerator → parse_test_suites | ✅ PASS | 0.020000s | Test parsing test suites from the sample file |  |
| TestDocumentationGenerator → parse_junit_results | ✅ PASS | 0.000000s | Test JUnit XML parsing |  |
| TestDocumentationGenerator → generate_markdown_tables | ✅ PASS | 0.020000s | Test full markdown generation |  |
| TestDocumentationGenerator → comment_parsing | ✅ PASS | 0.000000s | Test comment parsing functionality |  |
| TestDocumentationGenerator → loop_expansion | ✅ PASS | 0.000000s | Test loop variable expansion |  |
| TestSelfDocumentation | ✅ PASS | 0.040000s | TestSelfDocumentation tests that this test suite itself generates proper documentation |  |
| TestStructValidation | ✅ PASS | 0.000000s | TestStructValidation validates the core data structures |  |
| TestStructValidation → test_suite_structure | ✅ PASS | 0.000000s | Validate all fields are accessible |  |
| TestStructValidation → test_unit_structure | ✅ PASS | 0.000000s | Validate all fields are accessible |  |
| TestStructValidation → expanded_var_structure | ✅ PASS | 0.000000s | TestErrorHandling tests error conditions and edge cases |  |
| TestErrorHandling | ✅ PASS | 0.000000s | TestErrorHandling tests error conditions and edge cases |  |
| TestErrorHandling → invalid_junit_file | ✅ PASS | 0.000000s | Create invalid XML |  |
| TestErrorHandling → missing_source_directory | ✅ PASS | 0.000000s | Should handle missing directory gracefully |  |

