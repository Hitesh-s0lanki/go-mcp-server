package event

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setKafkaEnv clears then sets the KAFKA_* vars for one test via t.Setenv, so
// the process environment does not leak between cases.
func setKafkaEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{
		"KAFKA_BOOTSTRAP_SERVERS", "KAFKA_API_KEY", "KAFKA_API_SECRET",
		"KAFKA_CONSUMER_GROUP", "KAFKA_DEFAULT_TOPIC", "KAFKA_ALLOW_TOPIC_ADMIN",
	} {
		t.Setenv(k, kv[k])
	}
}

func TestNewClientConfig(t *testing.T) {
	tests := []struct {
		name           string
		env            map[string]string
		wantConfigured bool
	}{
		{
			name: "fully configured",
			env: map[string]string{
				"KAFKA_BOOTSTRAP_SERVERS": "pkc-abc.us-east-1.aws.confluent.cloud:9092",
				"KAFKA_API_KEY":           "KEY",
				"KAFKA_API_SECRET":        "SECRET",
			},
			wantConfigured: true,
		},
		{
			name:           "no bootstrap",
			env:            map[string]string{"KAFKA_API_KEY": "KEY", "KAFKA_API_SECRET": "SECRET"},
			wantConfigured: false,
		},
		{
			name: "missing secret",
			env: map[string]string{
				"KAFKA_BOOTSTRAP_SERVERS": "broker:9092",
				"KAFKA_API_KEY":           "KEY",
			},
			wantConfigured: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setKafkaEnv(t, tt.env)
			c := NewClient(context.Background(), nil, nil)

			if got := c.ready() == nil; got != tt.wantConfigured {
				t.Fatalf("ready()==nil = %v, want %v (err=%v)", got, tt.wantConfigured, c.err)
			}
			if tt.wantConfigured {
				if c.kc == nil || c.err != nil {
					t.Fatalf("configured client should have kc!=nil and err==nil, got kc=%v err=%v", c.kc, c.err)
				}
			} else if c.kc != nil {
				t.Fatalf("unconfigured client should have kc==nil, got %v", c.kc)
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	setKafkaEnv(t, map[string]string{
		"KAFKA_BOOTSTRAP_SERVERS": "broker:9092",
		"KAFKA_API_KEY":           "KEY",
		"KAFKA_API_SECRET":        "SECRET",
	})
	c := NewClient(context.Background(), nil, nil)

	if c.defaultGroup != defaultConsumerGroup {
		t.Errorf("defaultGroup = %q, want %q", c.defaultGroup, defaultConsumerGroup)
	}
	if c.allowTopicAdmin {
		t.Error("allowTopicAdmin should default to false")
	}
}

// TestCapabilitiesNeverLeaksSecret is the security-relevant assertion: no tool
// output may contain the API secret.
func TestCapabilitiesNeverLeaksSecret(t *testing.T) {
	const secret = "super-secret-value"
	setKafkaEnv(t, map[string]string{
		"KAFKA_BOOTSTRAP_SERVERS": "broker:9092",
		"KAFKA_API_KEY":           "KEY",
		"KAFKA_API_SECRET":        secret,
		"KAFKA_ALLOW_TOPIC_ADMIN": "true",
	})
	c := NewClient(context.Background(), nil, nil)

	_, out, err := c.capabilities(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !out.Configured || !out.AuthConfigured {
		t.Fatalf("want configured+auth, got %+v", out)
	}
	if !out.AllowTopicAdmin {
		t.Error("want AllowTopicAdmin=true")
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), secret) {
		t.Fatalf("capabilities output leaked the API secret: %s", b)
	}
}

func TestCapabilitiesUnconfigured(t *testing.T) {
	setKafkaEnv(t, nil)
	c := NewClient(context.Background(), nil, nil)

	_, out, err := c.capabilities(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if out.Configured {
		t.Error("want Configured=false")
	}
	if out.ConfigError == "" {
		t.Error("want a ConfigError explaining the missing config")
	}
	// The full tool catalogue is reported even when unconfigured.
	if len(out.Tools) == 0 {
		t.Error("want the tool catalogue")
	}
}

// TestToolsReportErrorWhenUnconfigured pins that every tool returns a clean
// IsError result (not a transport error) when Kafka is not configured.
func TestToolsReportErrorWhenUnconfigured(t *testing.T) {
	setKafkaEnv(t, nil)
	c := NewClient(context.Background(), nil, nil)
	ctx := context.Background()

	if res, _, err := c.publish(ctx, nil, publishInput{Topic: "t", Value: "v"}); err != nil || !res.IsError {
		t.Errorf("publish: want IsError result, got res.IsError=%v err=%v", res.IsError, err)
	}
	if res, _, err := c.consume(ctx, nil, consumeInput{Topic: "t"}); err != nil || !res.IsError {
		t.Errorf("consume: want IsError result, got res.IsError=%v err=%v", res.IsError, err)
	}
	if res, _, err := c.listTopics(ctx, nil, struct{}{}); err != nil || !res.IsError {
		t.Errorf("listTopics: want IsError result, got res.IsError=%v err=%v", res.IsError, err)
	}
}

// TestValidationErrors pins input validation that runs before any network call,
// on a configured client (so it passes the ready() gate).
func TestValidationErrors(t *testing.T) {
	setKafkaEnv(t, map[string]string{
		"KAFKA_BOOTSTRAP_SERVERS": "broker:9092",
		"KAFKA_API_KEY":           "KEY",
		"KAFKA_API_SECRET":        "SECRET",
		// KAFKA_ALLOW_TOPIC_ADMIN left off => admin disabled.
	})
	c := NewClient(context.Background(), nil, nil)
	ctx := context.Background()

	t.Run("publish without value", func(t *testing.T) {
		res, _, err := c.publish(ctx, nil, publishInput{Topic: "t"})
		mustToolError(t, res, err, "value is required")
	})
	t.Run("publish without topic", func(t *testing.T) {
		res, _, err := c.publish(ctx, nil, publishInput{Value: "v"})
		mustToolError(t, res, err, "no topic")
	})
	t.Run("consume bad from", func(t *testing.T) {
		res, _, err := c.consume(ctx, nil, consumeInput{Topic: "t", From: "sideways"})
		mustToolError(t, res, err, "invalid from")
	})
	t.Run("create topic gated", func(t *testing.T) {
		res, _, err := c.createTopic(ctx, nil, createTopicInput{Topic: "t"})
		mustToolError(t, res, err, "KAFKA_ALLOW_TOPIC_ADMIN")
	})
	t.Run("delete topic gated", func(t *testing.T) {
		res, _, err := c.deleteTopic(ctx, nil, deleteTopicInput{Topic: "t"})
		mustToolError(t, res, err, "KAFKA_ALLOW_TOPIC_ADMIN")
	})
}

func TestClampMax(t *testing.T) {
	cases := map[int]int{0: defaultConsumeMax, -5: defaultConsumeMax, 3: 3, 100: 100, 1000: maxConsumeMax}
	for in, want := range cases {
		if got := clampMax(in); got != want {
			t.Errorf("clampMax(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestNormalizeFrom(t *testing.T) {
	ok := map[string]string{"": "group", "GROUP": "group", " earliest ": "earliest", "Latest": "latest"}
	for in, want := range ok {
		got, err := normalizeFrom(in)
		if err != nil || got != want {
			t.Errorf("normalizeFrom(%q) = %q, %v; want %q, nil", in, got, err, want)
		}
	}
	if _, err := normalizeFrom("nope"); err == nil {
		t.Error("normalizeFrom(nope) should error")
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a:9092, b:9092\n c:9092 ")
	want := []string{"a:9092", "b:9092", "c:9092"}
	if len(got) != len(want) {
		t.Fatalf("splitList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitList = %v, want %v", got, want)
		}
	}
	if len(splitList("")) != 0 {
		t.Error("splitList(\"\") should be empty")
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	in := map[string]string{"a": "1", "b": "2"}
	got := fromKafkaHeaders(toKafkaHeaders(in))
	if len(got) != len(in) {
		t.Fatalf("round-trip = %v, want %v", got, in)
	}
	for k, v := range in {
		if got[k] != v {
			t.Fatalf("round-trip[%q] = %q, want %q", k, got[k], v)
		}
	}
	if toKafkaHeaders(nil) != nil || fromKafkaHeaders(nil) != nil {
		t.Error("nil headers should round-trip to nil")
	}
}

func TestIsTruthy(t *testing.T) {
	for _, s := range []string{"true", "1", "yes", "Y", "ON"} {
		if !isTruthy(s) {
			t.Errorf("isTruthy(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "false", "0", "no", "nope"} {
		if isTruthy(s) {
			t.Errorf("isTruthy(%q) = true, want false", s)
		}
	}
}

// mustToolError asserts a tool returned a clean IsError result (err==nil) whose
// text contains want.
func mustToolError(t *testing.T, res *mcp.CallToolResult, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("want nil transport error, got %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("want IsError result, got %+v", res)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	if !strings.Contains(text, want) {
		t.Fatalf("error text %q does not contain %q", text, want)
	}
}
