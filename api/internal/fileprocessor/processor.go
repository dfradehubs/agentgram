package fileprocessor

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/dfradehubs/agentgram-api/internal/llm"
	"github.com/dfradehubs/agentgram-api/internal/models"
	"go.uber.org/zap"
)

// Processor converts file attachments to text descriptions using an LLM
type Processor struct {
	provider llm.Provider
	model    *models.LLMModel // kept for provider-specific image processing
	logger   *zap.Logger
}

// New creates a new file Processor. Returns nil if model is missing.
func New(model *models.LLMModel, logger *zap.Logger) *Processor {
	if model == nil || model.APIKey == "" {
		return nil
	}
	provider, err := llm.NewProvider(model)
	if err != nil {
		logger.Warn("failed to create LLM provider for fileprocessor", zap.Error(err))
		return nil
	}
	return &Processor{
		provider: provider,
		model:    model,
		logger:   logger,
	}
}

// NewWithProvider creates a Processor with a pre-configured provider (e.g. traced).
// The model is still needed for provider-specific image block formatting.
func NewWithProvider(provider llm.Provider, model *models.LLMModel, logger *zap.Logger) *Processor {
	if provider == nil || model == nil {
		return nil
	}
	return &Processor{
		provider: provider,
		model:    model,
		logger:   logger,
	}
}

// extractedFile holds the filename and extracted text content from an attachment.
type extractedFile struct {
	filename string
	content  string
}

// ProcessAttachments extracts content from all attachments (including images via vision API),
// then uses an LLM to reformulate the user's question integrating the file context naturally.
func (p *Processor) ProcessAttachments(ctx context.Context, msg *models.ChatMessage) error {
	if len(msg.Attachments) == 0 {
		return nil
	}

	var extracted []extractedFile

	for _, att := range msg.Attachments {
		content, err := p.processFile(ctx, att)
		if err != nil {
			p.logger.Warn("failed to process attachment",
				zap.String("filename", att.Filename),
				zap.Error(err))
			content = fmt.Sprintf("[Error procesando %s: %v]", att.Filename, err)
		}
		extracted = append(extracted, extractedFile{filename: att.Filename, content: content})
	}

	// Reformulate the user's question with LLM, integrating file context
	reformulated, err := p.reformulate(ctx, extracted, msg.Content)
	if err != nil {
		p.logger.Warn("reformulation failed, using prepend fallback", zap.Error(err))
		prefix := buildPrependFallback(extracted)
		if msg.Content != "" {
			msg.Content = prefix + "\n\n" + msg.Content
		} else {
			msg.Content = prefix
		}
	} else {
		msg.Content = reformulated
	}

	msg.Attachments = nil
	return nil
}

const reformulationSystemPrompt = `You are an assistant that reformulates user questions by incorporating context from attached files.
The user has attached files and asked a question. Your task is to create a single question/message that naturally integrates the relevant information from the files with the original question.
- Keep the user's original language
- Include the specific relevant details from the files
- If the user did not write any text, formulate a question based on the file contents
- Do not mention that there were "attached files" - integrate the information directly
- Be concise but complete`

// reformulate calls the LLM to rewrite the user's question incorporating file context.
func (p *Processor) reformulate(ctx context.Context, files []extractedFile, userQuestion string) (string, error) {
	var parts []string
	for _, f := range files {
		parts = append(parts, fmt.Sprintf("--- %s ---\n%s", f.filename, f.content))
	}
	filesContext := strings.Join(parts, "\n\n")

	userMsg := filesContext
	if userQuestion != "" {
		userMsg += "\n\n--- User question ---\n" + userQuestion
	}

	resp, err := p.provider.GenerateContent(ctx, &llm.Request{
		Messages:     []llm.Message{{Role: "user", Content: userMsg}},
		SystemPrompt: reformulationSystemPrompt,
		MaxTokens:    2048,
	})
	if err != nil {
		return "", err
	}

	return resp.Text, nil
}

// buildPrependFallback creates a simple text prefix from extracted files (used when reformulation fails).
func buildPrependFallback(files []extractedFile) string {
	var descriptions []string
	for _, f := range files {
		descriptions = append(descriptions, fmt.Sprintf("[File: %s]\n%s", f.filename, f.content))
	}
	return strings.Join(descriptions, "\n\n")
}

func (p *Processor) processFile(ctx context.Context, att models.Attachment) (string, error) {
	ct := strings.ToLower(att.ContentType)

	switch {
	case strings.HasPrefix(ct, "image/"):
		return p.processImage(ctx, att)
	case ct == "application/pdf":
		return p.processTextContent(att, "PDF content")
	case strings.HasPrefix(ct, "text/"), ct == "application/json", ct == "text/csv":
		return p.processTextContent(att, "File content")
	default:
		return fmt.Sprintf("[Unsupported file type: %s]", att.ContentType), nil
	}
}

const imageDescriptionPrompt = "Describe this image in detail, including all visible text, graphical elements and relevant context. Respond in the same language as any text visible in the image, or in English if there is no text."

func (p *Processor) processImage(ctx context.Context, att models.Attachment) (string, error) {
	mediaType := att.ContentType
	if mediaType == "" {
		mediaType = "image/png"
	}

	// Build provider-specific image content blocks
	var content interface{}
	switch p.model.Provider {
	case "anthropic":
		content = []map[string]interface{}{
			{
				"type": "image",
				"source": map[string]string{
					"type":       "base64",
					"media_type": mediaType,
					"data":       att.Data,
				},
			},
			{"type": "text", "text": imageDescriptionPrompt},
		}
	case "openai":
		dataURL := fmt.Sprintf("data:%s;base64,%s", att.ContentType, att.Data)
		content = []map[string]interface{}{
			{"type": "text", "text": imageDescriptionPrompt},
			{
				"type":      "image_url",
				"image_url": map[string]string{"url": dataURL},
			},
		}
	case "google":
		content = []interface{}{
			map[string]interface{}{
				"inlineData": map[string]string{
					"mimeType": mediaType,
					"data":     att.Data,
				},
			},
			map[string]interface{}{"text": imageDescriptionPrompt},
		}
	default:
		return "", fmt.Errorf("unsupported provider for image processing: %s", p.model.Provider)
	}

	resp, err := p.provider.GenerateContent(ctx, &llm.Request{
		Messages:  []llm.Message{{Role: "user", Content: content}},
		MaxTokens: 1024,
	})
	if err != nil {
		return "", err
	}

	return resp.Text, nil
}

func (p *Processor) processTextContent(att models.Attachment, label string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(att.Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	content := string(data)
	if len(content) > 50000 {
		content = content[:50000] + "\n... [truncado]"
	}

	return fmt.Sprintf("%s:\n%s", label, content), nil
}
