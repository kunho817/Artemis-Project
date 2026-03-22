// Package visual provides validation utilities for visual testing.
package visual

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Validator defines the interface for validation functions.
type Validator interface {
	// Validate performs validation and returns a result.
	Validate(ctx context.Context, input interface{}) *ValidationResult

	// Name returns the validator name.
	Name() string
}

// ValidationResult represents the result of a validation.
type ValidationResult struct {
	// Valid indicates if validation passed.
	Valid bool

	// Message describes the validation result.
	Message string

	// Score is a confidence score (0-1).
	Score float64

	// Details contains additional validation details.
	Details map[string]interface{}

	// Timestamp is when validation was performed.
	Timestamp time.Time
}

// ValidationSuite runs multiple validators.
type ValidationSuite struct {
	validators []Validator
	results    []*ValidationResult
}

// NewValidationSuite creates a new validation suite.
func NewValidationSuite() *ValidationSuite {
	return &ValidationSuite{
		validators: make([]Validator, 0),
		results:    make([]*ValidationResult, 0),
	}
}

// AddValidator adds a validator to the suite.
func (vs *ValidationSuite) AddValidator(v Validator) {
	vs.validators = append(vs.validators, v)
}

// Validate runs all validators in the suite.
func (vs *ValidationSuite) Validate(ctx context.Context, input interface{}) []*ValidationResult {
	vs.results = make([]*ValidationResult, 0, len(vs.validators))

	for _, validator := range vs.validators {
		result := validator.Validate(ctx, input)
		result.Timestamp = time.Now()
		vs.results = append(vs.results, result)
	}

	return vs.results
}

// IsAllValid returns true if all validations passed.
func (vs *ValidationSuite) IsAllValid() bool {
	for _, result := range vs.results {
		if !result.Valid {
			return false
		}
	}
	return true
}

// GetFailedResults returns only failed validation results.
func (vs *ValidationSuite) GetFailedResults() []*ValidationResult {
	var failed []*ValidationResult
	for _, result := range vs.results {
		if !result.Valid {
			failed = append(failed, result)
		}
	}
	return failed
}

// GetScore returns the average validation score.
func (vs *ValidationSuite) GetScore() float64 {
	if len(vs.results) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, result := range vs.results {
		sum += result.Score
	}

	return sum / float64(len(vs.results))
}

// String returns a formatted validation report.
func (vs *ValidationSuite) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Validation Suite: %d validators\n", len(vs.validators)))
	sb.WriteString(fmt.Sprintf("Overall: %s\n", vs.GetStatus()))
	sb.WriteString(fmt.Sprintf("Score: %.2f\n\n", vs.GetScore()))

	for i, result := range vs.results {
		status := "✓"
		if !result.Valid {
			status = "✗"
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s: %s\n", i+1, status, result.Message, fmtScore(result.Score)))
	}

	return sb.String()
}

// GetStatus returns the overall validation status.
func (vs *ValidationSuite) GetStatus() string {
	if vs.IsAllValid() {
		return "PASS"
	}
	return "FAIL"
}

func fmtScore(score float64) string {
	if score >= 0.9 {
		return "Excellent"
	} else if score >= 0.7 {
		return "Good"
	} else if score >= 0.5 {
		return "Fair"
	}
	return "Poor"
}

// ContentValidator validates text content.
type ContentValidator struct {
	name        string
	minLength   int
	maxLength   int
	required    []string
	forbidden   []string
	patterns    map[string]*regexp.Regexp
}

// NewContentValidator creates a new content validator.
func NewContentValidator(name string) *ContentValidator {
	return &ContentValidator{
		name:     name,
		minLength: 0,
		maxLength: 10000,
		required: make([]string, 0),
		forbidden: make([]string, 0),
		patterns:  make(map[string]*regexp.Regexp),
	}
}

// SetLength sets min/max length constraints.
func (cv *ContentValidator) SetLength(min, max int) *ContentValidator {
	cv.minLength = min
	cv.maxLength = max
	return cv
}

// AddRequired adds required text patterns.
func (cv *ContentValidator) AddRequired(text string) *ContentValidator {
	cv.required = append(cv.required, text)
	return cv
}

// AddForbidden adds forbidden text patterns.
func (cv *ContentValidator) AddForbidden(text string) *ContentValidator {
	cv.forbidden = append(cv.forbidden, text)
	return cv
}

// AddPattern adds a regex pattern that must match.
func (cv *ContentValidator) AddPattern(name, pattern string) *ContentValidator {
	re := regexp.MustCompile(pattern)
	cv.patterns[name] = re
	return cv
}

// Name returns the validator name.
func (cv *ContentValidator) Name() string {
	return cv.name
}

// Validate validates content against rules.
func (cv *ContentValidator) Validate(ctx context.Context, input interface{}) *ValidationResult {
	content, ok := input.(string)
	if !ok {
		return &ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("%s: input must be string", cv.name),
			Score:   0.0,
		}
	}

	result := &ValidationResult{
		Valid:   true,
		Message: fmt.Sprintf("%s: content validation passed", cv.name),
		Score:   1.0,
	}

	// Check length
	length := len(content)
	if length < cv.minLength {
		result.Valid = false
		result.Message = fmt.Sprintf("%s: content too short (%d < %d)", cv.name, length, cv.minLength)
		result.Score = 0.3
		return result
	}

	if length > cv.maxLength {
		result.Valid = false
		result.Message = fmt.Sprintf("%s: content too long (%d > %d)", cv.name, length, cv.maxLength)
		result.Score = 0.3
		return result
	}

	// Check required patterns
	for _, req := range cv.required {
		if !strings.Contains(content, req) {
			result.Valid = false
			result.Message = fmt.Sprintf("%s: missing required text: %s", cv.name, req)
			result.Score = 0.5
			return result
		}
	}

	// Check forbidden patterns
	for _, forbidden := range cv.forbidden {
		if strings.Contains(content, forbidden) {
			result.Valid = false
			result.Message = fmt.Sprintf("%s: contains forbidden text: %s", cv.name, forbidden)
			result.Score = 0.2
			return result
		}
	}

	// Check regex patterns
	for name, pattern := range cv.patterns {
		if !pattern.MatchString(content) {
			result.Valid = false
			result.Message = fmt.Sprintf("%s: pattern '%s' not matched", cv.name, name)
			result.Score = 0.6
			return result
		}
	}

	return result
}

// LayoutValidator validates UI layout constraints.
type LayoutValidator struct {
	name      string
	rules     []LayoutRule
	strict    bool
}

// LayoutRule defines a layout constraint.
type LayoutRule struct {
	Name        string
	Description string
	ValidateFn  func(content string) *RuleResult
}

// RuleResult represents a rule validation result.
type RuleResult struct {
	Passed bool
	Reason string
	Details map[string]interface{}
}

// NewLayoutValidator creates a new layout validator.
func NewLayoutValidator(name string) *LayoutValidator {
	return &LayoutValidator{
		name:   name,
		rules:  make([]LayoutRule, 0),
		strict: false,
	}
}

// SetStrict enables strict mode (all rules must pass).
func (lv *LayoutValidator) SetStrict(strict bool) *LayoutValidator {
	lv.strict = strict
	return lv
}

// AddRule adds a layout rule.
func (lv *LayoutValidator) AddRule(name, description string, validateFn func(string) *RuleResult) *LayoutValidator {
	lv.rules = append(lv.rules, LayoutRule{
		Name:        name,
		Description: description,
		ValidateFn:  validateFn,
	})
	return lv
}

// Name returns the validator name.
func (lv *LayoutValidator) Name() string {
	return lv.name
}

// Validate validates layout against rules.
func (lv *LayoutValidator) Validate(ctx context.Context, input interface{}) *ValidationResult {
	content, ok := input.(string)
	if !ok {
		return &ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("%s: input must be string", lv.name),
			Score:   0.0,
		}
	}

	result := &ValidationResult{
		Valid:   true,
		Message: fmt.Sprintf("%s: layout validation passed", lv.name),
		Score:   1.0,
		Details: make(map[string]interface{}),
	}

	passedCount := 0
	var failures []string

	// Run all rules
	for _, rule := range lv.rules {
		ruleResult := rule.ValidateFn(content)
		if ruleResult.Passed {
			passedCount++
		} else {
			failures = append(failures, fmt.Sprintf("%s: %s", rule.Name, ruleResult.Reason))
		}
	}

	// Determine overall result
	if lv.strict && len(failures) > 0 {
		result.Valid = false
		result.Message = fmt.Sprintf("%s: %d rule(s) failed", lv.name, len(failures))
		result.Score = float64(passedCount) / float64(len(lv.rules))
	} else if !lv.strict && len(failures) > 0 {
		// Non-strict: warning only
		result.Message = fmt.Sprintf("%s: passed with %d warning(s)", lv.name, len(failures))
		result.Score = float64(passedCount) / float64(len(lv.rules))
	}

	result.Details["rules_passed"] = passedCount
	result.Details["rules_total"] = len(lv.rules)
	result.Details["failures"] = failures

	return result
}

// SemanticValidator validates semantic meaning of content.
type SemanticValidator struct {
	name         string
	expectations []Expectation
	vocab        map[string]string // Vocabulary mapping
}

// Expectation defines a semantic expectation.
type Expectation struct {
	Name        string
	Description string
	ValidateFn  func(content string) (bool, string)
}

// NewSemanticValidator creates a new semantic validator.
func NewSemanticValidator(name string) *SemanticValidator {
	return &SemanticValidator{
		name:         name,
		expectations: make([]Expectation, 0),
		vocab:        make(map[string]string),
	}
}

// AddExpectation adds a semantic expectation.
func (sv *SemanticValidator) AddExpectation(name, description string, validateFn func(string) (bool, string)) *SemanticValidator {
	sv.expectations = append(sv.expectations, Expectation{
		Name:        name,
		Description: description,
		ValidateFn:  validateFn,
	})
	return sv
}

// AddVocabulary adds vocabulary terms.
func (sv *SemanticValidator) AddVocabulary(term, definition string) *SemanticValidator {
	sv.vocab[term] = definition
	return sv
}

// Name returns the validator name.
func (sv *SemanticValidator) Name() string {
	return sv.name
}

// Validate validates semantic expectations.
func (sv *SemanticValidator) Validate(ctx context.Context, input interface{}) *ValidationResult {
	content, ok := input.(string)
	if !ok {
		return &ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("%s: input must be string", sv.name),
			Score:   0.0,
		}
	}

	result := &ValidationResult{
		Valid:   true,
		Message: fmt.Sprintf("%s: semantic validation passed", sv.name),
		Score:   1.0,
		Details: make(map[string]interface{}),
	}

	passedCount := 0
	var failures []string

	// Run all expectations
	for _, exp := range sv.expectations {
		passed, reason := exp.ValidateFn(content)
		if passed {
			passedCount++
		} else {
			failures = append(failures, fmt.Sprintf("%s: %s", exp.Name, reason))
		}
	}

	// Calculate score based on passed expectations
	if len(failures) > 0 {
		result.Valid = len(failures) < len(sv.expectations)/2 // Allow some failures
		result.Message = fmt.Sprintf("%s: %d/%d expectations met", sv.name, passedCount, len(sv.expectations))
		result.Score = float64(passedCount) / float64(len(sv.expectations))
		result.Details["failures"] = failures
	}

	result.Details["expectations_passed"] = passedCount
	result.Details["expectations_total"] = len(sv.expectations)

	return result
}

// CompositeValidator combines multiple validators with AND/OR logic.
type CompositeValidator struct {
	name       string
	validators []Validator
	logic      LogicOp
}

// LogicOp defines how to combine validator results.
type LogicOp int

const (
	LogicAnd LogicOp = iota // All validators must pass
	LogicOr                 // At least one validator must pass
)

// NewCompositeValidator creates a new composite validator.
func NewCompositeValidator(name string, logic LogicOp) *CompositeValidator {
	return &CompositeValidator{
		name:       name,
		validators: make([]Validator, 0),
		logic:      logic,
	}
}

// AddValidator adds a child validator.
func (cv *CompositeValidator) AddValidator(v Validator) *CompositeValidator {
	cv.validators = append(cv.validators, v)
	return cv
}

// Name returns the validator name.
func (cv *CompositeValidator) Name() string {
	return cv.name
}

// Validate validates using the specified logic operation.
func (cv *CompositeValidator) Validate(ctx context.Context, input interface{}) *ValidationResult {
	results := make([]*ValidationResult, 0, len(cv.validators))

	// Run all validators
	for _, validator := range cv.validators {
		result := validator.Validate(ctx, input)
		results = append(results, result)
	}

	// Combine results based on logic
	var finalResult *ValidationResult

	switch cv.logic {
	case LogicAnd:
		// All must pass
		allValid := true
		minScore := 1.0

		for _, result := range results {
			if !result.Valid {
				allValid = false
			}
			if result.Score < minScore {
				minScore = result.Score
			}
		}

		finalResult = &ValidationResult{
			Valid:   allValid,
			Message: fmt.Sprintf("%s: AND logic - %d/%d passed", cv.name, countPassed(results), len(results)),
			Score:   minScore,
			Details: map[string]interface{}{"results": results},
		}

	case LogicOr:
		// At least one must pass
		anyValid := false
		maxScore := 0.0

		for _, result := range results {
			if result.Valid {
				anyValid = true
			}
			if result.Score > maxScore {
				maxScore = result.Score
			}
		}

		finalResult = &ValidationResult{
			Valid:   anyValid,
			Message: fmt.Sprintf("%s: OR logic - %d/%d passed", cv.name, countPassed(results), len(results)),
			Score:   maxScore,
			Details: map[string]interface{}{"results": results},
		}
	}

	return finalResult
}

func countPassed(results []*ValidationResult) int {
	count := 0
	for _, result := range results {
		if result.Valid {
			count++
		}
	}
	return count
}

// Helper function to create common validators.

// RequiredTextValidator creates a validator that checks for required text.
func RequiredTextValidator(name string, required []string) Validator {
	cv := NewContentValidator(name)
	for _, req := range required {
		cv.AddRequired(req)
	}
	return cv
}

// ForbiddenTextValidator creates a validator that checks for forbidden text.
func ForbiddenTextValidator(name string, forbidden []string) Validator {
	cv := NewContentValidator(name)
	for _, fb := range forbidden {
		cv.AddForbidden(fb)
	}
	return cv
}

// LengthValidator creates a validator that checks text length.
func LengthValidator(name string, min, max int) Validator {
	return NewContentValidator(name).SetLength(min, max)
}

// RegexValidator creates a validator that checks regex patterns.
func RegexValidator(name string, patterns map[string]string) Validator {
	cv := NewContentValidator(name)
	for pname, pattern := range patterns {
		cv.AddPattern(pname, pattern)
	}
	return cv
}
