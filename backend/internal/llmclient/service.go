package llmclient

import (
	"context"
	"io"
	"time"

	"go.uber.org/zap"
)

// TriggerSummary starts async summary generation
func (s *service) TriggerSummary(ctx context.Context, fileID, objectPath, filename string) {
	s.summaryMu.RLock()
	if result, exists := s.summaryCache[fileID]; exists {
		if result.Status == StatusProcessing || result.Status == StatusCompleted {
			s.summaryMu.RUnlock()
			return
		}
	}
	s.summaryMu.RUnlock()
	s.summaryMu.Lock()
	s.summaryCache[fileID] = &SummaryResult{
		FileID:    fileID,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.summaryMu.Unlock()

	go s.generateSummary(fileID, objectPath, filename)
}

// GetSummary returns summary status
func (s *service) GetSummary(ctx context.Context, fileID string) (*SummaryResult, error) {
	s.summaryMu.RLock()
	defer s.summaryMu.RUnlock()

	result, exists := s.summaryCache[fileID]
	if !exists {
		return &SummaryResult{
			FileID: fileID,
			Status: StatusNotFound,
		}, nil
	}

	return result, nil
}

// generateSummary performs actual summarization
func (s *service) generateSummary(fileID, objectPath, filename string) {
	ctx := context.Background()

	s.logger.Info("Starting summary generation",
		zap.String("file_id", fileID),
		zap.String("filename", filename),
	)

	s.updateSummaryStatus(fileID, StatusProcessing, "", "")

	reader, err := s.storage.GetObjectReader(ctx, objectPath)
	if err != nil {
		s.logger.Error("Failed to get object", zap.String("file_id", fileID), zap.Error(err))
		s.updateSummaryStatus(fileID, StatusFailed, "", "Failed to access file")
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		s.logger.Error("Failed to read file", zap.String("file_id", fileID), zap.Error(err))
		s.updateSummaryStatus(fileID, StatusFailed, "", "Failed to read file")
		return
	}

	if len(content) < 5 || string(content[:5]) != "%PDF-" {
		s.updateSummaryStatus(fileID, StatusFailed, "", "Only PDF files supported")
		return
	}
	summaryCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	summary, err := s.llmClient.Summarize(summaryCtx, filename, content)
	if err != nil {
		s.logger.Error("Summarization failed", zap.String("file_id", fileID), zap.Error(err))
		s.updateSummaryStatus(fileID, StatusFailed, "", err.Error())
		return
	}

	s.updateSummaryStatus(fileID, StatusCompleted, summary, "")
	s.logger.Info("Summary completed", zap.String("file_id", fileID))
}

// ProcessForChat starts indexing for chat
func (s *service) ProcessForChat(ctx context.Context, fileID, objectPath, filename string) error {
	s.chatMu.RLock()
	if result, exists := s.chatProcessing[fileID]; exists {
		if result.Status == ChatStatusIndexing || result.Status == ChatStatusReady {
			s.chatMu.RUnlock()
			return nil
		}
	}
	s.chatMu.RUnlock()

	s.chatMu.Lock()
	s.chatProcessing[fileID] = &ChatProcessingResult{
		FileID:    fileID,
		Status:    ChatStatusNotStarted,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.chatMu.Unlock()

	go s.indexDocument(fileID, objectPath, filename)

	return nil
}

// GetChatStatus returns chat processing status
func (s *service) GetChatStatus(ctx context.Context, fileID string) (*ChatProcessingResult, error) {
	s.chatMu.RLock()
	result, exists := s.chatProcessing[fileID]
	s.chatMu.RUnlock()

	if exists {
		return result, nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ready, status, err := s.llmClient.GetDocumentStatus(checkCtx, fileID)
	if err != nil {
		return &ChatProcessingResult{
			FileID: fileID,
			Status: ChatStatusNotStarted,
		}, nil
	}

	var chatStatus ChatProcessingStatus
	if ready {
		chatStatus = ChatStatusReady
	} else if status == "indexing" {
		chatStatus = ChatStatusIndexing
	} else {
		chatStatus = ChatStatusNotStarted
	}

	return &ChatProcessingResult{
		FileID:    fileID,
		Status:    chatStatus,
		UpdatedAt: time.Now(),
		CreatedAt: time.Now(),
	}, nil
}

// Chat sends a question
func (s *service) Chat(ctx context.Context, fileID, question string) (*ChatResponse, error) {
	s.chatMu.RLock()
	result, exists := s.chatProcessing[fileID]
	s.chatMu.RUnlock()

	if exists && result.Status != ChatStatusReady {
		return nil, ErrDocumentNotReady
	}

	chatCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	answer, err := s.llmClient.Chat(chatCtx, fileID, question)
	if err != nil {
		return nil, err
	}

	response := &ChatResponse{
		Answer:    answer,
		FileID:    fileID,
		Question:  question,
		Timestamp: time.Now(),
	}

	s.addToHistory(fileID, question, answer)

	return response, nil
}

// GetChatHistory returns chat history
func (s *service) GetChatHistory(ctx context.Context, fileID string) (*ChatHistory, error) {
	s.historyMu.RLock()
	defer s.historyMu.RUnlock()

	history, exists := s.historyCache[fileID]
	if !exists {
		return &ChatHistory{
			FileID:   fileID,
			Messages: []ChatMessage{},
		}, nil
	}

	return history, nil
}

// ClearChatHistory clears history
func (s *service) ClearChatHistory(ctx context.Context, fileID string) error {
	s.historyMu.Lock()
	delete(s.historyCache, fileID)
	s.historyMu.Unlock()
	return nil
}

// indexDocument performs document indexing
func (s *service) indexDocument(fileID, objectPath, filename string) {
	ctx := context.Background()

	s.logger.Info("Starting document indexing", zap.String("file_id", fileID))

	s.updateChatStatus(fileID, ChatStatusIndexing, "", "Indexing...")

	reader, err := s.storage.GetObjectReader(ctx, objectPath)
	if err != nil {
		s.logger.Error("Failed to get object", zap.Error(err))
		s.updateChatStatus(fileID, ChatStatusFailed, "Failed to access file", "")
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		s.logger.Error("Failed to read file", zap.Error(err))
		s.updateChatStatus(fileID, ChatStatusFailed, "Failed to read file", "")
		return
	}

	if len(content) < 5 || string(content[:5]) != "%PDF-" {
		s.updateChatStatus(fileID, ChatStatusFailed, "Only PDF supported", "")
		return
	}

	processCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	err = s.llmClient.ProcessDocument(processCtx, fileID, filename, content)
	if err != nil {
		s.logger.Error("Indexing failed", zap.Error(err))
		s.updateChatStatus(fileID, ChatStatusFailed, err.Error(), "")
		return
	}

	s.updateChatStatus(fileID, ChatStatusReady, "", "Ready for chat")
	s.logger.Info("Document indexed", zap.String("file_id", fileID))
}

// Helper methods
func (s *service) updateSummaryStatus(fileID string, status SummaryStatus, summary, errMsg string) {
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()

	if result, exists := s.summaryCache[fileID]; exists {
		result.Status = status
		result.Summary = summary
		result.Error = errMsg
		result.UpdatedAt = time.Now()
	} else {
		s.summaryCache[fileID] = &SummaryResult{
			FileID:    fileID,
			Status:    status,
			Summary:   summary,
			Error:     errMsg,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
}

func (s *service) updateChatStatus(fileID string, status ChatProcessingStatus, errMsg, message string) {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()

	if result, exists := s.chatProcessing[fileID]; exists {
		result.Status = status
		result.Error = errMsg
		result.Message = message
		result.UpdatedAt = time.Now()
	} else {
		s.chatProcessing[fileID] = &ChatProcessingResult{
			FileID:    fileID,
			Status:    status,
			Error:     errMsg,
			Message:   message,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
}

func (s *service) addToHistory(fileID, question, answer string) {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()

	history, exists := s.historyCache[fileID]
	if !exists {
		history = &ChatHistory{
			FileID:    fileID,
			Messages:  []ChatMessage{},
			CreatedAt: time.Now(),
		}
		s.historyCache[fileID] = history
	}

	history.Messages = append(history.Messages, ChatMessage{
		Question:  question,
		Answer:    answer,
		Timestamp: time.Now(),
	})
	history.UpdatedAt = time.Now()

	if len(history.Messages) > 10 {
		history.Messages = history.Messages[len(history.Messages)-10:]
	}
}

// Cleanup methods
func (s *service) CleanupOld(maxAge time.Duration) {
	s.summaryMu.Lock()
	now := time.Now()
	for fileID, result := range s.summaryCache {
		if now.Sub(result.UpdatedAt) > maxAge {
			delete(s.summaryCache, fileID)
		}
	}
	s.summaryMu.Unlock()

	s.chatMu.Lock()
	for fileID, result := range s.chatProcessing {
		if now.Sub(result.UpdatedAt) > maxAge {
			delete(s.chatProcessing, fileID)
		}
	}
	s.chatMu.Unlock()

	s.historyMu.Lock()
	for fileID, history := range s.historyCache {
		if now.Sub(history.UpdatedAt) > maxAge {
			delete(s.historyCache, fileID)
		}
	}
	s.historyMu.Unlock()
}

func (s *service) StartCleanupRoutine(interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			s.CleanupOld(maxAge)
		}
	}()
}
