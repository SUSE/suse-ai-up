package proxy

import (
	"context"
	"fmt"
	"log"
)

// AdvancedFeaturesHandler handles advanced MCP protocol features
type AdvancedFeaturesHandler struct {
	samplingHandler    func(*SamplingRequest) *SamplingResponse
	elicitationHandler func(*ElicitationRequest) *ElicitationResponse
	loggingHandler     func(*LogMessage)
	progressHandler    func(*ProgressNotification)
}

// SamplingRequest represents an LLM sampling request from backend
type SamplingRequest struct {
	Messages      []SamplingMessage      `json:"messages"`
	MaxTokens     int                    `json:"maxTokens,omitempty"`
	Temperature   float64                `json:"temperature,omitempty"`
	StopSequences []string               `json:"stopSequences,omitempty"`
	SystemPrompt  string                 `json:"systemPrompt,omitempty"`
	IncludeUsage  bool                   `json:"includeUsage,omitempty"`
	Metadata      map[string]interface{} `json:"_meta,omitempty"`
}

// SamplingMessage represents a message in sampling request
type SamplingMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SamplingResponse represents the response to a sampling request
type SamplingResponse struct {
	Model      string            `json:"model"`
	StopReason string            `json:"stopReason,omitempty"`
	Content    []SamplingContent `json:"content"`
	Usage      *SamplingUsage    `json:"usage,omitempty"`
}

// SamplingContent represents content in sampling response
type SamplingContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// SamplingUsage represents token usage information
type SamplingUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// ElicitationRequest represents a user input request from backend
type ElicitationRequest struct {
	Message  string                 `json:"message"`
	Metadata map[string]interface{} `json:"_meta,omitempty"`
}

// ElicitationResponse represents the response to an elicitation request
type ElicitationResponse struct {
	Content string `json:"content"`
}

// LogMessage represents a log message from backend
type LogMessage struct {
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ProgressNotification represents a progress update from backend
type ProgressNotification struct {
	ProgressToken string  `json:"progressToken"`
	Progress      float64 `json:"progress"`
	Total         float64 `json:"total,omitempty"`
	Message       string  `json:"message,omitempty"`
}

// SetAdvancedFeaturesHandlers sets the handlers for advanced MCP features
func (pc *ProxyClient) SetAdvancedFeaturesHandlers(handler *AdvancedFeaturesHandler) {
	pc.advancedFeatures = handler
}

// ForwardSampling forwards a sampling request to the client
func (pc *ProxyClient) ForwardSampling(req *SamplingRequest) *SamplingResponse {
	if pc.advancedFeatures != nil && pc.advancedFeatures.samplingHandler != nil {
		return pc.advancedFeatures.samplingHandler(req)
	}
	// Default: return empty response (client doesn't support sampling)
	return &SamplingResponse{
		Content: []SamplingContent{},
	}
}

// ForwardElicitation forwards an elicitation request to the client
func (pc *ProxyClient) ForwardElicitation(req *ElicitationRequest) *ElicitationResponse {
	if pc.advancedFeatures != nil && pc.advancedFeatures.elicitationHandler != nil {
		return pc.advancedFeatures.elicitationHandler(req)
	}
	// Default: return empty response
	return &ElicitationResponse{
		Content: "",
	}
}

// ForwardLogging forwards a log message to the client
func (pc *ProxyClient) ForwardLogging(msg *LogMessage) {
	if pc.advancedFeatures != nil && pc.advancedFeatures.loggingHandler != nil {
		pc.advancedFeatures.loggingHandler(msg)
	} else {
		// Default: log to server console
		log.Printf("[%s] %s", msg.Level, msg.Message)
	}
}

// ForwardProgress forwards a progress notification to the client
func (pc *ProxyClient) ForwardProgress(notification *ProgressNotification) {
	if pc.advancedFeatures != nil && pc.advancedFeatures.progressHandler != nil {
		pc.advancedFeatures.progressHandler(notification)
	}
	// Progress notifications are typically fire-and-forget
}

// HandleAdvancedFeaturesRequest handles advanced MCP feature requests from backend
func (pc *ProxyClient) HandleAdvancedFeaturesRequest(ctx context.Context, method string, params interface{}) (interface{}, error) {
	switch method {
	case "sampling/createMessage":
		if samplingReq, ok := params.(map[string]interface{}); ok {
			req := &SamplingRequest{
				Messages:      parseSamplingMessages(samplingReq["messages"]),
				MaxTokens:     int(samplingReq["maxTokens"].(float64)),
				Temperature:   samplingReq["temperature"].(float64),
				StopSequences: parseStringSlice(samplingReq["stopSequences"]),
			}
			return pc.ForwardSampling(req), nil
		}

	case "elicitation/askUser":
		if elicitationReq, ok := params.(map[string]interface{}); ok {
			req := &ElicitationRequest{
				Message: elicitationReq["message"].(string),
			}
			return pc.ForwardElicitation(req), nil
		}

	case "logging/log":
		if logReq, ok := params.(map[string]interface{}); ok {
			msg := &LogMessage{
				Level:   logReq["level"].(string),
				Message: logReq["message"].(string),
				Data:    logReq["data"].(map[string]interface{}),
			}
			pc.ForwardLogging(msg)
			return map[string]interface{}{}, nil
		}

	case "progress/report":
		if progressReq, ok := params.(map[string]interface{}); ok {
			notification := &ProgressNotification{
				ProgressToken: progressReq["progressToken"].(string),
				Progress:      progressReq["progress"].(float64),
			}
			if total, ok := progressReq["total"].(float64); ok {
				notification.Total = total
			}
			if message, ok := progressReq["message"].(string); ok {
				notification.Message = message
			}
			pc.ForwardProgress(notification)
			return map[string]interface{}{}, nil
		}
	}

	return nil, fmt.Errorf("unsupported advanced feature method: %s", method)
}

// Helper functions for parsing
func parseSamplingMessages(messages interface{}) []SamplingMessage {
	if msgSlice, ok := messages.([]interface{}); ok {
		result := make([]SamplingMessage, len(msgSlice))
		for i, msg := range msgSlice {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				result[i] = SamplingMessage{
					Role:    msgMap["role"].(string),
					Content: msgMap["content"].(string),
				}
			}
		}
		return result
	}
	return []SamplingMessage{}
}

func parseStringSlice(slice interface{}) []string {
	if strSlice, ok := slice.([]interface{}); ok {
		result := make([]string, len(strSlice))
		for i, s := range strSlice {
			result[i] = s.(string)
		}
		return result
	}
	return []string{}
}
