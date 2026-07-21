// Package event hosts the /event/mcp namespace: a worked example of Kafka
// realtime event-driven architecture over Confluent Cloud. It exposes produce
// and consume tools an agent uses to put messages on a topic and read them back,
// plus topic listing and (gated) topic admin.
//
// Connection is SASL_SSL with mechanism PLAIN: the Confluent API key is the SASL
// username and the API secret is the password, over TLS with system root CAs.
//
// Like skills and gsc (and unlike memory, which hard-requires its database), a
// missing broker or credentials is not a mount failure: the namespace always
// mounts and every tool reports the configuration problem. event_capabilities
// surfaces the current status.
package event

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hitesh-s0lanki/go-mcp-server/internal/auth"
)

const (
	// defaultConsumerGroup is used by event_consume when no group is configured
	// or passed.
	defaultConsumerGroup = "go-mcp-server"

	// dialTimeout bounds broker connect + SASL/TLS handshake for every operation.
	dialTimeout = 10 * time.Second
)

// Client wraps the Kafka connection plus the namespace's configuration. It is
// built once at mount time. When the broker or credentials are absent the
// namespace still mounts (so the rest of the server boots) but kc is nil and err
// explains why; every tool reports that error rather than panicking.
type Client struct {
	// kc drives producing, metadata and topic admin over the shared transport.
	// nil when the namespace is unconfigured.
	kc        *kafka.Client
	transport *kafka.Transport
	// dialer carries SASL/TLS for the per-call consumer readers.
	dialer *kafka.Dialer

	// keys resolves the caller's X-API-Key to its api_key_id, which owns the
	// events it publishes and is the only owner it may consume. Nil when no
	// database is configured; callerScope then fails closed rather than serving
	// unscoped events. The resolver memoizes, so per-call lookups are map hits.
	keys keyResolver

	brokers []string
	// bootstrap is the broker list joined for display in capabilities. Broker
	// hosts are not secret; the API secret is never stored on the Client.
	bootstrap string

	defaultGroup    string
	defaultTopic    string
	allowTopicAdmin bool

	authConfigured bool
	err            error
	log            *slog.Logger
}

// NewClient reads the KAFKA_* environment and builds the Kafka client.
//
// Configuration (all three required to be "configured"):
//   - KAFKA_BOOTSTRAP_SERVERS — Confluent Cloud bootstrap host:port (comma-sep).
//   - KAFKA_API_KEY / KAFKA_API_SECRET — the SASL PLAIN username/password.
//
// A missing value is captured on the returned Client rather than returned as an
// error, so the namespace mounts either way and reports the problem per-call and
// via event_capabilities. When configured, connection cleanup is tied to ctx:
// the Namespace interface has no teardown hook, so a goroutine closes idle
// broker connections when the process context is cancelled — the same technique
// memory uses to tie its workers to shutdown.
func NewClient(ctx context.Context, db *pgxpool.Pool, log *slog.Logger) *Client {
	c := &Client{
		brokers:         splitList(os.Getenv("KAFKA_BOOTSTRAP_SERVERS")),
		defaultGroup:    firstNonEmpty(os.Getenv("KAFKA_CONSUMER_GROUP"), defaultConsumerGroup),
		defaultTopic:    os.Getenv("KAFKA_DEFAULT_TOPIC"),
		allowTopicAdmin: isTruthy(os.Getenv("KAFKA_ALLOW_TOPIC_ADMIN")),
		log:             log,
	}
	c.bootstrap = strings.Join(c.brokers, ",")

	// The api-key resolver is what makes events per-user: it turns the caller's
	// key into the api_key_id stamped on published events and matched on consume.
	// A nil db (no DATABASE_URL) leaves keys nil and callerScope fails closed, so
	// no request can read events unscoped.
	if db != nil {
		c.keys = auth.NewResolver(db)
	}

	key := os.Getenv("KAFKA_API_KEY")
	secret := os.Getenv("KAFKA_API_SECRET")

	switch {
	case len(c.brokers) == 0:
		c.err = fmt.Errorf("KAFKA_BOOTSTRAP_SERVERS is not set")
	case key == "" || secret == "":
		c.err = fmt.Errorf("KAFKA_API_KEY and KAFKA_API_SECRET must both be set for Confluent Cloud (SASL_SSL/PLAIN)")
	}
	if c.err != nil {
		if log != nil {
			log.Warn("event namespace mounted without Kafka config; tools will report the configuration error", "err", c.err)
		}
		return c
	}

	// Confluent Cloud: SASL_SSL, mechanism PLAIN. The mechanism is a value that
	// implements sasl.Mechanism and must be set on both the transport (producer/
	// admin) and the dialer (consumer). TLS uses system root CAs — never skip
	// verification.
	mechanism := plain.Mechanism{Username: key, Password: secret}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	c.transport = &kafka.Transport{SASL: mechanism, TLS: tlsConfig, DialTimeout: dialTimeout}
	c.dialer = &kafka.Dialer{SASLMechanism: mechanism, TLS: tlsConfig, Timeout: dialTimeout, DualStack: true}
	c.kc = &kafka.Client{Addr: kafka.TCP(c.brokers...), Timeout: dialTimeout, Transport: c.transport}
	c.authConfigured = true

	if ctx != nil {
		go func() {
			<-ctx.Done()
			c.transport.CloseIdleConnections()
		}()
	}
	return c
}

// ready returns the configuration error if the client is not usable.
func (c *Client) ready() error {
	if c == nil || c.kc == nil {
		if c != nil && c.err != nil {
			return c.err
		}
		return fmt.Errorf("no Kafka connection configured (set KAFKA_BOOTSTRAP_SERVERS, KAFKA_API_KEY and KAFKA_API_SECRET)")
	}
	return nil
}

// callerScope resolves the request's X-API-Key into the api_key_id that owns
// the events it produces and is the only owner it may consume. It fails closed:
// a missing/malformed key, an unregistered key, or an absent resolver all return
// an error rather than falling through to a shared or unscoped view of events.
func (c *Client) callerScope(ctx context.Context, req *mcp.CallToolRequest) (string, error) {
	key, err := callerKey(req)
	if err != nil {
		return "", err
	}
	if c.keys == nil {
		return "", fmt.Errorf("event ownership scoping is unavailable: no api-key resolver is configured (set DATABASE_URL)")
	}
	return c.keys.Resolve(ctx, key)
}

// stampOwner builds the record headers for a published event: the caller's
// string headers plus an authoritative owner header. Any caller-supplied header
// named ownerHeader is dropped first, so a client cannot forge ownership of an
// event by presenting its own owner header.
func stampOwner(headers map[string]string, ownerID string) []kafka.Header {
	out := make([]kafka.Header, 0, len(headers)+1)
	for k, v := range headers {
		if k == ownerHeader {
			continue
		}
		out = append(out, kafka.Header{Key: k, Value: []byte(v)})
	}
	out = append(out, kafka.Header{Key: ownerHeader, Value: []byte(ownerID)})
	return out
}

// scopedGroup derives a per-owner consumer group from the requested base group.
// Each owner gets an independent cursor over the shared topic, so one user's
// reads never advance (or drain) another user's offsets -- the offset half of
// "unique event for unique user"; the owner-header filter on consume is the
// payload half.
func scopedGroup(base, ownerID string) string { return base + "." + ownerID }

// requireTopicAdmin returns an error unless topic create/delete are enabled.
func (c *Client) requireTopicAdmin(op string) error {
	if !c.allowTopicAdmin {
		return fmt.Errorf("%s is a mutating operation and is disabled; set KAFKA_ALLOW_TOPIC_ADMIN=true to enable it", op)
	}
	return nil
}

// produce writes one record to the topic and returns the partition and offset it
// landed at. It selects the partition with a hash balancer over the topic's
// current partitions (same key -> same partition), then produces via the
// low-level Client.Produce so the assigned offset is returned truthfully — the
// high-level batching Writer does not surface it.
func (c *Client) produce(ctx context.Context, topic, key, value string, headers map[string]string, ownerID string) (int, int64, error) {
	parts, err := c.partitionIDs(ctx, topic)
	if err != nil {
		return 0, 0, err
	}

	var keyBytes []byte
	if key != "" {
		keyBytes = []byte(key)
	}
	// Hash balancer: hashes the key to a partition, or round-robins when the key
	// is nil. It uses the partition list only for its length; partitions are
	// always the contiguous ids 0..N-1, so the result is a valid partition id.
	partition := (&kafka.Hash{}).Balance(kafka.Message{Key: keyBytes}, parts...)

	rec := kafka.Record{
		Key:     kafka.NewBytes(keyBytes),
		Value:   kafka.NewBytes([]byte(value)),
		Headers: stampOwner(headers, ownerID),
	}
	resp, err := c.kc.Produce(ctx, &kafka.ProduceRequest{
		Topic:        topic,
		Partition:    partition,
		RequiredAcks: kafka.RequireAll,
		Records:      kafka.NewRecordReader(rec),
	})
	if err != nil {
		return 0, 0, err
	}
	if resp.Error != nil {
		return 0, 0, resp.Error
	}
	return partition, resp.BaseOffset, nil
}

// partitionIDs returns a topic's partition ids from cluster metadata.
func (c *Client) partitionIDs(ctx context.Context, topic string) ([]int, error) {
	resp, err := c.kc.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		return nil, err
	}
	for _, t := range resp.Topics {
		if t.Name != topic {
			continue
		}
		if t.Error != nil {
			return nil, t.Error
		}
		ids := make([]int, 0, len(t.Partitions))
		for _, p := range t.Partitions {
			ids = append(ids, p.ID)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("topic %q has no partitions", topic)
		}
		return ids, nil
	}
	return nil, fmt.Errorf("topic %q not found", topic)
}

// newReader builds an ephemeral consumer for one bounded poll. In group mode it
// joins the consumer group (durable per-group offsets that advance across
// calls); for earliest/latest it reads partition 0 without a group so a peek does
// not disturb any group's committed offsets.
func (c *Client) newReader(topic, from, group string) *kafka.Reader {
	cfg := kafka.ReaderConfig{
		Brokers:  c.brokers,
		Topic:    topic,
		Dialer:   c.dialer,
		MaxWait:  500 * time.Millisecond,
		MinBytes: 1,
		MaxBytes: 10 << 20, // 10 MiB
	}
	switch from {
	case "earliest":
		cfg.Partition = 0
		cfg.StartOffset = kafka.FirstOffset
	case "latest":
		cfg.Partition = 0
		cfg.StartOffset = kafka.LastOffset
	default: // "group"
		cfg.GroupID = group
		cfg.StartOffset = kafka.FirstOffset
	}
	return kafka.NewReader(cfg)
}

// --- small helpers shared across the tool files ---

// jsonResult builds a tool result whose text content is the pretty-printed JSON
// of out, alongside the same value as structured output.
func jsonResult[T any](out T) (*mcp.CallToolResult, T, error) {
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return toolErr[T]("encode result: %v", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, out, nil
}

// toolErr builds a non-protocol tool error (IsError result) with a formatted
// message, so the model sees a clean explanation instead of a transport fault.
func toolErr[T any](format string, args ...any) (*mcp.CallToolResult, T, error) {
	var zero T
	msg := fmt.Sprintf(format, args...)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, zero, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// splitList splits a comma/whitespace/newline-delimited list into trimmed,
// non-empty entries. Used to parse KAFKA_BOOTSTRAP_SERVERS.
func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if t := strings.TrimSpace(f); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// toKafkaHeaders converts string headers to Kafka record headers.
func toKafkaHeaders(h map[string]string) []kafka.Header {
	if len(h) == 0 {
		return nil
	}
	out := make([]kafka.Header, 0, len(h))
	for k, v := range h {
		out = append(out, kafka.Header{Key: k, Value: []byte(v)})
	}
	return out
}

// fromKafkaHeaders converts Kafka record headers back to a string map.
func fromKafkaHeaders(h []kafka.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for _, kv := range h {
		out[kv.Key] = string(kv.Value)
	}
	return out
}
