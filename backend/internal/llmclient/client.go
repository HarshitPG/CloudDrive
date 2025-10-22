package llmclient

import (
	"context"
	"fmt"
	"time"

	pb "backend/internal/llmclient/proto"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	conn    *grpc.ClientConn
	client  pb.LLMServiceClient
	cb      *gobreaker.CircuitBreaker
	token   string
	timeout time.Duration
}

// Config holds client configuration
type Config struct {
	Address string
	Token   string
	Timeout time.Duration
}

// NewClient creates a new LLM gRPC client with circuit breaker
func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	// Create circuit breaker
	cbSettings := gobreaker.Settings{
		Name:        "llm-grpc",
		MaxRequests: 3,
		Interval:    time.Minute,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	}
	cb := gobreaker.NewCircuitBreaker(cbSettings)

	// Connect to gRPC server using NewClient (non-deprecated)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LLM service: %w", err)
	}

	// Wait for connection to be ready
	grpcCtx, grpcCancel := context.WithTimeout(ctx, 5*time.Second)
	defer grpcCancel()

	if err := waitForReady(grpcCtx, conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("LLM service not ready: %w", err)
	}

	return &Client{
		conn:    conn,
		client:  pb.NewLLMServiceClient(conn),
		cb:      cb,
		token:   cfg.Token,
		timeout: cfg.Timeout,
	}, nil
}

// waitForReady waits for the connection to be ready (replaces WithBlock)
func waitForReady(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()
		if state == 2 { // connectivity.Ready
			return nil
		}
		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("connection timeout")
		}
	}
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// contextWithAuth adds bearer token to context
func (c *Client) contextWithAuth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}

// Summarize generates a quick summary of the document
func (c *Client) Summarize(ctx context.Context, filename string, content []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var summary string
	_, err := c.cb.Execute(func() (interface{}, error) {
		ctx = c.contextWithAuth(ctx)
		resp, err := c.client.Summarize(ctx, &pb.SummarizeRequest{
			Filename: filename,
			Content:  content,
		})
		if err != nil {
			return nil, err
		}
		if resp.Error != "" {
			return nil, fmt.Errorf("summarization error: %s", resp.Error)
		}
		summary = resp.Summary
		return nil, nil
	})

	return summary, err
}

// ProcessDocument indexes a document for chat
func (c *Client) ProcessDocument(ctx context.Context, fileID, filename string, content []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second) // Longer timeout for indexing
	defer cancel()

	_, err := c.cb.Execute(func() (interface{}, error) {
		ctx = c.contextWithAuth(ctx)
		resp, err := c.client.ProcessDocument(ctx, &pb.ProcessDocumentRequest{
			FileId:   fileID,
			Filename: filename,
			Content:  content,
		})
		if err != nil {
			return nil, err
		}
		if !resp.Success {
			return nil, fmt.Errorf("processing failed: %s", resp.Error)
		}
		return nil, nil
	})

	return err
}

// Chat asks a question about an indexed document
func (c *Client) Chat(ctx context.Context, fileID, question string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var answer string
	_, err := c.cb.Execute(func() (interface{}, error) {
		ctx = c.contextWithAuth(ctx)
		resp, err := c.client.Chat(ctx, &pb.ChatRequest{
			FileId:   fileID,
			Question: question,
		})
		if err != nil {
			return nil, err
		}
		if resp.Error != "" {
			return nil, fmt.Errorf("chat error: %s", resp.Error)
		}
		answer = resp.Answer
		return nil, nil
	})

	return answer, err
}

// GetDocumentStatus checks if a document is ready for chat
func (c *Client) GetDocumentStatus(ctx context.Context, fileID string) (bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var ready bool
	var status string
	_, err := c.cb.Execute(func() (interface{}, error) {
		ctx = c.contextWithAuth(ctx)
		resp, err := c.client.GetDocumentStatus(ctx, &pb.DocumentStatusRequest{
			FileId: fileID,
		})
		if err != nil {
			return nil, err
		}
		ready = resp.Ready
		status = resp.Status
		return nil, nil
	})

	return ready, status, err
}

// GetCircuitBreakerState returns current circuit breaker state
func (c *Client) GetCircuitBreakerState() gobreaker.State {
	return c.cb.State()
}
