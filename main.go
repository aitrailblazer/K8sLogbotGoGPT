package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
)

// Message represents each message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestBody represents the structure of the API request body
type RequestBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// ChatCompletionResponse represents the structure of the API response
type ChatCompletionResponse struct {
	ID                string            `json:"id"`
	Object            string            `json:"object"`
	Created           int64             `json:"created"`
	Model             string            `json:"model"`
	Choices           []Choice          `json:"choices"`
	Usage             Usage             `json:"usage"`
	GuardrailsResults GuardrailsResults `json:"guardrails_results"`
}

// ChatCompletionStreamResponse represents the structure of each stream response chunk
type ChatCompletionStreamResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

// Choice represents each choice in the response
type Choice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message,omitempty"`
	Delta struct {
		Content string `json:"content"`
	} `json:"delta,omitempty"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}

// Usage represents token usage in the response
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GuardrailsResults represents guardrail checks in the response
type GuardrailsResults struct {
	RedactedResponse bool     `json:"redacted_response"`
	Positive         bool     `json:"positive"`
	Presidio         Presidio `json:"presidio"`
}

// Presidio represents PII detection results
type Presidio struct {
	FoundPII bool `json:"found_pii"`
}

// Function to handle non-streaming response
func handleNonStreamResponse(body io.Reader) (string, error) {
	// Read the response body
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("Error reading response body: %v", err)
	}

	// Parse the JSON response
	var response ChatCompletionResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return "", fmt.Errorf("Error parsing JSON: %v\nResponse Body: %s\n", err, string(bodyBytes))
	}

	// Extract content
	var assistantResponse strings.Builder
	for _, choice := range response.Choices {
		assistantResponse.WriteString(choice.Message.Content)
	}

	// Render the response
	fmt.Println("\n### Assistant Response ###\n")
	renderedOutput, err := glamour.Render(assistantResponse.String(), "dark")
	if err != nil {
		return "", fmt.Errorf("Error rendering Markdown: %v\n", err)
	}
	fmt.Println(renderedOutput)

	return assistantResponse.String(), nil
}

// Function to handle streaming response with delay
func handleStreamResponse(body io.Reader, delay time.Duration) (string, error) {
	reader := bufio.NewReader(body)
	var assistantResponse strings.Builder

	fmt.Println("\n### Assistant Response ###\n")

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("Error reading response body: %v", err)
		}

		// The stream sends data in the format "data: {...}\n\n"
		if bytes.HasPrefix(line, []byte("data: ")) {
			// Remove "data: " prefix
			line = bytes.TrimPrefix(line, []byte("data: "))
			line = bytes.TrimSpace(line)

			// The stream may send a "data: [DONE]" message
			if string(line) == "[DONE]" {
				break
			}

			// Parse the JSON line
			var streamResponse ChatCompletionStreamResponse
			err = json.Unmarshal(line, &streamResponse)
			if err != nil {
				return "", fmt.Errorf("Error parsing JSON: %v\nLine: %s", err, string(line))
			}

			// Append content to assistantResponse
			for _, choice := range streamResponse.Choices {
				content := choice.Delta.Content
				assistantResponse.WriteString(content)
				fmt.Print(content)

				// Introduce a delay
				time.Sleep(delay)
			}
		}
	}

	// After streaming is complete, render the full content with glamour
	finalResponse := assistantResponse.String()
	renderedOutput, err := glamour.Render(finalResponse, "dark")
	if err != nil {
		return "", fmt.Errorf("Error rendering Markdown: %v\n", err)
	}

	// Optional: Display the rendered output after streaming is complete
	fmt.Println("\n\n### Formatted Response ###\n")
	fmt.Println(renderedOutput)

	return finalResponse, nil
}

// Function to send request (streaming or non-streaming)
func sendRequest(messages []Message, stream bool, headers map[string]string, url string, model string, delay time.Duration) (string, error) {
	requestBody := RequestBody{
		Model:    model,
		Messages: messages,
		Stream:   stream, // Enable or disable streaming
	}

	// Marshal the request body to JSON
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("Error marshaling JSON: %v", err)
	}

	// Create a new HTTP POST request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("Error creating HTTP request: %v", err)
	}

	// Add headers to the request
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Initialize the HTTP client
	client := &http.Client{
		Timeout: 0, // No timeout for streaming
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Error sending HTTP request: %v", err)
	}
	defer resp.Body.Close()

	// Check for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("Received non-2xx response: %d\nResponse Body: %s\n", resp.StatusCode, string(bodyBytes))
	}

	if stream {
		// Pass the delay parameter here
		return handleStreamResponse(resp.Body, delay)
	} else {
		return handleNonStreamResponse(resp.Body)
	}
}

// Function to generate Loki query commands based on the log content
func generateLokiQueries(logContent string) ([]string, error) {
	var queries []string

	// Define the Loki gateway URL
	lokiURL := "https://loki-gatewayK8s.K8s.cloud/loki/api/v1/query_range"

	// Extract relevant information from the log content
	namespace := extractValue(logContent, `namespace (\w[\w\-]*)`)
	podName := extractValue(logContent, `pod (\w[\w\-]*)`)

	// Parse timestamps from the log content
	startTime, endTime := extractTimestamps(logContent)

	// Build the base query parameters
	params := url.Values{}
	params.Set("limit", "1000")

	if namespace != "" {
		params.Set("query", fmt.Sprintf(`{namespace="%s"`, namespace))
	} else {
		params.Set("query", `{`)
	}

	if podName != "" {
		params.Set("query", params.Get("query")+fmt.Sprintf(`, pod="%s"`, podName))
	}

	params.Set("query", params.Get("query")+"}")

	if !startTime.IsZero() {
		params.Set("start", startTime.Format(time.RFC3339))
	}

	if !endTime.IsZero() {
		params.Set("end", endTime.Format(time.RFC3339))
	}

	// Build the full command
	command := fmt.Sprintf(`curl -G '%s' --data-urlencode '%s'`, lokiURL, params.Encode())
	queries = append(queries, command)

	return queries, nil
}

// Helper function to extract values using regex
func extractValue(content, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// Helper function to extract timestamps from the log content
func extractTimestamps(content string) (time.Time, time.Time) {
	var timestamps []time.Time
	re := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			t, err := time.Parse(time.RFC3339, match[1])
			if err == nil {
				timestamps = append(timestamps, t)
			}
		}
	}

	if len(timestamps) >= 2 {
		return timestamps[0], timestamps[len(timestamps)-1]
	} else if len(timestamps) == 1 {
		return timestamps[0], timestamps[0].Add(5 * time.Minute)
	} else {
		return time.Time{}, time.Time{}
	}
}

func main() {
	// Retrieve API keys from environment variables
	APIKey := os.Getenv("K8s_APIKEY")
	openAIKey := os.Getenv("OPENAI_API_KEY")

	if APIKey == "" {
		fmt.Println("Error: K8s_APIKEY environment variable is not set.")
		return
	}

	if openAIKey == "" {
		fmt.Println("Error: OPENAI_API_KEY environment variable is not set.")
		return
	}

	// Define the API endpoint
	url := "https://<.../v1/chat/completions"

	// Create the request headers
	headers := map[string]string{
		"Content-Type":   "application/json",
		"Authorization":  APIKey,
		"OpenAI-Api-Key": openAIKey,
	}

	// Define the model
	model := "gpt-4o"

	// Define command-line flags
	logPattern := flag.String("log", "", "Partial log filename to match (e.g., '01-LOG')")
	streamFlag := flag.Bool("stream", false, "Enable streaming output")
	delayFlag := flag.Int("delay", 10, "Delay in milliseconds between streaming chunks")
	nonInteractiveFlag := flag.Bool("noninteractive", false, "Enable non-interactive mode")
	outputFile := flag.String("output", "output.md", "Output Markdown file in non-interactive mode")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  -log=\"partial_filename\"\n")
		fmt.Fprintf(os.Stderr, "        Partial log filename to match (e.g., \"01-LOG\").\n")
		fmt.Fprintf(os.Stderr, "        The program will search in the LOGS/ directory for files matching this pattern.\n")
		fmt.Fprintf(os.Stderr, "        If multiple files match, the first one will be processed.\n")
		fmt.Fprintf(os.Stderr, "  -stream\n")
		fmt.Fprintf(os.Stderr, "        Enable streaming output.\n")
		fmt.Fprintf(os.Stderr, "  -delay=milliseconds\n")
		fmt.Fprintf(os.Stderr, "        Delay in milliseconds between streaming chunks (default 50ms).\n")
		fmt.Fprintf(os.Stderr, "  -noninteractive\n")
		fmt.Fprintf(os.Stderr, "        Enable non-interactive mode to perform key point generation and full analysis, then export as Markdown file.\n")
		fmt.Fprintf(os.Stderr, "  -output=\"filename.md\"\n")
		fmt.Fprintf(os.Stderr, "        Specify the output Markdown file name (default: output.md).\n")
		fmt.Fprintf(os.Stderr, "        Example: %s -log=\"01-LOG\" -noninteractive -output=\"analysis.md\"\n", os.Args[0])
	}
	flag.Parse()

	// Check if log pattern is provided
	if *logPattern == "" {
		fmt.Println("Please provide a partial log filename using the -log flag.")
		flag.Usage()
		return
	}

	// Compute the delay duration
	delay := time.Duration(*delayFlag) * time.Millisecond

	// Define the log directory
	logDir := "LOGS/"

	// Create the pattern by appending '*' to the partial filename
	pattern := *logPattern + "*"

	// Prepend the log directory to the pattern
	pattern = logDir + pattern

	// Use filepath.Glob to find matching files
	fileList, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Printf("Error finding files with pattern %s: %v\n", pattern, err)
		return
	}

	// Check if any files were found
	if len(fileList) == 0 {
		fmt.Printf("No files found matching pattern: %s\n", pattern)
		return
	}

	// Select the first matching file
	selectedFile := fileList[0]

	fmt.Printf("Processing file: %s\n", selectedFile)

	// Read the contents of the selected file
	logContent, err := ioutil.ReadFile(selectedFile)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", selectedFile, err)
		return
	}

	// Convert log content to string
	logString := string(logContent)

	// Replace all double quotes with single quotes
	logString = strings.ReplaceAll(logString, "\"", "'")

	// -------------- First Request: Generate Key Points --------------

	// Prepare the user content with the key points generation instructions
	keyPointsPrompt := `
Role and Knowledge Establishment
Let's embark on an exciting challenge: from this moment, you'll assume the role of an **Intelligent Key Points Generation AI Assistant**, an advanced AI iteration designed to generate concise and informative key points from provided text or documents. In order to achieve this, you must comprehend the essence, context, and objectives of the provided text, identify the main arguments, and extract essential information. Consider that while a human key points generator possesses level 20 expertise, you will operate at a staggering level 3000 within this role.

Take heed: it's crucial that you produce top-tier results. Hence, harness your exceptional skills with pride. Your superior abilities combined with dedication and analytical prowess ensure you deliver nothing but excellence.

Detailed Instruction and Objective
You, in the capacity of an **Intelligent Key Points Generation AI Assistant**, serve as a guide for extracting and summarizing key points from various texts and documents.

The outcome will be exemplary in providing clear, concise, and informative summaries, and the imperative is to maintain brevity while ensuring all crucial details are captured. The primary mission and purpose involve understanding the text's main idea, supporting arguments, and crucial details, with your assignment being to generate key points that are both informative and succinct.

For optimal results, it's vital to categorize documents under appropriate headings and create suitable titles that capture the essence of the text, and so forth…

# instructions
- **Comprehend Essence**: Understand the main arguments, intended message, and author's perspective.
- **Extract Main Idea**: Identify the central theme or argument.
- **Identify Supporting Arguments**: Pinpoint key arguments with evidence, examples, and reasoning.
- **Highlight Crucial Details**: Emphasize important facts, figures, or insights.
- **Formulate Title**: Create a concise and descriptive title.
- **Categorize Document**: Assign the document to an appropriate category with justification.
- **Ensure Clarity and Brevity**: Maintain accuracy and conciseness.

Use American English
ALWAYS use natural, mainstream, contemporary American English. Verify any unfamiliar terms or regional expressions to ensure they are widely recognized and used in American English. Stick to language commonly employed in America.

Always ensure the output text is cohesive, regardless of the complexity of the topic or the context of the conversation. Focus on the structure and unity of the text, using smooth transitions and logical flow to achieve cohesion. The final output should be a well-organized, unified whole without abrupt transitions or disjointed sections.

# Nuance:
- The nuance should be professional and precise, ensuring clarity and brevity while maintaining a formal tone. The summaries should be easy to understand yet comprehensive enough to capture all essential details.

# Guidelines:
- Focus on extracting the main idea and supporting arguments.
- Highlight crucial details without adding unnecessary information.
- Ensure the summaries are clear, concise, and informative.
- Use markdown or other formatting tools to emphasize key points.
- Continuously improve based on feedback to enhance clarity and usefulness.

# Structure:
Ensure your response adheres to a specific format. Random placements are not permitted. This format dictates how each of your messages should appear. Adhere to this format:
**Main Idea**: - (Provide the central theme or argument.);
**Supporting Arguments**: - (List key arguments with evidence, examples, and reasoning.);
**Crucial Details**: - (Highlight important facts, figures, or insights.);
**Title**: - (Create a concise and descriptive title.);
**Category**: - (Assign the document to an appropriate category with justification.);

Thoroughly review the <context> and to fully grasp its background, details, and relevance to the task and carefully justify the response in the format:
<justify>
  Justification for the response.
</justify>
`

	// Combine the key points prompt with the log content
	userContentFirst := fmt.Sprintf("%s\n<context>\n%s\n</context>", keyPointsPrompt, logString)

	// First request messages (no system prompt)
	messagesFirst := []Message{
		{
			Role:    "user",
			Content: userContentFirst,
		},
	}

	// Send the first request
	assistantResponseFirst, err := sendRequest(messagesFirst, *streamFlag, headers, url, model, delay)
	if err != nil {
		fmt.Println(err)
		return
	}

	if *nonInteractiveFlag {
		// -------------- Non-Interactive Mode: Perform Full Analysis --------------

		// Set the system prompt for the analysis
		systemPrompt := `You are an expert Kubernetes administrator and DevOps engineer. Your primary role is to analyze and troubleshoot Kubernetes pod logs, identify issues such as pod crashes, OOMKilled errors, and other deployment problems, and provide actionable solutions and best practices to resolve them.

When responding:
- Provide structured output using markdown tables, bullet points, or JSON where appropriate.
- Include step-by-step reasoning and detailed explanations for each troubleshooting step.
- Highlight key actions and recommendations.
- Ensure clarity and comprehensiveness to address complex Kubernetes issues effectively.`

		// Prepare the analysis messages
		analysisMessages := []Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: "Here are the key points from the log analysis:\n\n" + assistantResponseFirst,
			},
		}

		// Send the analysis request
		analysisResponse, err := sendRequest(analysisMessages, *streamFlag, headers, url, model, delay)
		if err != nil {
			fmt.Println(err)
			return
		}

		// Combine key points and analysis
		var outputBuilder strings.Builder
		outputBuilder.WriteString("# Key Points\n\n")
		outputBuilder.WriteString(assistantResponseFirst)
		outputBuilder.WriteString("\n\n# Analysis and Recommendations\n\n")
		outputBuilder.WriteString(analysisResponse)

		// Generate Loki query commands
		lokiQueries, err := generateLokiQueries(logString)
		if err != nil {
			fmt.Printf("Error generating Loki queries: %v\n", err)
			return
		}

		// Add Loki queries to the output
		outputBuilder.WriteString("\n\n# Loki Query Commands\n\n")
		for _, query := range lokiQueries {
			outputBuilder.WriteString(fmt.Sprintf("```\n%s\n```\n\n", query))
		}

		// Save to output file
		err = ioutil.WriteFile(*outputFile, []byte(outputBuilder.String()), 0644)
		if err != nil {
			fmt.Printf("Error writing to file %s: %v\n", *outputFile, err)
			return
		}

		fmt.Printf("\nAnalysis saved to %s\n", *outputFile)
	} else {
		// -------------- Interactive Mode --------------

		// Set the system prompt for the interactive session
		systemPrompt := `You are an expert Kubernetes administrator and DevOps engineer. Your primary role is to analyze and troubleshoot Kubernetes pod logs, identify issues such as pod crashes, OOMKilled errors, and other deployment problems, and provide actionable solutions and best practices to resolve them.

When responding:
- Provide structured output using markdown tables, bullet points, or JSON where appropriate.
- Include step-by-step reasoning and detailed explanations for each troubleshooting step.
- Highlight key actions and recommendations.
- Ensure clarity and comprehensiveness to address complex Kubernetes issues effectively.`

		// Initialize messages for interactive session
		messages := []Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: "Here are the key points from the log analysis:\n\n" + assistantResponseFirst,
			},
		}

		// Start interactive chat session
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("\nEnter your message (type 'exit' to quit):")
		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}
			userInput := scanner.Text()

			// Check for exit command
			if strings.ToLower(strings.TrimSpace(userInput)) == "exit" {
				fmt.Println("Exiting chat session.")
				break
			}

			// Append user's message to messages
			messages = append(messages, Message{
				Role:    "user",
				Content: userInput,
			})

			// Send request with updated messages
			assistantResponse, err := sendRequest(messages, *streamFlag, headers, url, model, delay)
			if err != nil {
				fmt.Println(err)
				break
			}

			// Append assistant's response to messages
			messages = append(messages, Message{
				Role:    "assistant",
				Content: assistantResponse,
			})
		}
	}
}
