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

func TestNormalizeAWSAccountID(t *testing.T) {
	tests := []struct {
		name      string
		accountID string
		fieldName string
		expected  string
	}{
		{
			name:      "valid 12 digit account ID",
			accountID: "123456789012",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "123456789012",
		},
		{
			name:      "valid account ID with leading zeros",
			accountID: "000123456789",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "000123456789",
		},
		{
			name:      "scientific notation - normalized successfully",
			accountID: "5.892517896e+11",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "589251789600",
		},
		{
			name:      "scientific notation uppercase E",
			accountID: "1.23456789012E+11",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "123456789012",
		},
		{
			name:      "SelfHostedEC2 account ID with scientific notation",
			accountID: "1.23456789012e+11",
			fieldName: "SELFHOSTEDEC2_ACCOUNT_ID",
			expected:  "123456789012",
		},
		{
			name:      "empty string",
			accountID: "",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "",
		},
		{
			name:      "alphanumeric - passes through unchanged",
			accountID: "my-account-123",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "my-account-123",
		},
		{
			name:      "short number - passes through unchanged",
			accountID: "12345",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "12345",
		},
		{
			name:      "scientific notation small number",
			accountID: "1.23e+5",
			fieldName: "EKS_ACCOUNT_ID",
			expected:  "123000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeAWSAccountID(tt.accountID, tt.fieldName)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
