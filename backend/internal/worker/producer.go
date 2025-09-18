package worker

import (
	"context"
	"encoding/json"
	"os"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

func NewProducer() *Producer {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		broker = "kafka:9092"
	}
	return &Producer{
		writer: &kafka.Writer{
			Addr:     kafka.TCP(broker),
			Topic:    "gc-jobs",
			Balancer: &kafka.LeastBytes{},
		},
	}
}

func (p *Producer) PublishGCJob(ctx context.Context, job GCJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Value: data,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
