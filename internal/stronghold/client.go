package stronghold

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	citadelConfig "github.com/TryMightyAI/citadel/pkg/config"
	"github.com/TryMightyAI/citadel/pkg/ml"
	"stronghold/internal/config"
)

// Decision represents the scan decision
type Decision string

const (
	DecisionAllow Decision = "ALLOW"
	DecisionWarn  Decision = "WARN"
	DecisionBlock Decision = "BLOCK"
)

// ScanResult represents the result of a security scan
type ScanResult struct {
	Decision          Decision               `json:"decision"`
	Scores            map[string]float64     `json:"scores"`
	Reason            string                 `json:"reason"`
	LatencyMs         int64                  `json:"latency_ms"`
	RequestID         string                 `json:"request_id"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	SanitizedText     string                 `json:"sanitized_text,omitempty"`     // Clean version with threats removed
	ThreatsFound      []Threat               `json:"threats_found,omitempty"`      // Detailed threat info
	RecommendedAction string                 `json:"recommended_action,omitempty"` // What the agent should do
}

// Threat represents a detected threat with location info
type Threat struct {
	Category    string `json:"category"`    // Broad category: "prompt_injection", "credential_leak"
	Pattern     string `json:"pattern"`     // What matched (the specific pattern)
	Location    string `json:"location"`    // Where in text (line/offset if available)
	Severity    string `json:"severity"`    // "high", "medium", "low"
	Description string `json:"description"` // Human-readable explanation
}

// Scanner wraps the Citadel security scanner
type Scanner struct {
	config          *config.StrongholdConfig
	threatScorer    *ml.ThreatScorer
	hybridDetector  *ml.HybridDetector
	outputScanner   *ml.OutputScanner
	semanticEnabled bool
	hugotEnabled    bool
	llmEnabled      bool
}

// NewScanner creates a new Scanner with Citadel integration
func NewScanner(cfg *config.StrongholdConfig) (*Scanner, error) {
	// Create Citadel config
	citadelCfg := &citadelConfig.Config{
		BlockThreshold: cfg.BlockThreshold,
		WarnThreshold:  cfg.WarnThreshold,
	}

	// Create threat scorer (always enabled - base layer)
	threatScorer := ml.NewThreatScorer(citadelCfg)

	s := &Scanner{
		config:          cfg,
		threatScorer:    threatScorer,
		semanticEnabled: cfg.EnableSemantics,
		hugotEnabled:    cfg.EnableHugot,
		llmEnabled:      cfg.LLMProvider != "",
	}

	// Initialize hybrid detector if semantic or LLM detection is enabled
	if cfg.EnableSemantics || cfg.LLMProvider != "" {
		ollamaURL := os.Getenv("OLLAMA_URL") // Optional: for local embeddings
		openRouterKey := cfg.LLMAPIKey
		openRouterModel := cfg.LLMProvider

		hybridDetector, err := ml.NewHybridDetector(ollamaURL, openRouterKey, openRouterModel)
		if err != nil {
			// Log but don't fail - fallback to threat scorer only
			slog.Warn("failed to initialize hybrid detector", "error", err)
		} else {
			s.hybridDetector = hybridDetector
		}
	}

	// Initialize output scanner for credential detection
	s.outputScanner = ml.NewOutputScanner()

	return s, nil
}

// ScanContent scans external content for prompt injection attacks using Citadel
func (s *Scanner) ScanContent(ctx context.Context, text, sourceURL, sourceType, contentType string) (*ScanResult, error) {
	start := time.Now()

	var result *ScanResult
	var err error

	// Use hybrid detector if available (semantic + LLM detection)
	if s.hybridDetector != nil {
		result, err = s.scanWithHybrid(ctx, text, sourceURL, sourceType, contentType)
	} else {
		// Fallback to threat scorer only (heuristics)
		result, err = s.scanWithThreatScorer(text, sourceURL, sourceType, contentType)
	}

	if err != nil {
		return nil, err
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result, nil
}

// scanWithHybrid uses the full Citadel hybrid detector (heuristic + semantic + LLM)
func (s *Scanner) scanWithHybrid(ctx context.Context, text, sourceURL, sourceType, contentType string) (*ScanResult, error) {
	hybridResult, err := s.hybridDetector.Detect(ctx, text)
	if err != nil {
		// Fallback to threat scorer on error
		return s.scanWithThreatScorer(text, sourceURL, sourceType, contentType)
	}

	decision := DecisionAllow
	reason := "No threats detected"
	recommendedAction := "Content is safe to process"

	if hybridResult.Action == "BLOCK" {
		decision = DecisionBlock
		reason = fmt.Sprintf("Critical: %s (Score: %.2f)", hybridResult.RiskLevel, hybridResult.CombinedScore)
		recommendedAction = "DO NOT PROCEED - Content contains active threats. Discard immediately."
	} else if hybridResult.Action == "WARN" {
		decision = DecisionWarn
		reason = fmt.Sprintf("Warning: %s (Score: %.2f)", hybridResult.RiskLevel, hybridResult.CombinedScore)
		recommendedAction = "Caution advised - Review content manually before processing."
	}

	// Convert Citadel threats to our format
	threats := convertHybridThreats(hybridResult)

	return &ScanResult{
		Decision:          decision,
		Scores:            map[string]float64{
			"combined":      hybridResult.CombinedScore,
			"heuristic":     hybridResult.HeuristicScore,
			"semantic":      float64(hybridResult.SemanticScore),
			"ml_confidence": getMLConfidence(hybridResult),
		},
		Reason:            reason,
		SanitizedText:     s.sanitizeText(text, threats),
		ThreatsFound:      threats,
		RecommendedAction: recommendedAction,
		Metadata: map[string]interface{}{
			"source_url":      sourceURL,
			"source_type":     sourceType,
			"content_type":    contentType,
			"risk_level":      hybridResult.RiskLevel,
			"semantic_match":  hybridResult.SemanticMatch,
			"profile_used":    hybridResult.ProfileUsed,
		},
	}, nil
}

// scanWithThreatScorer uses only the heuristic threat scorer
func (s *Scanner) scanWithThreatScorer(text, sourceURL, sourceType, contentType string) (*ScanResult, error) {
	score := s.threatScorer.Evaluate(text)

	decision := DecisionAllow
	reason := "No threats detected"
	recommendedAction := "Content is safe to process"
	var threats []Threat

	if score >= s.config.BlockThreshold {
		decision = DecisionBlock
		reason = "Critical: Prompt injection attack detected in external content"
		recommendedAction = "DO NOT PROCEED - Content contains active prompt injection attack. Discard content immediately and do not pass to LLM under any circumstances."
		threats = extractThreatsFromScore(text, score)
	} else if score >= s.config.WarnThreshold {
		decision = DecisionWarn
		reason = "Suspicious patterns detected in external content"
		recommendedAction = "Caution advised - Review content manually before processing. Consider using sanitized version."
		threats = extractThreatsFromScore(text, score)
	}

	return &ScanResult{
		Decision:          decision,
		Scores: map[string]float64{
			"heuristic":     score,
			"ml_confidence": 0.0,
			"semantic":      0.0,
		},
		Reason:            reason,
		SanitizedText:     text,
		ThreatsFound:      threats,
		RecommendedAction: recommendedAction,
		Metadata: map[string]interface{}{
			"source_url":   sourceURL,
			"source_type":  sourceType,
			"content_type": contentType,
			"detection":    "heuristic_only",
		},
	}, nil
}

// ScanOutput scans LLM output for credential leaks using Citadel
func (s *Scanner) ScanOutput(ctx context.Context, text string) (*ScanResult, error) {
	start := time.Now()

	// Use Citadel's output scanner for credential detection
	result := s.outputScanner.ScanOutput(text)
	score := float64(result.RiskScore) / 100.0 // Convert 0-100 to 0.0-1.0

	decision := DecisionAllow
	reason := "No credentials detected"

	if score >= s.config.BlockThreshold {
		decision = DecisionBlock
		reason = "Possible credential leak detected"
	} else if score >= s.config.WarnThreshold {
		decision = DecisionWarn
		reason = "Potential sensitive data detected"
	}

	if !result.IsSafe {
		reason = strings.Join(result.Findings, "; ")
	}

	// Convert findings to threats
	threats := convertOutputFindings(result.Details)

	return &ScanResult{
		Decision: decision,
		Scores: map[string]float64{
			"credential_score": score,
			"findings_count":   float64(len(result.Details)),
		},
		Reason:    reason,
		LatencyMs: time.Since(start).Milliseconds(),
		ThreatsFound: threats,
		Metadata: map[string]interface{}{
			"findings":     len(result.Details),
			"risk_level":   result.RiskLevel,
			"is_safe":      result.IsSafe,
			"categories":   result.ThreatCategories,
		},
	}, nil
}

// sanitizeText sanitizes text based on detected threats
func (s *Scanner) sanitizeText(text string, threats []Threat) string {
	if len(threats) == 0 {
		return text
	}

	// Use Citadel's threat scorer for secret redaction if available
	if s.threatScorer != nil {
		redacted, wasRedacted := s.threatScorer.RedactSecrets(text)
		if wasRedacted {
			return redacted
		}
	}

	// Fallback: Basic redaction based on threat patterns found
	sanitized := text
	for _, threat := range threats {
		if threat.Category == "credential_leak" || threat.Category == "obfuscation" {
			// Redact the matched pattern
			sanitized = strings.ReplaceAll(sanitized, threat.Pattern, "[REDACTED]")
		}
	}

	return sanitized
}

// Close cleans up scanner resources
func (s *Scanner) Close() error {
	if s.hybridDetector != nil {
		// Hybrid detector doesn't have a Close method, but we could add cleanup here
	}
	return nil
}

// Helper functions

func convertHybridThreats(result *ml.HybridResult) []Threat {
	var threats []Threat

	// Convert all signals from the hybrid detector to threats
	// This uses Citadel's actual detection signals instead of placeholder logic
	for _, signal := range result.Signals {
		// Only include signals that indicate actual threats or important context
		if signal.Score < 0.3 && signal.Label != "MULTI_TURN" {
			continue // Skip low-confidence benign signals
		}

		threat := Threat{
			Category:    categorizeSignalCategory(&signal),
			Pattern:     string(signal.Source),
			Severity:    severityFromScore(signal.Score),
			Description: formatSignalDescription(&signal),
		}

		// Enhance description with category if available
		if signal.Category != "" {
			threat.Description = fmt.Sprintf("%s (%s)", threat.Description, signal.Category)
		}

		threats = append(threats, threat)
	}

	// Add semantic threat details if detected
	if result.SemanticScore > 0.5 && result.SemanticMatch != "" {
		// Check if we already have a semantic signal, if not add one
		hasSemantic := false
		for _, t := range threats {
			if t.Pattern == "semantic" {
				hasSemantic = true
				break
			}
		}
		if !hasSemantic {
			threats = append(threats, Threat{
				Category:    result.SemanticCategory,
				Pattern:     result.SemanticMatch,
				Severity:    severityFromScore(float64(result.SemanticScore)),
				Description: fmt.Sprintf("Semantic match: %s", result.SemanticCategory),
			})
		}
	}

	// Add obfuscation detection if present
	if len(result.ObfuscationTypes) > 0 {
		obfuscationTypes := make([]string, len(result.ObfuscationTypes))
		for i, ot := range result.ObfuscationTypes {
			obfuscationTypes[i] = string(ot)
		}
		threats = append(threats, Threat{
			Category:    "obfuscation",
			Pattern:     strings.Join(obfuscationTypes, ", "),
			Severity:    severityFromScore(result.HeuristicScore),
			Description: fmt.Sprintf("Obfuscation detected: %s", strings.Join(obfuscationTypes, ", ")),
		})
	}

	// Add multi-turn detection if present
	if result.MultiTurnPhase != "" && result.MultiTurnPhase != "BENIGN" {
		threats = append(threats, Threat{
			Category:    "multiturn_attack",
			Pattern:     result.MultiTurnPatternMatch,
			Severity:    severityFromScore(result.MultiTurnAggregateScore),
			Description: fmt.Sprintf("Multi-turn attack: %s phase (confidence: %.0f%%)",
				result.MultiTurnPhase, result.MultiTurnPhaseConf*100),
		})
	}

	// If no specific threats found but score is high, add generic threat
	if len(threats) == 0 && result.CombinedScore >= 0.5 {
		threats = append(threats, Threat{
			Category:    "suspicious",
			Pattern:     "aggregate_score",
			Severity:    severityFromScore(result.CombinedScore),
			Description: fmt.Sprintf("Combined detection score: %.0f%%", result.CombinedScore*100),
		})
	}

	return threats
}

// categorizeSignalCategory maps a detection signal to a threat category
func categorizeSignalCategory(signal *ml.DetectionSignal) string {
	if signal.Category != "" {
		return signal.Category
	}

	switch signal.Source {
	case ml.SignalSourceHeuristic:
		return "heuristic_detection"
	case ml.SignalSourceSemantic:
		return "semantic_similarity"
	case ml.SignalSourceBERT:
		if signal.Label == "INJECTION" {
			return "prompt_injection"
		}
		return "ml_classification"
	case ml.SignalSourceHugot:
		return "local_ml_detection"
	case ml.SignalSourceContext:
		return "context_anomaly"
	case ml.SignalSourceDeeperGo:
		return "deep_analysis"
	default:
		return "unknown"
	}
}

// formatSignalDescription creates a human-readable description from a detection signal
func formatSignalDescription(signal *ml.DetectionSignal) string {
	if len(signal.Reasons) > 0 {
		return strings.Join(signal.Reasons, "; ")
	}

	if signal.Label != "" {
		return fmt.Sprintf("%s detection from %s (confidence: %.0f%%)",
			signal.Label, signal.Source, signal.Confidence*100)
	}

	return fmt.Sprintf("Detection from %s layer", signal.Source)
}

func convertOutputFindings(findings []ml.OutputFinding) []Threat {
	var threats []Threat
	for _, f := range findings {
		threats = append(threats, Threat{
			Category:    f.Category,
			Pattern:     f.PatternName,
			Severity:    severityFromScore(float64(f.Severity) / 100.0),
			Description: f.Description,
		})
	}
	return threats
}

func getMLConfidence(result *ml.HybridResult) float64 {
	if result.LLMClassification != nil {
		return result.LLMClassification.Confidence
	}
	return 0.0
}

func severityFromScore(score float64) string {
	if score >= 0.8 {
		return "high"
	} else if score >= 0.5 {
		return "medium"
	}
	return "low"
}

func severityFromConfidence(confidence float64) string {
	if confidence >= 0.8 {
		return "high"
	} else if confidence >= 0.5 {
		return "medium"
	}
	return "low"
}

func extractThreatsFromScore(text string, score float64) []Threat {
	// When using heuristic-only mode (no hybrid detector), we rely on the
	// ThreatScorer's keyword weights to determine what patterns contributed to the score.
	// This provides production-grade threat extraction based on actual Citadel detection.
	var threats []Threat

	// Get matched keywords from Citadel's keyword weights
	// These are the actual patterns that contributed to the heuristic score
	matchedKeywords := ml.GetMatchedScorerKeywords(text)

	for _, keyword := range matchedKeywords {
		threats = append(threats, Threat{
			Category:    categorizeThreat(keyword),
			Pattern:     keyword,
			Severity:    severityFromScore(score),
			Description: fmt.Sprintf("Heuristic detection: '%s' pattern matched", keyword),
		})
	}

	// For high scores with no specific keywords matched, the ThreatScorer likely
	// detected specific attack patterns. The score ranges align with:
	// - 0.95: DAN jailbreak, high entropy, buried attacks
	// - 0.90: Markdown exfiltration, buried attacks
	// - 0.85: System prompt extraction patterns
	if score >= 0.85 && len(threats) == 0 {
		var description string
		switch {
		case score >= 0.95:
			description = "Critical threat: Likely DAN jailbreak, buried attack, or high-entropy payload"
		case score >= 0.90:
			description = "High threat: Likely exfiltration attempt or buried injection"
		default:
			description = "High threat: Likely system prompt extraction or policy injection"
		}
		threats = append(threats, Threat{
			Category:    "prompt_injection",
			Pattern:     "critical_heuristic_match",
			Severity:    "high",
			Description: description,
		})
	}

	return threats
}

// categorizeThreat maps a matched keyword to its threat category
func categorizeThreat(keyword string) string {
	// Map common keywords to their categories based on Citadel's threat taxonomy
	categoryMap := map[string]string{
		"ignore":           "instruction_override",
		"disregard":        "instruction_override",
		"override":         "instruction_override",
		"bypass":           "instruction_override",
		"system prompt":    "system_extraction",
		"instructions":     "system_extraction",
		"previous context": "context_manipulation",
		"jailbreak":        "jailbreak",
		"DAN":              "jailbreak",
		"roleplay":         "roleplay_attack",
		"act as":           "roleplay_attack",
		"pretend":          "roleplay_attack",
		"data":             "data_exfil",
		"exfiltrate":       "data_exfil",
		"send to":          "data_exfil",
		"API key":          "credential_leak",
		"password":         "credential_leak",
		"token":            "credential_leak",
		"secret":           "credential_leak",
	}

	for key, category := range categoryMap {
		if strings.Contains(strings.ToLower(keyword), key) {
			return category
		}
	}

	return "prompt_injection" // Default category
}
