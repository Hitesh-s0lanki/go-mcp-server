package memory

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGenerateAPIKeyFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		k := GenerateAPIKey()
		if !keyPattern.MatchString(k) {
			t.Fatalf("generated key %q does not match mcp_<32 hex>", k)
		}
		if seen[k] {
			t.Fatalf("generated a duplicate key: %q", k)
		}
		seen[k] = true
	}
}

func reqWithKey(v string) *mcp.CallToolRequest {
	h := http.Header{}
	if v != "" {
		h.Set(APIKeyHeader, v)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: h}}
}

func TestCallerKey(t *testing.T) {
	valid := GenerateAPIKey()
	tests := []struct {
		name   string
		header string
		want   string
		wantOK bool
	}{
		{"valid minted key", valid, valid, true},
		{"whitespace trimmed", "  " + valid + "  ", valid, true},
		{"missing header", "", "", false},
		{"wrong prefix", "key_d398e9b902cc4cf3ab438dbe8cf76715", "", false},
		{"too short", "mcp_deadbeef", "", false},
		{"uppercase hex rejected", "mcp_D398E9B902CC4CF3AB438DBE8CF76715", "", false},
		{"an email (old scheme) rejected", "user@example.com", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := callerKey(reqWithKey(tc.header))
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

// TestCallerKeyRequiresExtra: with no HTTP metadata at all (e.g. stdio), identity
// must fail closed, not fall through to some shared scope.
func TestCallerKeyRequiresExtra(t *testing.T) {
	if _, err := callerKey(&mcp.CallToolRequest{}); err == nil {
		t.Fatal("want error when RequestExtra is absent, got nil")
	}
	if _, err := callerKey(&mcp.CallToolRequest{Extra: &mcp.RequestExtra{}}); err == nil {
		t.Fatal("want error when Header is absent, got nil")
	}
}
