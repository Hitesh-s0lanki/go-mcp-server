package event

import (
	"context"
	"sort"
	"strings"

	kafka "github.com/segmentio/kafka-go"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// confluentReplicationFactor is the replication factor Confluent Cloud requires
// for created topics; other values are rejected by the managed cluster.
const confluentReplicationFactor = 3

// registerTopicTools wires the topic listing tool plus the gated create/delete
// tools. Listing is always available; create/delete are mutating and disabled
// unless KAFKA_ALLOW_TOPIC_ADMIN is set.
func registerTopicTools(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "event_topics",
		Description: "List the topics on the Kafka cluster with their partition counts. Read-only.",
	}, c.listTopics)

	mcp.AddTool(s, &mcp.Tool{
		Name: "event_create_topic",
		Description: "Create a Kafka topic. Mutating — disabled unless KAFKA_ALLOW_TOPIC_ADMIN=true. On " +
			"Confluent Cloud the replication factor must be 3 (the default here).",
	}, c.createTopic)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "event_delete_topic",
		Description: "Delete a Kafka topic. Mutating — disabled unless KAFKA_ALLOW_TOPIC_ADMIN=true.",
	}, c.deleteTopic)
}

type topicInfo struct {
	Name       string `json:"name"`
	Partitions int    `json:"partitions"`
}

type listTopicsOutput struct {
	Topics []topicInfo `json:"topics"`
}

func (c *Client) listTopics(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listTopicsOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[listTopicsOutput]("%v", err)
	}
	resp, err := c.kc.Metadata(ctx, &kafka.MetadataRequest{})
	if err != nil {
		return toolErr[listTopicsOutput]("list topics: %v", err)
	}
	topics := make([]topicInfo, 0, len(resp.Topics))
	for _, t := range resp.Topics {
		if t.Internal {
			continue
		}
		topics = append(topics, topicInfo{Name: t.Name, Partitions: len(t.Partitions)})
	}
	sort.Slice(topics, func(i, j int) bool { return topics[i].Name < topics[j].Name })
	return jsonResult(listTopicsOutput{Topics: topics})
}

type createTopicInput struct {
	Topic             string `json:"topic" jsonschema:"the topic name to create"`
	Partitions        int    `json:"partitions,omitempty" jsonschema:"partition count (default 1)"`
	ReplicationFactor int    `json:"replication_factor,omitempty" jsonschema:"replication factor (default 3, required by Confluent Cloud)"`
}

type topicActionOutput struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
}

func (c *Client) createTopic(ctx context.Context, _ *mcp.CallToolRequest, in createTopicInput) (*mcp.CallToolResult, topicActionOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[topicActionOutput]("%v", err)
	}
	if err := c.requireTopicAdmin("event_create_topic"); err != nil {
		return toolErr[topicActionOutput]("%v", err)
	}
	if strings.TrimSpace(in.Topic) == "" {
		return toolErr[topicActionOutput]("topic is required")
	}
	partitions := in.Partitions
	if partitions <= 0 {
		partitions = 1
	}
	rf := in.ReplicationFactor
	if rf <= 0 {
		rf = confluentReplicationFactor
	}

	resp, err := c.kc.CreateTopics(ctx, &kafka.CreateTopicsRequest{
		Topics: []kafka.TopicConfig{{
			Topic:             in.Topic,
			NumPartitions:     partitions,
			ReplicationFactor: rf,
		}},
	})
	if err != nil {
		return toolErr[topicActionOutput]("create topic %q: %v", in.Topic, err)
	}
	// CreateTopics can succeed at the request level but fail per-topic.
	if e := resp.Errors[in.Topic]; e != nil {
		return toolErr[topicActionOutput]("create topic %q: %v", in.Topic, e)
	}
	return jsonResult(topicActionOutput{Topic: in.Topic, Status: "created"})
}

type deleteTopicInput struct {
	Topic string `json:"topic" jsonschema:"the topic name to delete"`
}

func (c *Client) deleteTopic(ctx context.Context, _ *mcp.CallToolRequest, in deleteTopicInput) (*mcp.CallToolResult, topicActionOutput, error) {
	if err := c.ready(); err != nil {
		return toolErr[topicActionOutput]("%v", err)
	}
	if err := c.requireTopicAdmin("event_delete_topic"); err != nil {
		return toolErr[topicActionOutput]("%v", err)
	}
	if strings.TrimSpace(in.Topic) == "" {
		return toolErr[topicActionOutput]("topic is required")
	}

	resp, err := c.kc.DeleteTopics(ctx, &kafka.DeleteTopicsRequest{Topics: []string{in.Topic}})
	if err != nil {
		return toolErr[topicActionOutput]("delete topic %q: %v", in.Topic, err)
	}
	if e := resp.Errors[in.Topic]; e != nil {
		return toolErr[topicActionOutput]("delete topic %q: %v", in.Topic, e)
	}
	return jsonResult(topicActionOutput{Topic: in.Topic, Status: "deleted"})
}
