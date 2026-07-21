package event

import (
	"context"
	"errors"
	"strings"
	"time"

	kafka "github.com/segmentio/kafka-go"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultConsumeMax     = 10
	maxConsumeMax         = 100
	defaultConsumeTimeout = 5 * time.Second
)

// registerPublishTool wires event_publish: produce one record to a topic.
func registerPublishTool(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "event_publish",
		Description: "Publish (produce) one message to a Kafka topic on Confluent Cloud. Provide a `value` and " +
			"optionally a `topic` (defaults to KAFKA_DEFAULT_TOPIC), a partition `key` (the same key always " +
			"routes to the same partition), and string `headers`. Returns the partition and offset the record " +
			"was written to.",
	}, c.publish)
}

type publishInput struct {
	Topic   string            `json:"topic,omitempty" jsonschema:"topic to publish to; defaults to KAFKA_DEFAULT_TOPIC"`
	Key     string            `json:"key,omitempty" jsonschema:"optional partition key; the same key always routes to the same partition"`
	Value   string            `json:"value" jsonschema:"the message payload (a string; JSON is fine)"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"optional string headers attached to the record"`
}

type publishOutput struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

func (c *Client) publish(ctx context.Context, _ *mcp.CallToolRequest, in publishInput) (*mcp.CallToolResult, publishOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[publishOutput]("%v", err)
	}
	topic := firstNonEmpty(in.Topic, c.defaultTopic)
	if topic == "" {
		return toolErr[publishOutput]("no topic given and KAFKA_DEFAULT_TOPIC is not set")
	}
	if in.Value == "" {
		return toolErr[publishOutput]("value is required")
	}

	partition, offset, err := c.produce(ctx, topic, in.Key, in.Value, in.Headers)
	if err != nil {
		return toolErr[publishOutput]("publish to %q: %v", topic, err)
	}
	return jsonResult(publishOutput{Topic: topic, Partition: partition, Offset: offset})
}

// registerConsumeTool wires event_consume: a bounded poll of a topic.
func registerConsumeTool(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "event_consume",
		Description: "Consume (read) up to `max` messages from a Kafka topic, returning within `timeout_ms`. " +
			"`from` selects where to read: `group` (default) uses a durable consumer group whose offsets " +
			"advance across calls so repeated calls drain new messages; `earliest`/`latest` peek partition 0 " +
			"without disturbing any group. Returns each message's partition, offset, key, value, headers and " +
			"timestamp. This is a bounded poll, not a live stream — call it again to read more.",
	}, c.consume)
}

type consumeInput struct {
	Topic     string `json:"topic,omitempty" jsonschema:"topic to read; defaults to KAFKA_DEFAULT_TOPIC"`
	Group     string `json:"group,omitempty" jsonschema:"consumer group; defaults to KAFKA_CONSUMER_GROUP. Only used when from=group"`
	Max       int    `json:"max,omitempty" jsonschema:"max messages to return (default 10, capped at 100)"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"overall poll budget in milliseconds (default 5000)"`
	From      string `json:"from,omitempty" jsonschema:"where to read from: group (default), earliest, or latest"`
}

type consumedMessage struct {
	Partition int               `json:"partition"`
	Offset    int64             `json:"offset"`
	Key       string            `json:"key,omitempty"`
	Value     string            `json:"value"`
	Headers   map[string]string `json:"headers,omitempty"`
	Timestamp string            `json:"timestamp"`
}

type consumeOutput struct {
	Topic    string            `json:"topic"`
	Group    string            `json:"group,omitempty"`
	Count    int               `json:"count"`
	Messages []consumedMessage `json:"messages"`
}

func (c *Client) consume(ctx context.Context, _ *mcp.CallToolRequest, in consumeInput) (*mcp.CallToolResult, consumeOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[consumeOutput]("%v", err)
	}
	topic := firstNonEmpty(in.Topic, c.defaultTopic)
	if topic == "" {
		return toolErr[consumeOutput]("no topic given and KAFKA_DEFAULT_TOPIC is not set")
	}
	from, err := normalizeFrom(in.From)
	if err != nil {
		return toolErr[consumeOutput]("%v", err)
	}
	limit := clampMax(in.Max)
	timeout := defaultConsumeTimeout
	if in.TimeoutMs > 0 {
		timeout = time.Duration(in.TimeoutMs) * time.Millisecond
	}
	group := firstNonEmpty(in.Group, c.defaultGroup)

	reader := c.newReader(topic, from, group)
	defer func() { _ = reader.Close() }()

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var (
		msgs      []consumedMessage
		collected []kafka.Message
	)
	for len(msgs) < limit {
		m, err := reader.FetchMessage(pollCtx)
		if err != nil {
			// A hit deadline (or cancel) is the normal end of a bounded poll:
			// return whatever we gathered rather than erroring.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				break
			}
			return toolErr[consumeOutput]("consume from %q: %v", topic, err)
		}
		collected = append(collected, m)
		msgs = append(msgs, consumedMessage{
			Partition: m.Partition,
			Offset:    m.Offset,
			Key:       string(m.Key),
			Value:     string(m.Value),
			Headers:   fromKafkaHeaders(m.Headers),
			Timestamp: m.Time.UTC().Format(time.RFC3339),
		})
	}

	// Commit progress in group mode so a later poll advances past these records.
	// Use a fresh context: pollCtx may already be past its deadline, and a hit
	// deadline must not drop the commit.
	if from == "group" && len(collected) > 0 {
		if err := reader.CommitMessages(context.Background(), collected...); err != nil {
			return toolErr[consumeOutput]("commit offsets for group %q: %v", group, err)
		}
	}

	out := consumeOutput{Topic: topic, Count: len(msgs), Messages: msgs}
	if from == "group" {
		out.Group = group
	}
	return jsonResult(out)
}

// normalizeFrom validates and defaults the `from` option.
func normalizeFrom(from string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(from)) {
	case "", "group":
		return "group", nil
	case "earliest":
		return "earliest", nil
	case "latest":
		return "latest", nil
	default:
		return "", errors.New("invalid from: use group, earliest, or latest")
	}
}

// clampMax defaults and caps the requested message count.
func clampMax(n int) int {
	if n <= 0 {
		return defaultConsumeMax
	}
	if n > maxConsumeMax {
		return maxConsumeMax
	}
	return n
}
