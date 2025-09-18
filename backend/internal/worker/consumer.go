package worker

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"backend/internal/storage"

	"github.com/segmentio/kafka-go"
)

type GCWorker struct {
	reader  *kafka.Reader
	storage *storage.MinioStorage
}

func NewGCWorker(st *storage.MinioStorage) *GCWorker {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		broker = "kafka:9092"
	}

	return &GCWorker{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:  []string{broker},
			GroupID:  "gc-workers",
			Topic:    "gc-jobs",
			MinBytes: 1,
			MaxBytes: 10e6,
		}),
		storage: st,
	}
}

func (w *GCWorker) Run(ctx context.Context) {
	for {
		m, err := w.reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("GCWorker: read error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		var job GCJob
		if err := json.Unmarshal(m.Value, &job); err != nil {
			log.Printf("GCWorker: bad job: %v", err)
			continue
		}

		log.Printf("GCWorker: deleting blobKey=%s (contentID=%s)", job.BlobKey, job.ContentID)
		err = w.storage.Client.RemoveObject(ctx, w.storage.Bucket, job.BlobKey, storage.MinioRemoveOpts())
		if err != nil {
			log.Printf("GCWorker: failed delete blobKey=%s: %v", job.BlobKey, err)
		}
	}
}

func (w *GCWorker) Close() error {
	return w.reader.Close()
}
