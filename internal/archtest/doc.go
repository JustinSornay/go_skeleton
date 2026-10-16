// Package archtest enforces the dependency rules of the architecture (see
// docs/architecture.md). The rules live in arch_test.go and run with the
// regular test suite, so a forbidden import fails CI instead of being caught
// (or missed) in review.
package archtest
