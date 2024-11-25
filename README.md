# K8sLogbotGoGPT Documentation

## Overview

K8sLogbotGoGPT is a command-line tool designed for Kubernetes administrators and DevOps teams. It assists in analyzing Kubernetes pod logs by leveraging advanced language models to identify and troubleshoot common issues, such as pod crashes, OOMKilled errors, and deployment failures. K8sLogbotGoGPT provides actionable insights to enhance cluster stability and performance.

## Usage

### Setup
Before running K8sLogbotGoGPT, set the necessary API keys:

```bash
export OPENAI_API_KEY=<your_openai_key>
export APIKEY=<your_K8s_key>
```
### Command-Line Flags
- `-log="partial_filename"`: Specify a partial log filename to match (e.g., "01-LOG").
- `-stream`: Enable streaming output.
- `-delay=milliseconds`: Set delay in milliseconds between streaming chunks (default is 50ms).
- `-noninteractive`: Enable non-interactive mode for key point generation and full analysis.
- `-output="filename.md"`: Specify the output Markdown file name (default is output.md).

### Basic Commands

#### Run K8sLogbotGoGPT
Execute K8sLogbotGoGPT with or without additional options:

```bash
go run .
```

#### Log Analysis
Analyze a specific log file with streaming output:

```bash
go run . -log="01-LOG" -stream
```

Perform non-interactive analysis and save output to a file:

```bash
go run . -log="01-LOG" -noninteractive -output="analysis.md"
```

### View Specific Log
Open a specific log file for review:

```bash
cat "LOGS/01-LOG-HIGH - Crashing pod K8s-controller-manager-controller-manager-758756d966-5q8pz in namespace kube-system.log"
```

### Step-by-Step Analysis
Use K8sLogbotGoGPT’s step-by-step log analysis for detailed investigation:

```bash
go run . -log="01-LOG" -stream
```

### Export Analysis
Run analysis in a non-interactive mode and save the results in Markdown format:

```bash
go run . -log="01-LOG" -noninteractive -output="analysis.md"
```

## Example Workflow
Here’s a basic workflow using K8sLogbotGoGPT for Kubernetes log analysis:

1. **Initialize**: Ensure your API keys are set.
2. **Start K8sLogbotGoGPT**: Run `go run .` to start the analysis.
3. **Provide Logs**: Use options like `-log`, `-stream`, and `-noninteractive` to configure how logs are processed.
4. **Review Output**: Check the `analysis.md` file or the terminal output for insights.

K8sLogbotGoGPT's straightforward commands and powerful analysis make it a valuable tool for Kubernetes log analysis and troubleshooting.

## Notes
- Ensure that you have Go installed and properly configured on your system to run K8sLogbotGoGPT.
- Replace `<your_openai_key>` and `<your_K8s_key>` with your actual API keys.
- For non-interactive analysis, the output will be saved in the specified Markdown file, which can be reviewed later.


## Description of the Go Program

This Go program is designed to interact with an API for generating key points and analyzing Kubernetes pod logs. It utilizes various packages to handle HTTP requests, JSON parsing, and Markdown rendering. Below is a breakdown of its main components and functionalities:

### Key Components

- **Message Struct**: Represents each message in the conversation with a role (user or assistant) and content.
  
- **RequestBody Struct**: Defines the structure of the API request body, including the model, messages, and streaming option.

- **ChatCompletionResponse Struct**: Represents the structure of the API response, including details like ID, model used, choices made by the assistant, and usage statistics.

- **Functions**:
  - `handleNonStreamResponse`: Processes non-streaming responses from the API.
  - `handleStreamResponse`: Manages streaming responses with a delay for better readability.
  - `sendRequest`: Sends requests to the API and handles both streaming and non-streaming scenarios.
  - `generateLokiQueries`: Generates Loki query commands based on log content.

### Main Functionality

1. **API Key Retrieval**: The program retrieves necessary API keys from environment variables to authenticate requests.

2. **Command-Line Flags**: It accepts flags for log filename patterns, streaming options, delays between chunks, and output file specifications.

3. **Log File Processing**: The program searches for log files matching a specified pattern in a designated directory. It reads the contents of the first matching file.

4. **Key Points Generation**: It sends a request to generate key points from the log content using a structured prompt.

5. **Non-Interactive vs Interactive Mode**:
   - In non-interactive mode, it performs full analysis and saves results to a Markdown file.
   - In interactive mode, it allows users to engage in a chat session for further analysis based on generated key points.

6. **Output Formatting**: The program formats responses using Markdown for clarity and readability.

### Conclusion

This Go program effectively integrates API interactions with Kubernetes log analysis, providing users with tools to extract insights and generate actionable queries while maintaining a user-friendly interface through command-line options.