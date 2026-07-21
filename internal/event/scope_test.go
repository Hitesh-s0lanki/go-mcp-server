package event

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
)

// fakeResolver maps api keys to ids in-memory so the owner-scoping logic can be
// exercised without a database. An unknown key mimics auth.ErrInvalidKey.
type fakeResolver map[string]string

func (f fakeResolver) Resolve(_ context.Context, key string) (string, error) {
	if id, ok := f[key]; ok {
		return id, nil
	}
	return "", auth.ErrInvalidKey
}

// reqWithKey builds a CallToolRequest carrying an X-API-Key header, mirroring
// how the transport delivers the caller's identity.
func reqWithKey(v string) *mcp.CallToolRequest {
	h := http.Header{}
	if v != "" {
		h.Set(APIKeyHeader, v)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: h}}
}

// TestStampOwnerIsAuthoritative pins that publishing always tags the record with
// the caller's id and that a client cannot forge ownership by sending its own
// owner header.
func TestStampOwnerIsAuthoritative(t *testing.T) {
	const owner = "11111111-1111-1111-1111-111111111111"
	headers := map[string]string{
		"trace":     "abc",
		ownerHeader: "22222222-2222-2222-2222-222222222222", // a spoof attempt
	}

	got := fromKafkaHeaders(stampOwner(headers, owner))

	if got[ownerHeader] != owner {
		t.Fatalf("owner header = %q, want the caller's id %q (spoof not stripped)", got[ownerHeader], owner)
	}
	if got["trace"] != "abc" {
		t.Fatalf("caller headers should be preserved, got %v", got)
	}
	// Exactly one owner header must exist (no duplicate from the spoofed one).
	var count int
	for _, kv := range stampOwner(headers, owner) {
		if kv.Key == ownerHeader {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want exactly one owner header, got %d", count)
	}
}

func TestScopedGroupIsPerOwner(t *testing.T) {
	a := scopedGroup("g", "owner-a")
	b := scopedGroup("g", "owner-b")
	if a == b {
		t.Fatalf("distinct owners must get distinct groups, both = %q", a)
	}
	if a != "g.owner-a" {
		t.Fatalf("scopedGroup = %q, want %q", a, "g.owner-a")
	}
}

func TestCallerScope(t *testing.T) {
	const key = "mcp_00000000000000000000000000000001"
	c := &Client{keys: fakeResolver{key: "id-1"}}

	t.Run("valid key resolves to id", func(t *testing.T) {
		got, err := c.callerScope(context.Background(), reqWithKey(key))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "id-1" {
			t.Fatalf("owner = %q, want %q", got, "id-1")
		}
	})

	t.Run("missing key fails closed", func(t *testing.T) {
		if _, err := c.callerScope(context.Background(), reqWithKey("")); err == nil {
			t.Fatal("want error for missing X-API-Key, got nil")
		}
	})

	t.Run("unregistered key rejected", func(t *testing.T) {
		other := "mcp_00000000000000000000000000000009"
		if _, err := c.callerScope(context.Background(), reqWithKey(other)); err == nil {
			t.Fatal("want error for unregistered key, got nil")
		}
	})

	t.Run("nil resolver fails closed", func(t *testing.T) {
		noDB := &Client{}
		if _, err := noDB.callerScope(context.Background(), reqWithKey(key)); err == nil {
			t.Fatal("want error when no resolver is configured, got nil")
		}
	})
}

// TestPublishConsumeRequireIdentity pins that a configured client still refuses
// to publish or consume without a caller identity -- admission is not scoping.
func TestPublishConsumeRequireIdentity(t *testing.T) {
	setKafkaEnv(t, map[string]string{
		"KAFKA_BOOTSTRAP_SERVERS": "broker:9092",
		"KAFKA_API_KEY":           "KEY",
		"KAFKA_API_SECRET":        "SECRET",
	})
	c := NewClient(context.Background(), nil, nil) // configured Kafka, but no resolver
	ctx := context.Background()

	// Input is valid (topic + value present), so the failure is identity, not
	// validation -- and it happens before any network call.
	res, _, err := c.publish(ctx, reqWithKey(""), publishInput{Topic: "t", Value: "v"})
	mustToolError(t, res, err, "identify caller")

	res, _, err = c.consume(ctx, reqWithKey(""), consumeInput{Topic: "t"})
	mustToolError(t, res, err, "identify caller")
}
