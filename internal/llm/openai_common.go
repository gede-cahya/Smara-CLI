package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Shared OpenAI types (moved from openai.go)
type openAIChatRequest struct {
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	Tools           []openAITool    `json:"tools,omitempty"`
	Stream          bool            `json:"stream"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIToolCallFunc `json:"function"`
	Index    int                `json:"index"` // Required for streaming to track which tool call is being updated
}

type openAIToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

type ImageGenerationOptions struct {
	Model          string
	Prompt         string
	Size           string
	Quality        string
	N              int
	ResponseFormat string
}

type ImageEditOptions struct {
	Model          string
	Prompt         string
	Size           string
	Quality        string
	N              int
	ResponseFormat string
	MaskPath       string
}

type ImageGenerationResult struct {
	Data          []byte
	Model         string
	RevisedPrompt string
	MIME          string
	Extension     string
}

type openAIImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type openAIImageResponse struct {
	Created int64                  `json:"created"`
	Data    []openAIImageDataItem  `json:"data"`
	Output  []openAIResponseOutput `json:"output,omitempty"`
	Error   *openAIAPIError        `json:"error,omitempty"`
}

type openAIAPIError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

type openAIImageDataItem struct {
	B64JSON       string `json:"b64_json"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type openAIResponseOutput struct {
	Type    string                  `json:"type,omitempty"`
	Content []openAIResponseContent `json:"content,omitempty"`
}

type openAIResponseContent struct {
	Type          string `json:"type,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	ImageBase64   string `json:"image_base64,omitempty"`
	ImageURL      string `json:"image_url,omitempty"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// Streaming types
type openAIChatStreamResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content   string           `json:"content,omitempty"`
			Role      string           `json:"role,omitempty"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
			Reasoning string           `json:"reasoning_content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func normalizeReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return ""
	}
}

// convertMessagesToOpenAI converts internal messages to OpenAI format.
// Defensive deduplication for upstream 400 errors:
//  1) "Duplicate tool response for tool_call_id='...'" — same tool response sent twice
//  2) "'xxx_dup1' does not match any tool_calls[].id" — rewritten tool response ID
//     does not exist in any preceding assistant tool_calls.
//
// Root cause of (2) in the previous implementation: a single `seen` set was
// used for both assistant tool_calls and tool responses. Every valid tool
// response (which MUST reuse the assistant ID) was then treated as duplicate
// and renamed to _dupN, breaking the pairing and triggering (2).
// New implementation keeps the two namespaces separate and pairs via FIFO.
func convertMessagesToOpenAI(messages []Message) []openAIMessage {
	om := make([]openAIMessage, 0, len(messages))
	seenAssistant := make(map[string]bool)
	seenTool := make(map[string]bool)
	pending := make(map[string][]string) // origID -> queue of outIDs awaiting tool response
	dupCounter := 0

	for _, m := range messages {
		if m.Role == RoleTool && m.ToolCallID != "" {
			orig := m.ToolCallID
			var outID string
			if q, ok := pending[orig]; ok && len(q) > 0 {
				outID = q[0]
				pending[orig] = q[1:]
				if len(pending[orig]) == 0 {
					delete(pending, orig)
				}
			} else if seenAssistant[orig] && !seenTool[orig] {
				outID = orig
			} else {
				// No matching assistant tool_call — skip to avoid
				// 400 "'id' does not match any tool_calls[].id".
				continue
			}
			if seenTool[outID] {
				continue
			}
			seenTool[outID] = true
			om = append(om, openAIMessage{
				Role:             string(m.Role),
				Content:          m.Content,
				ToolCallID:       outID,
				ReasoningContent: m.ReasoningContent,
			})
			continue
		}

		msg := openAIMessage{
			Role:             string(m.Role),
			Content:          m.Content,
			ToolCallID:       m.ToolCallID,
			ReasoningContent: m.ReasoningContent,
		}

		for _, tc := range m.ToolCalls {
			origID := tc.ID
			outID := origID
			if outID != "" && seenAssistant[outID] {
				dupCounter++
				outID = fmt.Sprintf("%s_%d", origID, dupCounter)
			}
			if outID != "" {
				seenAssistant[outID] = true
				pending[origID] = append(pending[origID], outID)
			}
			args := tc.Args
			if args == nil || len(args) == 0 {
				args = map[string]interface{}{"_noop": true}
			}
			argsJSON, _ := json.Marshal(args)
			msg.ToolCalls = append(msg.ToolCalls, openAIToolCall{
				ID:   outID,
				Type: "function",
				Function: openAIToolCallFunc{
					Name:      tc.Function,
					Arguments: string(argsJSON),
				},
			})
		}
		om = append(om, msg)
	}
	return om
}

func streamOpenAIWithContext(ctx context.Context, client *http.Client, host, apiKey string, req openAIChatRequest, callback StreamCallback) (*ChatResponse, []ToolCall, error) {
	req.Stream = true
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", host+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	// Add context headers for OpenRouter (ignored by others)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/gede-cahya/Smara-CLI")
	httpReq.Header.Set("X-Title", "Smara CLI")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal menghubungi API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var fullContent strings.Builder
	var fullThinking strings.Builder
	var finalModel string
	var toolCallsMap = make(map[int]*ToolCall)
	var toolCallsRawArgs = make(map[int]*strings.Builder)
	var dsmlFilter DSMLStreamFilter
	var thinkFilter ThinkStreamFilter

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIChatStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if chunk.Model != "" {
			finalModel = chunk.Model
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			delta := choice.Delta

			if delta.Content != "" {
				// Split inline <think>...</think> reasoning out of the content
				// stream first so it is routed to the thinking channel instead
				// of leaking into the live answer. Providers that already use
				// the separate `reasoning` field below are unaffected (their
				// content carries no think tags).
				visible, inlineThink := thinkFilter.Write(delta.Content)
				if inlineThink != "" {
					fullThinking.WriteString(inlineThink)
					if callback != nil {
						callback(inlineThink, true, PhaseThinking)
					}
				}
				if visible != "" {
					fullContent.WriteString(visible)
					safeChunk := dsmlFilter.Write(visible)
					if safeChunk != "" && callback != nil {
						callback(safeChunk, false, PhaseGenerating)
					}
				}
			}

			if delta.Reasoning != "" {
				fullThinking.WriteString(delta.Reasoning)
				if callback != nil {
					callback(delta.Reasoning, true, PhaseThinking)
				}
			}

			for _, tc := range delta.ToolCalls {
				idx := tc.Index
				if _, ok := toolCallsMap[idx]; !ok {
					toolCallsMap[idx] = &ToolCall{
						ID:       tc.ID,
						Function: tc.Function.Name,
					}
					toolCallsRawArgs[idx] = &strings.Builder{}
				}
				if tc.ID != "" {
					toolCallsMap[idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCallsMap[idx].Function = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					toolCallsRawArgs[idx].WriteString(tc.Function.Arguments)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error saat membaca stream: %w", err)
	}

	// Flush any buffered tail from the think filter first, routing leftover
	// reasoning to the thinking channel and leftover content through DSML.
	tailContent, tailThink := thinkFilter.Close()
	if tailThink != "" {
		fullThinking.WriteString(tailThink)
		if callback != nil {
			callback(tailThink, true, PhaseThinking)
		}
	}
	if tailContent != "" {
		fullContent.WriteString(tailContent)
		safe := dsmlFilter.Write(tailContent)
		if safe != "" && callback != nil {
			callback(safe, false, PhaseGenerating)
		}
	}

	// Flush any remaining buffered text from DSML filter
	safeRemaining := dsmlFilter.Close()
	if safeRemaining != "" && callback != nil {
		callback(safeRemaining, false, PhaseGenerating)
	}

	// Parse accumulated tool call arguments
	var toolCalls []ToolCall
	for i := 0; i < len(toolCallsMap); i++ {
		if tc, ok := toolCallsMap[i]; ok {
			raw := toolCallsRawArgs[i].String()
			if raw != "" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(raw), &args); err == nil {
					tc.Args = args
				}
			}
			toolCalls = append(toolCalls, *tc)
		}
	}

	return &ChatResponse{
		Content:  fullContent.String(),
		Thinking: fullThinking.String(),
		Model:    finalModel,
	}, toolCalls, nil
}

func generateOpenAIImage(client *http.Client, host, apiKey, defaultModel string, opts ImageGenerationOptions) (*ImageGenerationResult, error) {
	return generateOpenAIImageWithContext(context.Background(), client, host, apiKey, defaultModel, opts)
}

func generateOpenAIImageWithContext(ctx context.Context, client *http.Client, host, apiKey, defaultModel string, opts ImageGenerationOptions) (*ImageGenerationResult, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt gambar kosong")
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = defaultModel
	}
	if model == "" {
		return nil, fmt.Errorf("model gambar kosong")
	}
	format := opts.ResponseFormat
	if format == "" {
		format = "b64_json"
	}

	reqBody := openAIImageRequest{
		Model:          model,
		Prompt:         prompt,
		N:              opts.N,
		Size:           opts.Size,
		Quality:        opts.Quality,
		ResponseFormat: format,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal image request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(host, "/")+"/images/generations", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi image provider: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca image response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeOpenAIImageError(resp.StatusCode, body)
	}

	return decodeOpenAIImageResponse(client, body, model)
}

func editOpenAIImage(client *http.Client, host, apiKey, defaultModel, imagePath string, opts ImageEditOptions) (*ImageGenerationResult, error) {
	return editOpenAIImageWithContext(context.Background(), client, host, apiKey, defaultModel, imagePath, opts)
}

func editOpenAIImageWithContext(ctx context.Context, client *http.Client, host, apiKey, defaultModel, imagePath string, opts ImageEditOptions) (*ImageGenerationResult, error) {
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt edit gambar kosong")
	}
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return nil, fmt.Errorf("image_path kosong")
	}
	imageFile, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("gagal membuka image_path: %w", err)
	}
	defer imageFile.Close()

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = defaultModel
	}
	if model == "" {
		return nil, fmt.Errorf("model edit gambar kosong")
	}
	format := opts.ResponseFormat
	if format == "" {
		format = "b64_json"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":           model,
		"prompt":          prompt,
		"response_format": format,
	}
	if opts.N > 0 {
		fields["n"] = strconv.Itoa(opts.N)
	}
	if strings.TrimSpace(opts.Size) != "" {
		fields["size"] = strings.TrimSpace(opts.Size)
	}
	if strings.TrimSpace(opts.Quality) != "" {
		fields["quality"] = strings.TrimSpace(opts.Quality)
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("gagal menulis field image edit %s: %w", k, err)
		}
	}
	imageMIME, err := detectImageMIME(imageFile, imagePath)
	if err != nil {
		return nil, err
	}
	filename := filepath.Base(imagePath)
	if filepath.Ext(filename) == "" {
		filename += extensionForImageMIME(imageMIME)
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, escapeMultipartFilename(filename)))
	partHeader.Set("Content-Type", imageMIME)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat form file image: %w", err)
	}
	if _, err := io.Copy(part, imageFile); err != nil {
		return nil, fmt.Errorf("gagal menulis file image ke request: %w", err)
	}
	if strings.TrimSpace(opts.MaskPath) != "" {
		maskFile, err := os.Open(strings.TrimSpace(opts.MaskPath))
		if err != nil {
			return nil, fmt.Errorf("gagal membuka mask_path: %w", err)
		}
		defer maskFile.Close()
		maskMIME, err := detectImageMIME(maskFile, opts.MaskPath)
		if err != nil {
			return nil, err
		}
		maskName := filepath.Base(opts.MaskPath)
		if filepath.Ext(maskName) == "" {
			maskName += extensionForImageMIME(maskMIME)
		}
		maskHeader := make(textproto.MIMEHeader)
		maskHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="mask"; filename="%s"`, escapeMultipartFilename(maskName)))
		maskHeader.Set("Content-Type", maskMIME)
		maskPart, err := writer.CreatePart(maskHeader)
		if err != nil {
			return nil, fmt.Errorf("gagal membuat form file mask: %w", err)
		}
		if _, err := io.Copy(maskPart, maskFile); err != nil {
			return nil, fmt.Errorf("gagal menulis file mask ke request: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gagal menutup multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(host, "/")+"/images/edits", &body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi image edit provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca image edit response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeOpenAIImageError(resp.StatusCode, respBody)
	}
	return decodeOpenAIImageResponse(client, respBody, model)
}

func detectImageMIME(file *os.File, imagePath string) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("gagal seek image file: %w", err)
	}
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("gagal membaca header image: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("gagal reset image file: %w", err)
	}

	contentType := http.DetectContentType(buf[:n])
	if !strings.HasPrefix(contentType, "image/") || contentType == "application/octet-stream" {
		if ext := strings.ToLower(filepath.Ext(imagePath)); ext != "" {
			if byExt := mime.TypeByExtension(ext); strings.HasPrefix(byExt, "image/") {
				contentType = strings.Split(byExt, ";")[0]
			}
		}
	}
	if contentType == "image/jpg" {
		contentType = "image/jpeg"
	}
	if !strings.HasPrefix(contentType, "image/") || contentType == "application/octet-stream" {
		return "", fmt.Errorf("file input tidak terdeteksi sebagai image valid (MIME: %s). Gunakan PNG/JPG/WEBP/GIF", contentType)
	}
	return contentType, nil
}

func extensionForImageMIME(mimeType string) string {
	switch strings.ToLower(strings.Split(mimeType, ";")[0]) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func escapeMultipartFilename(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}

func decodeOpenAIImageResponse(client *http.Client, body []byte, model string) (*ImageGenerationResult, error) {
	var imageResp openAIImageResponse
	if err := json.Unmarshal(body, &imageResp); err != nil {
		return nil, fmt.Errorf("gagal decode image response: %w", err)
	}

	if imageResp.Error != nil {
		return nil, formatOpenAIImageAPIError(0, imageResp.Error)
	}

	var b64, url, revised string
	if len(imageResp.Data) > 0 {
		item := imageResp.Data[0]
		b64 = item.B64JSON
		url = item.URL
		revised = item.RevisedPrompt
	}
	if b64 == "" && url == "" {
		for _, out := range imageResp.Output {
			for _, content := range out.Content {
				if b64 == "" {
					b64 = firstNonEmpty(content.B64JSON, content.ImageBase64)
				}
				if url == "" {
					url = firstNonEmpty(content.URL, content.ImageURL)
				}
				if revised == "" {
					revised = content.RevisedPrompt
				}
				if b64 != "" || url != "" {
					break
				}
			}
			if b64 != "" || url != "" {
				break
			}
		}
	}

	if strings.HasPrefix(b64, "data:") {
		if _, data, ok := strings.Cut(b64, ","); ok {
			b64 = data
		}
	}
	if strings.HasPrefix(url, "data:") {
		if _, data, ok := strings.Cut(url, ","); ok {
			b64 = data
			url = ""
		}
	}

	var decoded []byte
	var err error
	if b64 != "" {
		decoded, err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("gagal decode image base64: %w", err)
		}
	} else if url != "" {
		decoded, err = downloadGeneratedImage(client, url)
		if err != nil {
			return nil, err
		}
	} else {
		preview := strings.TrimSpace(string(body))
		if len(preview) > 600 {
			preview = preview[:600] + "..."
		}
		return nil, fmt.Errorf("image response kosong: tidak ditemukan data[].b64_json/url atau output[].content image. response: %s", preview)
	}

	mime := http.DetectContentType(decoded)
	return &ImageGenerationResult{
		Data:          decoded,
		Model:         model,
		RevisedPrompt: revised,
		MIME:          mime,
		Extension:     imageExtension(mime),
	}, nil
}

func decodeOpenAIImageError(status int, body []byte) error {
	var imageResp openAIImageResponse
	if err := json.Unmarshal(body, &imageResp); err == nil && imageResp.Error != nil {
		return formatOpenAIImageAPIError(status, imageResp.Error)
	}
	preview := strings.TrimSpace(string(body))
	if len(preview) > 600 {
		preview = preview[:600] + "..."
	}
	if status > 0 {
		return fmt.Errorf("image provider error (status %d): %s", status, preview)
	}
	return fmt.Errorf("image provider error: %s", preview)
}

func formatOpenAIImageAPIError(status int, apiErr *openAIAPIError) error {
	if apiErr == nil {
		return fmt.Errorf("image provider error")
	}
	message := strings.TrimSpace(apiErr.Message)
	if message == "" {
		message = "unknown provider error"
	}
	parts := []string{fmt.Sprintf("image provider error: %s", message)}
	meta := make([]string, 0, 3)
	if status > 0 {
		meta = append(meta, fmt.Sprintf("status=%d", status))
	}
	if strings.TrimSpace(apiErr.Type) != "" {
		meta = append(meta, "type="+strings.TrimSpace(apiErr.Type))
	}
	if strings.TrimSpace(apiErr.Code) != "" {
		meta = append(meta, "code="+strings.TrimSpace(apiErr.Code))
	}
	if len(meta) > 0 {
		parts = append(parts, "("+strings.Join(meta, ", ")+")")
	}
	return fmt.Errorf("%s", strings.Join(parts, " "))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func downloadGeneratedImage(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal mengunduh image url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("gagal mengunduh image url (status %d): %s", resp.StatusCode, string(body))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca image url: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image url mengembalikan data kosong")
	}
	return data, nil
}

func imageExtension(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
