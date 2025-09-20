package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"backend/pkg/logger"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const (
	defaultKafkaBroker = "kafka:9092"
	topicGCJobs        = "gc-jobs"
	topicFolderShare   = "folder-share-events"
	defaultPartitions  = 3
	defaultReplicas    = 1
)

type Producer struct {
	gcWriter          *kafka.Writer
	folderShareWriter *kafka.Writer
	broker            string
	once              sync.Once
	closed            bool
	mu                sync.RWMutex
}

func NewProducer() *Producer {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		broker = defaultKafkaBroker
	}

	p := &Producer{
		broker: broker,
		gcWriter: &kafka.Writer{
			Addr:         kafka.TCP(broker),
			Topic:        topicGCJobs,
			Balancer:     &kafka.LeastBytes{},
			WriteTimeout: 10 * time.Second,
			ReadTimeout:  10 * time.Second,
		},
		folderShareWriter: &kafka.Writer{
			Addr:         kafka.TCP(broker),
			Topic:        topicFolderShare,
			Balancer:     &kafka.LeastBytes{},
			WriteTimeout: 10 * time.Second,
			ReadTimeout:  10 * time.Second,
		},
	}

	// Initialize topics asynchronously to avoid blocking
	go p.ensureTopicsExist()

	return p
}

func (p *Producer) ensureTopicsExist() {
	p.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.createTopicsIfNotExist(ctx); err != nil {
			logger.L.Warn("failed to create kafka topics", zap.Error(err))
			// Don't fail, topics might exist or be created by external tools
		}
	})
}

func (p *Producer) createTopicsIfNotExist(ctx context.Context) error {
	conn, err := kafka.DialContext(ctx, "tcp", p.broker)
	if err != nil {
		return fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	controllerAddr := fmt.Sprintf("%s:%d", controller.Host, controller.Port)
	controllerConn, err := kafka.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             topicGCJobs,
			NumPartitions:     defaultPartitions,
			ReplicationFactor: defaultReplicas,
		},
		{
			Topic:             topicFolderShare,
			NumPartitions:     defaultPartitions,
			ReplicationFactor: defaultReplicas,
		},
	}

	return controllerConn.CreateTopics(topicConfigs...)
}

func (p *Producer) PublishGCJob(ctx context.Context, job GCJob) error {
	if p.isClosed() {
		return fmt.Errorf("producer is closed")
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal GC job: %w", err)
	}

	return p.gcWriter.WriteMessages(ctx, kafka.Message{
		Value: data,
		Time:  time.Now(),
	})
}

func (p *Producer) PublishFolderShareJob(ctx context.Context, job FolderShareJob) error {
	if p.isClosed() {
		return fmt.Errorf("producer is closed")
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal folder share job: %w", err)
	}

	return p.folderShareWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(job.ShareID),
		Value: data,
		Time:  time.Now(),
	})
}

func (p *Producer) PublishFolderShareEvent(ctx context.Context, event FolderShareEvent) error {
	if p.isClosed() {
		return fmt.Errorf("producer is closed")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal folder share event: %w", err)
	}

	return p.folderShareWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.ShareID),
		Value: data,
		Time:  time.Now(),
	})
}

func (p *Producer) isClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	var errors []error
	if err := p.gcWriter.Close(); err != nil {
		errors = append(errors, fmt.Errorf("failed to close GC writer: %w", err))
	}
	if err := p.folderShareWriter.Close(); err != nil {
		errors = append(errors, fmt.Errorf("failed to close folder share writer: %w", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing producer: %v", errors)
	}

	return nil
}
