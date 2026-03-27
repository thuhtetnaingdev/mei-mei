package stats

import (
	"testing"
)

func TestParseStatName(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedEmail string
		expectedDir   string
	}{
		{
			name:          "downlink stat",
			input:         "user>>>user1@example.com>>>traffic>>>downlink",
			expectedEmail: "user1@example.com",
			expectedDir:   "downlink",
		},
		{
			name:          "uplink stat",
			input:         "user>>>user2@example.com>>>traffic>>>uplink",
			expectedEmail: "user2@example.com",
			expectedDir:   "uplink",
		},
		{
			name:          "invalid format",
			input:         "invalid",
			expectedEmail: "",
			expectedDir:   "",
		},
		{
			name:          "missing traffic segment",
			input:         "user>>>email>>>downlink",
			expectedEmail: "",
			expectedDir:   "",
		},
		{
			name:          "empty email",
			input:         "user>>>>>>traffic>>>downlink",
			expectedEmail: "",
			expectedDir:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, direction := parseStatName(tt.input)
			if email != tt.expectedEmail {
				t.Fatalf("expected email %s, got %s", tt.expectedEmail, email)
			}
			if direction != tt.expectedDir {
				t.Fatalf("expected direction %s, got %s", tt.expectedDir, direction)
			}
		})
	}
}

func TestSplitByTripleGreater(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "user>>>email>>>traffic>>>downlink",
			expected: []string{"user", "email", "traffic", "downlink"},
		},
		{
			input:    "a>>>b>>>c",
			expected: []string{"a", "b", "c"},
		},
		{
			input:    "no_delimiter",
			expected: []string{"no_delimiter"},
		},
		{
			input:    "",
			expected: []string{""},
		},
		{
			input:    ">>>start",
			expected: []string{"", "start"},
		},
		{
			input:    "end>>>",
			expected: []string{"end", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitByTripleGreater(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d parts, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Fatalf("part %d: expected %s, got %s", i, exp, result[i])
				}
			}
		})
	}
}

func TestStatsClient_Creation(t *testing.T) {
	client := NewStatsClient("127.0.0.1:10085")
	if client == nil {
		t.Fatal("expected stats client to be created")
	}

	if client.IsConnected() {
		t.Fatal("expected client to not be connected initially")
	}
}
