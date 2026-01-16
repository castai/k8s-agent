package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeNumericID(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput string
		expectedNorm   bool
	}{
		{
			name:           "scientific notation positive exponent",
			input:          "5.892517896e+11",
			expectedOutput: "589251789600",
			expectedNorm:   true,
		},
		{
			name:           "scientific notation uppercase E",
			input:          "5.892517896E+11",
			expectedOutput: "589251789600",
			expectedNorm:   true,
		},
		{
			name:           "scientific notation negative exponent",
			input:          "1.23e-5",
			expectedOutput: "0",
			expectedNorm:   true,
		},
		{
			name:           "normal numeric string",
			input:          "589251789600",
			expectedOutput: "589251789600",
			expectedNorm:   false,
		},
		{
			name:           "alphanumeric string",
			input:          "my-project-123",
			expectedOutput: "my-project-123",
			expectedNorm:   false,
		},
		{
			name:           "empty string",
			input:          "",
			expectedOutput: "",
			expectedNorm:   false,
		},
		{
			name:           "invalid scientific notation",
			input:          "5.89e",
			expectedOutput: "5.89e",
			expectedNorm:   false,
		},
		{
			name:           "12 digit account ID",
			input:          "123456789012",
			expectedOutput: "123456789012",
			expectedNorm:   false,
		},
		{
			name:           "scientific notation for 12 digit number",
			input:          "1.23456789012e+11",
			expectedOutput: "123456789012",
			expectedNorm:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, wasNormalized := normalizeNumericID(tt.input)
			assert.Equal(t, tt.expectedOutput, output)
			assert.Equal(t, tt.expectedNorm, wasNormalized)
		})
	}
}

func TestNormalizeCloudProviderID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		fieldName string
		expected  string
	}{
		// AWS account ID cases
		{
			name:      "AWS: valid 12 digit account ID",
			id:        "123456789012",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "123456789012",
		},
		{
			name:      "AWS: valid account ID with leading zeros",
			id:        "000123456789",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "000123456789",
		},
		{
			name:      "AWS: scientific notation - normalized successfully",
			id:        "5.892517896e+11",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "589251789600",
		},
		{
			name:      "AWS: scientific notation uppercase E",
			id:        "1.23456789012E+11",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "123456789012",
		},
		// GCP project ID cases
		{
			name:      "GCP: valid alphanumeric project ID",
			id:        "my-project-123",
			fieldName: "GKE_PROJECT_ID",
			expected:  "my-project-123",
		},
		{
			name:      "GCP: valid project ID lowercase",
			id:        "myproject",
			fieldName: "GKE_PROJECT_ID",
			expected:  "myproject",
		},
		{
			name:      "GCP: valid project ID with hyphens",
			id:        "my-gcp-project-2024",
			fieldName: "GKE_PROJECT_ID",
			expected:  "my-gcp-project-2024",
		},
		{
			name:      "GCP: valid numeric project ID (legacy)",
			id:        "123456789012",
			fieldName: "GKE_PROJECT_ID",
			expected:  "123456789012",
		},
		{
			name:      "GCP: scientific notation numeric project ID",
			id:        "1.23456789012e+11",
			fieldName: "GKE_PROJECT_ID",
			expected:  "123456789012",
		},
		// Generic cases
		{
			name:      "empty string",
			id:        "",
			fieldName: "SOME_FIELD",
			expected:  "",
		},
		{
			name:      "alphanumeric - passes through unchanged",
			id:        "my-account-123",
			fieldName: "SOME_FIELD",
			expected:  "my-account-123",
		},
		{
			name:      "short number - passes through unchanged",
			id:        "12345",
			fieldName: "SOME_FIELD",
			expected:  "12345",
		},
		{
			name:      "scientific notation small number",
			id:        "1.23e+5",
			fieldName: "SOME_FIELD",
			expected:  "123000",
		},
		{
			name:      "uppercase - passes through",
			id:        "My-Project",
			fieldName: "SOME_FIELD",
			expected:  "My-Project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeCloudProviderID(tt.id, tt.fieldName)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
