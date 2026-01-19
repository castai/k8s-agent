package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// normalizeNumericID attempts to convert scientific notation strings back to normal numeric strings.
// This handles cases where YAML parsers convert large unquoted numbers like 589251789600 to 5.892517896e+11.
// Returns the normalized string and true if normalization occurred, or the original string and false if not needed.
func normalizeNumericID(value string) (string, bool) {
	if value == "" {
		return value, false
	}

	// Check if the string contains scientific notation (e or E followed by +/- and digits)
	if !strings.ContainsAny(value, "eE") {
		return value, false
	}

	// Try to parse as float64
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		// Not a valid float, return as-is
		return value, false
	}

	// Convert back to string without scientific notation
	// Use %.0f to remove decimal places for integer values
	normalized := fmt.Sprintf("%.0f", f)

	return normalized, true
}

// normalizeAWSAccountID normalizes AWS account IDs that may be in scientific notation.
// AWS account IDs are always 12 digits, making them susceptible to YAML's automatic
// conversion of large numbers to scientific notation when unquoted.
// Does not enforce strict format validation since AWS could change the format in the future.
func normalizeAWSAccountID(id string, fieldName string) (string, error) {
	if id == "" {
		return "", nil // Empty validation handled elsewhere
	}

	// Attempt to normalize scientific notation
	normalized, wasNormalized := normalizeNumericID(id)

	if wasNormalized {
		// Write warning to stderr for visibility during config initialization
		fmt.Fprintf(os.Stderr, "WARNING: %s was provided in scientific notation (%q) and has been normalized to %q. "+
			"To avoid this in the future, quote numeric values in YAML/Helm: --set additionalEnv.%s=\"%s\"\n",
			fieldName, id, normalized, fieldName, normalized)
	}

	return normalized, nil
}
