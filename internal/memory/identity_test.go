package memory

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func reqWithHeader(v string) *mcp.CallToolRequest {
	h := http.Header{}
	if v != "" {
		h.Set(UserEmailHeader, v)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: h}}
}

func TestCallerEmail(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		wantOK bool
	}{
		{"plain", "user@example.com", "user@example.com", true},
		{"uppercase is normalized", "User@Example.COM", "user@example.com", true},
		{"whitespace is trimmed", "  user@example.com  ", "user@example.com", true},
		{"rfc5322 form is unwrapped", "Alice <alice@example.com>", "alice@example.com", true},
		{"missing header", "", "", false},
		{"not an email", "not-an-email", "", false},
		{"empty after trim", "   ", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := callerEmail(reqWithHeader(tc.header))
			if tc.wantOK && err != nil {
				t.Fatalf("want ok, got error: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("want error, got %q", got)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCallerEmailRequiresExtra guards the case where a transport provides no
// HTTP metadata at all (e.g. stdio): identity must fail closed, not default to
// some shared bucket.
func TestCallerEmailRequiresExtra(t *testing.T) {
	if _, err := callerEmail(&mcp.CallToolRequest{}); err == nil {
		t.Fatal("want error when RequestExtra is absent, got nil")
	}
	if _, err := callerEmail(&mcp.CallToolRequest{Extra: &mcp.RequestExtra{}}); err == nil {
		t.Fatal("want error when Header is absent, got nil")
	}
}
