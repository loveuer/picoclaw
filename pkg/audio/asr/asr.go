package asr

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/audio"
	"github.com/sipeed/picoclaw/pkg/config"
)

const elevenLabsSupportedModelID = "scribe_v1"

func ElevenLabsSupportedModelID() string {
	return elevenLabsSupportedModelID
}

type Transcriber interface {
	Name() string
	Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error)
}

type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

func supportsAudioTranscription(modelCfg *config.ModelConfig) bool {
	protocol, _, _ := audio.ResolveModel(modelCfg)
	return protocol == "openai"
}

func supportsWhisperTranscription(modelCfg *config.ModelConfig) bool {
	protocol, _, _ := audio.ResolveModel(modelCfg)
	return protocol == "openai" || protocol == "groq" || protocol == "mimo"
}

func whisperModelID(modelCfg *config.ModelConfig) string {
	if modelCfg == nil || modelCfg.APIKey() == "" {
		return ""
	}

	if !supportsWhisperTranscription(modelCfg) {
		return ""
	}

	_, modelID, _ := audio.ResolveModel(modelCfg)
	if strings.Contains(strings.ToLower(modelID), "whisper") {
		return modelID
	}
	return ""
}

func isElevenLabsTranscriptionModel(modelCfg *config.ModelConfig) bool {
	if modelCfg == nil || modelCfg.APIKey() == "" {
		return false
	}

	protocol, _, _ := audio.ResolveModel(modelCfg)
	return protocol == "elevenlabs"
}

func transcriberFromModelConfig(modelCfg *config.ModelConfig) Transcriber {
	if modelCfg == nil {
		return nil
	}

	if isElevenLabsTranscriptionModel(modelCfg) {
		_, modelID, _ := audio.ResolveModel(modelCfg)
		return NewElevenLabsTranscriber(modelCfg.APIKey(), modelCfg.APIBase, modelID)
	}
	if modelID := whisperModelID(modelCfg); modelID != "" {
		return NewWhisperTranscriber(modelCfg)
	}
	if supportsAudioTranscription(modelCfg) {
		return NewAudioModelTranscriber(modelCfg)
	}
	return nil
}

func fallbackTranscriberFromModelConfig(modelCfg *config.ModelConfig) Transcriber {
	if modelCfg == nil {
		return nil
	}

	if isElevenLabsTranscriptionModel(modelCfg) {
		_, modelID, _ := audio.ResolveModel(modelCfg)
		return NewElevenLabsTranscriber(modelCfg.APIKey(), modelCfg.APIBase, modelID)
	}
	if modelID := whisperModelID(modelCfg); modelID != "" {
		return NewWhisperTranscriber(modelCfg)
	}
	return nil
}

// DetectTranscriber inspects cfg and returns the appropriate Transcriber, or
// nil if no supported transcription provider is configured.
func DetectTranscriber(cfg *config.Config) Transcriber {
	if cfg == nil {
		return nil
	}

	if modelName := strings.TrimSpace(cfg.Voice.ModelName); modelName != "" {
		modelCfg, err := cfg.GetModelConfig(modelName)
		if err == nil {
			if tr := transcriberFromModelConfig(modelCfg); tr != nil {
				return tr
			}
		}
	}

	// Fall back to compatibility scanning for legacy auto-detected ASR providers.
	for _, mc := range cfg.ModelList {
		if tr := fallbackTranscriberFromModelConfig(mc); tr != nil {
			return tr
		}
	}
	return nil
}
