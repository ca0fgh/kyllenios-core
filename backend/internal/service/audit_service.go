package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ca0fgh/hermes-proxy/internal/config"
	infraerrors "github.com/ca0fgh/hermes-proxy/internal/pkg/errors"
	"github.com/ca0fgh/hermes-proxy/internal/pkg/logger"
	"github.com/ca0fgh/hermes-proxy/internal/util/logredact"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type AuditService struct {
	repo AuditRepository
	cfg  *config.Config
}

func NewAuditService(repo AuditRepository, cfg *config.Config) *AuditService {
	return &AuditService{repo: repo, cfg: cfg}
}

func (s *AuditService) Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.Audit.Enabled && s.repo != nil
}

func (s *AuditService) RequireEnabled() error {
	if s == nil || s.cfg == nil || !s.cfg.Gateway.Audit.Enabled {
		return infraerrors.NotFound("AUDIT_DISABLED", "Audit is disabled")
	}
	if s.repo == nil {
		return infraerrors.ServiceUnavailable("AUDIT_REPO_UNAVAILABLE", "Audit repository not available")
	}
	return nil
}

func (s *AuditService) ListAuditEvents(ctx context.Context, filter *AuditEventFilter) (*AuditEventList, error) {
	if err := s.RequireEnabled(); err != nil {
		return nil, err
	}
	return s.repo.ListAuditEvents(ctx, filter)
}

func (s *AuditService) GetAuditEventByID(ctx context.Context, id int64) (*AuditEvent, error) {
	if err := s.RequireEnabled(); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, infraerrors.BadRequest("AUDIT_INVALID_ID", "invalid audit event id")
	}
	event, err := s.repo.GetAuditEventByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, infraerrors.NotFound("AUDIT_EVENT_NOT_FOUND", "Audit event not found")
	}
	return event, nil
}

func (s *AuditService) RecordGatewayAudit(ctx context.Context, input *GatewayAuditCaptureInput) error {
	if !s.Enabled() || input == nil {
		return nil
	}

	event, err := s.BuildGatewayAuditEvent(ctx, input)
	if err != nil {
		return err
	}
	if _, err := s.repo.InsertAuditEvent(ctx, event); err != nil {
		return err
	}

	if event.RiskLevel == "high" {
		logger.FromContext(ctx).With(
			zap.String("component", "audit.gateway"),
			zap.String("request_id", event.RequestID),
			zap.String("platform", event.Platform),
			zap.String("path", event.Path),
			zap.Strings("risk_flags", event.RiskFlags),
			zap.Int("tool_count", event.ToolCount),
		).Warn("audit.gateway.high_risk_event")
		_ = s.sendAlertWebhook(ctx, event)
	}

	return nil
}

func (s *AuditService) BuildGatewayAuditEvent(_ context.Context, input *GatewayAuditCaptureInput) (*AuditEvent, error) {
	if input == nil {
		return nil, nil
	}

	toolCalls := extractAuditToolCalls(input.ResponseBody)
	upstreamEndpoint := strings.TrimSpace(input.UpstreamEndpoint)
	if upstreamEndpoint == "" {
		upstreamEndpoint = strings.TrimSpace(input.InboundEndpoint)
	}
	toolHashes := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		toolHashes = append(toolHashes, hashJSON(map[string]any{
			"name":      tc.Name,
			"arguments": tc.Arguments,
		}))
	}

	riskFlags := s.detectRiskFlags(toolCalls)
	riskLevel := "low"
	switch {
	case containsString(riskFlags, "high_risk_tool") || containsString(riskFlags, "suspicious_shell_pattern"):
		riskLevel = "high"
	case len(toolCalls) > 0:
		riskLevel = "medium"
	}

	return &AuditEvent{
		CreatedAt:         time.Now().UTC(),
		RequestID:         strings.TrimSpace(input.RequestID),
		ClientRequestID:   strings.TrimSpace(input.ClientRequestID),
		UserID:            input.UserID,
		APIKeyID:          input.APIKeyID,
		AccountID:         input.AccountID,
		GroupID:           input.GroupID,
		Platform:          strings.TrimSpace(input.Platform),
		RequestType:       input.RequestType.Normalize(),
		Method:            strings.TrimSpace(input.Method),
		Path:              strings.TrimSpace(input.Path),
		InboundEndpoint:   strings.TrimSpace(input.InboundEndpoint),
		UpstreamEndpoint:  upstreamEndpoint,
		UpstreamTarget:    strings.TrimSpace(input.UpstreamTarget),
		StatusCode:        input.StatusCode,
		RequestedModel:    strings.TrimSpace(input.RequestedModel),
		EffectiveModel:    strings.TrimSpace(input.EffectiveModel),
		UpstreamModel:     strings.TrimSpace(input.UpstreamModel),
		UserAgent:         strings.TrimSpace(input.UserAgent),
		RequestHash:       strings.TrimSpace(input.RequestHash),
		ResponseHash:      strings.TrimSpace(input.ResponseHash),
		RequestBytes:      input.RequestBytes,
		ResponseBytes:     input.ResponseBytes,
		RequestTruncated:  input.RequestTruncated,
		ResponseTruncated: input.ResponseTruncated,
		HasToolCalls:      len(toolCalls) > 0,
		ToolCount:         len(toolCalls),
		ToolCalls:         toolCalls,
		ToolHashes:        toolHashes,
		RiskFlags:         riskFlags,
		RiskLevel:         riskLevel,
		CanaryInjected:    input.CanaryInjected,
		CanaryLabels:      cloneStringSlice(input.CanaryLabels),
	}, nil
}

func (s *AuditService) detectRiskFlags(toolCalls []AuditToolCall) []string {
	var flags []string
	if len(toolCalls) > 0 {
		flags = append(flags, "tool_call_present")
	}

	patterns := []string{"curl", "wget", "pip install", "npm install", "cargo add"}
	if s != nil && s.cfg != nil && len(s.cfg.Gateway.Audit.AlertShellPatterns) > 0 {
		patterns = append(patterns, s.cfg.Gateway.Audit.AlertShellPatterns...)
	}

	for _, tc := range toolCalls {
		switch strings.ToLower(strings.TrimSpace(tc.Name)) {
		case "bash", "run_command":
			flags = appendIfMissing(flags, "high_risk_tool")
		}
		for _, fragment := range collectArgumentStrings(tc.Arguments) {
			lower := strings.ToLower(fragment)
			if strings.Contains(lower, "| sh") || strings.Contains(lower, "| bash") {
				flags = appendIfMissing(flags, "suspicious_shell_pattern")
				break
			}
			for _, p := range patterns {
				if p != "" && strings.Contains(lower, strings.ToLower(p)) {
					flags = appendIfMissing(flags, "suspicious_shell_pattern")
					break
				}
			}
		}
	}

	sort.Strings(flags)
	return flags
}

func (s *AuditService) sendAlertWebhook(ctx context.Context, event *AuditEvent) error {
	if s == nil || s.cfg == nil {
		return nil
	}
	url := strings.TrimSpace(s.cfg.Gateway.Audit.AlertWebhookURL)
	if url == "" || event == nil {
		return nil
	}

	timeout := 5 * time.Second
	if s.cfg.Gateway.Audit.AlertWebhookTimeoutSeconds > 0 {
		timeout = time.Duration(s.cfg.Gateway.Audit.AlertWebhookTimeoutSeconds) * time.Second
	}

	payload, _ := json.Marshal(map[string]any{
		"request_id":    event.RequestID,
		"platform":      event.Platform,
		"path":          event.Path,
		"risk_level":    event.RiskLevel,
		"risk_flags":    event.RiskFlags,
		"tool_count":    event.ToolCount,
		"status_code":   event.StatusCode,
		"created_at":    event.CreatedAt.Format(time.RFC3339Nano),
		"response_hash": event.ResponseHash,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return infraerrors.BadRequest("AUDIT_WEBHOOK_FAILED", "Audit webhook returned non-2xx")
	}
	return nil
}

func extractAuditToolCalls(raw []byte) []AuditToolCall {
	if len(raw) == 0 {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	if json.Valid(trimmed) {
		return extractAuditToolCallsFromJSON(trimmed)
	}
	return extractAuditToolCallsFromSSE(trimmed)
}

func extractAuditToolCallsFromJSON(raw []byte) []AuditToolCall {
	out := make([]AuditToolCall, 0)
	appendTool := func(name string, args any) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		out = append(out, AuditToolCall{Name: name, Arguments: redactAuditArguments(args)})
	}

	for _, item := range gjson.GetBytes(raw, "output").Array() {
		itemType := item.Get("type").String()
		if itemType == "function_call" || itemType == "tool_call" {
			appendTool(item.Get("name").String(), parseArgumentsValue(item.Get("arguments").Value()))
		}
	}

	for _, choice := range gjson.GetBytes(raw, "choices").Array() {
		for _, tc := range choice.Get("message.tool_calls").Array() {
			appendTool(tc.Get("function.name").String(), parseArgumentsValue(tc.Get("function.arguments").Value()))
		}
	}

	for _, block := range gjson.GetBytes(raw, "content").Array() {
		if block.Get("type").String() == "tool_use" {
			appendTool(block.Get("name").String(), parseArgumentsValue(block.Get("input").Value()))
		}
	}

	for _, candidate := range gjson.GetBytes(raw, "candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			if fn := part.Get("functionCall"); fn.Exists() {
				appendTool(fn.Get("name").String(), parseArgumentsValue(fn.Get("args").Value()))
			}
		}
	}

	return dedupeAuditToolCalls(out)
}

func extractAuditToolCallsFromSSE(raw []byte) []AuditToolCall {
	type openAIChatTool struct {
		Name string
		Args strings.Builder
	}

	openAIChat := map[int]*openAIChatTool{}
	openAIResponses := map[string]*AuditToolCall{}
	anthropicTools := map[int]*AuditToolCall{}
	anthropicInputBuffers := map[int]*strings.Builder{}
	var out []AuditToolCall

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	currentEvent := ""

	flushAnthropicBuilder := func(index int) {
		builder, ok := anthropicInputBuffers[index]
		if !ok {
			return
		}
		tool, ok := anthropicTools[index]
		if !ok || builder.Len() == 0 {
			return
		}
		tool.Arguments = redactAuditArguments(parseArgumentsValue(builder.String()))
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" || !gjson.Valid(payload) {
				continue
			}

			for _, tc := range extractAuditToolCallsFromJSON([]byte(payload)) {
				out = append(out, tc)
			}

			for _, choice := range gjson.Get(payload, "choices").Array() {
				for _, tc := range choice.Get("delta.tool_calls").Array() {
					idx := int(tc.Get("index").Int())
					entry := openAIChat[idx]
					if entry == nil {
						entry = &openAIChatTool{}
						openAIChat[idx] = entry
					}
					if name := tc.Get("function.name").String(); name != "" {
						entry.Name = name
					}
					if args := tc.Get("function.arguments").String(); args != "" {
						entry.Args.WriteString(args)
					}
				}
			}

			switch gjson.Get(payload, "type").String() {
			case "response.output_item.added", "response.output_item.done":
				item := gjson.Get(payload, "item")
				if item.Get("type").String() == "function_call" || item.Get("type").String() == "tool_call" {
					id := item.Get("id").String()
					openAIResponses[id] = &AuditToolCall{
						Name:      item.Get("name").String(),
						Arguments: redactAuditArguments(parseArgumentsValue(item.Get("arguments").Value())),
					}
				}
			case "response.function_call_arguments.delta":
				itemID := gjson.Get(payload, "item_id").String()
				if itemID == "" {
					break
				}
				entry := openAIResponses[itemID]
				if entry == nil {
					entry = &AuditToolCall{}
					openAIResponses[itemID] = entry
				}
				delta := gjson.Get(payload, "delta").String()
				prev, _ := entry.Arguments.(string)
				entry.Arguments = prev + delta
			}

			if currentEvent == "content_block_start" {
				block := gjson.Get(payload, "content_block")
				if block.Get("type").String() == "tool_use" {
					idx := int(gjson.Get(payload, "index").Int())
					anthropicTools[idx] = &AuditToolCall{
						Name:      block.Get("name").String(),
						Arguments: redactAuditArguments(parseArgumentsValue(block.Get("input").Value())),
					}
				}
			}
			if currentEvent == "content_block_delta" {
				deltaType := gjson.Get(payload, "delta.type").String()
				if deltaType == "input_json_delta" {
					idx := int(gjson.Get(payload, "index").Int())
					builder := anthropicInputBuffers[idx]
					if builder == nil {
						builder = &strings.Builder{}
						anthropicInputBuffers[idx] = builder
					}
					builder.WriteString(gjson.Get(payload, "delta.partial_json").String())
					flushAnthropicBuilder(idx)
				}
			}
		}
	}

	for _, entry := range openAIChat {
		if entry == nil || strings.TrimSpace(entry.Name) == "" {
			continue
		}
		out = append(out, AuditToolCall{
			Name:      entry.Name,
			Arguments: redactAuditArguments(parseArgumentsValue(entry.Args.String())),
		})
	}
	for _, entry := range openAIResponses {
		if entry == nil || strings.TrimSpace(entry.Name) == "" {
			continue
		}
		entry.Arguments = redactAuditArguments(parseArgumentsValue(entry.Arguments))
		out = append(out, *entry)
	}
	for idx, entry := range anthropicTools {
		flushAnthropicBuilder(idx)
		if entry == nil || strings.TrimSpace(entry.Name) == "" {
			continue
		}
		out = append(out, *entry)
	}

	return dedupeAuditToolCalls(out)
}

func parseArgumentsValue(v any) any {
	switch raw := v.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return ""
		}
		var decoded any
		if json.Valid([]byte(trimmed)) && json.Unmarshal([]byte(trimmed), &decoded) == nil {
			return decoded
		}
		return trimmed
	default:
		return raw
	}
}

func redactAuditArguments(v any) any {
	switch value := v.(type) {
	case map[string]any:
		return logredact.RedactMap(value)
	case []any:
		copied := make([]any, len(value))
		for i, item := range value {
			copied[i] = redactAuditArguments(item)
		}
		return copied
	case string:
		return logredact.RedactText(value)
	default:
		return value
	}
}

func dedupeAuditToolCalls(items []AuditToolCall) []AuditToolCall {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]AuditToolCall, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		key := hashJSON(map[string]any{
			"name":      item.Name,
			"arguments": item.Arguments,
		})
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func collectArgumentStrings(v any) []string {
	switch value := v.(type) {
	case string:
		return []string{value}
	case map[string]any:
		out := make([]string, 0, len(value))
		for _, child := range value {
			out = append(out, collectArgumentStrings(child)...)
		}
		return out
	case []any:
		out := make([]string, 0, len(value))
		for _, child := range value {
			out = append(out, collectArgumentStrings(child)...)
		}
		return out
	default:
		return nil
	}
}

func hashJSON(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendIfMissing(values []string, value string) []string {
	if value == "" || containsString(values, value) {
		return values
	}
	return append(values, value)
}
