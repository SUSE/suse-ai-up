package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// ANSI color codes
const (
	Reset    = "\033[0m"
	Red      = "\033[31m"
	Green    = "\033[32m"
	Yellow   = "\033[33m"
	Blue     = "\033[34m"
	Magenta  = "\033[35m"
	Cyan     = "\033[36m"
	White    = "\033[37m"
	Bold     = "\033[1m"
	BgBlue   = "\033[44m"
	BgGreen  = "\033[42m"
	BgYellow = "\033[43m"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: %s <agent-name> <api-url>\n", os.Args[0])
		fmt.Println("Example: ./example my-agent http://localhost:8910")
		os.Exit(1)
	}

	agentName := os.Args[1]
	apiURL := os.Args[2]

	// Initialize LangChain chat model
	chat, err := openai.New(
		openai.WithBaseURL(apiURL+"/v1"),
		openai.WithToken("dummy"),
		openai.WithModel(agentName),
	)
	if err != nil {
		fmt.Printf("Failed to create chat model: %v\n", err)
		os.Exit(1)
	}

	// Initialize Markdown renderer for terminal
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		fmt.Printf("Failed to initialize Markdown renderer: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s🤖 Rancher SA Chat Client%s\n", Cyan, Reset)
	fmt.Printf("Agent: %s%s%s\n", Yellow, agentName, Reset)
	fmt.Printf("API: %s%s%s\n", Yellow, apiURL, Reset)
	fmt.Println("Type 'quit' or 'exit' to end the conversation")
	fmt.Println(strings.Repeat("=", 50))

	var messages []llms.MessageContent
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Printf("\n%s👤 You:%s ", Green, Reset)
		if !scanner.Scan() {
			break
		}

		var userInput string
		userInput = strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		if userInput == "quit" || userInput == "exit" {
			fmt.Printf("%s👋 Goodbye!%s\n", Cyan, Reset)
			break
		}

		// Add user message
		messages = append(messages, llms.TextParts(llms.ChatMessageTypeHuman, userInput))

		// Generate content with LangChain
		contentResponse, err := chat.GenerateContent(context.Background(), messages)
		if err != nil {
			fmt.Printf("%s❌ Error: %v%s\n", Red, err, Reset)
			continue
		}

		// Get the full response content
		var fullContent string
		if len(contentResponse.Choices) > 0 {
			fullContent = contentResponse.Choices[0].Content
		}

		// Render the assistant response with markdown
		if fullContent != "" {
			rendered, err := r.Render(fullContent)
			if err != nil {
				// Fallback to plain text if markdown rendering fails
				fmt.Print(fullContent)
			} else {
				fmt.Print(rendered)
			}
		}

		// Add assistant response to conversation history
		messages = append(messages, llms.TextParts(llms.ChatMessageTypeAI, fullContent))

		// Print usage
		if len(contentResponse.Choices) > 0 && contentResponse.Choices[0].GenerationInfo != nil {
			printUsageInfo(contentResponse.Choices[0].GenerationInfo)
		}

		fmt.Printf("\n%s⏱️  Request completed%s\n", Cyan, Reset)
	}
}

// renderRoleContent renders content for a specific role with appropriate styling and markdown
func renderRoleContent(renderer *glamour.TermRenderer, role, content string) {
	var prefix, color string
	switch role {
	case "assistant":
		prefix = "🤖 Assistant"
		color = Blue
	case "supervisor":
		prefix = "👔 Supervisor Synthesis"
		color = Magenta
	case "worker":
		prefix = "🔧 Worker Synthesis"
		color = Yellow
	default:
		prefix = "Response"
		color = White
	}

	fmt.Printf("\n%s%s:%s\n", color, prefix, Reset)

	// Render markdown content
	rendered, err := renderer.Render(content)
	if err != nil {
		// Fallback to plain text if markdown rendering fails
		fmt.Print(content)
	} else {
		fmt.Print(rendered)
	}
}

func printUsageInfo(usage interface{}) {
	// Type assertion to map for flexible parsing
	usageMap, ok := usage.(map[string]interface{})
	if !ok {
		return
	}

	fmt.Printf("\n%s📊 Usage Statistics:%s\n", Cyan, Reset)

	// Extract worker usage
	if workerUsage, ok := usageMap["worker_usage"].(map[string]interface{}); ok {
		promptTokens := int(workerUsage["prompt_tokens"].(float64))
		completionTokens := int(workerUsage["completion_tokens"].(float64))
		totalTokens := int(workerUsage["total_tokens"].(float64))
		fmt.Printf("  %s🔧 Worker: %d prompt + %d completion = %d total tokens%s\n",
			Yellow, promptTokens, completionTokens, totalTokens, Reset)
	}

	// Extract supervisor usage
	if supervisorUsage, ok := usageMap["supervisor_usage"].(map[string]interface{}); ok {
		promptTokens := int(supervisorUsage["prompt_tokens"].(float64))
		completionTokens := int(supervisorUsage["completion_tokens"].(float64))
		totalTokens := int(supervisorUsage["total_tokens"].(float64))
		fmt.Printf("  %s👔 Supervisor: %d prompt + %d completion = %d total tokens%s\n",
			Magenta, promptTokens, completionTokens, totalTokens, Reset)
	}

	// Extract total usage
	if totalTokens, ok := usageMap["total_tokens"].(float64); ok {
		fmt.Printf("  %s📈 Total: %d tokens%s\n", Green, int(totalTokens), Reset)
	}
}
