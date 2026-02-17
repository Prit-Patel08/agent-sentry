package tokens

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// Pricing per 1K tokens (approximate blended input/output for simplicity, or specific output)
// Using Output pricing as we are monitoring output logs.
var pricing = map[string]float64{
	"gpt-4":           0.06,  // $0.06 / 1K tokens (Output)
	"gpt-4-turbo":     0.03,  // $0.03 / 1K tokens
	"gpt-3.5-turbo":   0.002, // $0.002 / 1K tokens
	"claude-3-opus":   0.075, // $0.075 / 1K
	"claude-3-sonnet": 0.015, // $0.015 / 1K
}

var (
	encoder *tiktoken.Tiktoken
	once    sync.Once
)

// Count returns the number of tokens in the text for the given model.
// If model is unknown, defaults to gpt-4 encoding (cl100k_base).
func Count(text string, model string) int {
	// Lazy load encoder to avoid startup penalty
	once.Do(func() {
		var err error
		// identifying the encoding for a specific model
		// if model not found, fallback to cl100k_base (standard for GPT-4/3.5)
		encoder, err = tiktoken.EncodingForModel(model)
		if err != nil {
			encoder, _ = tiktoken.GetEncoding("cl100k_base")
		}
	})

	// tiktoken-go might panic on nil encoder if init failed entirely (unlikely with fallback)
	if encoder == nil {
		return len(text) / 4 // Rough character approximation fallback
	}

	return len(encoder.Encode(text, nil, nil))
}

// EstimateCost calculates cost based on token count and model price.
func EstimateCost(tokens int, model string) float64 {
	pricePer1K, ok := pricing[model]
	if !ok {
		// Default to GPT-4 pricing if unknown, to be safe/conservative
		pricePer1K = pricing["gpt-4"]
	}
	return (float64(tokens) / 1000.0) * pricePer1K
}
