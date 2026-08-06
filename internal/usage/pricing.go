package usage

// modelPricing maps LLM/embedding model names to per-1K-token costs in USD.
// Input cost is for prompt tokens; output cost is for completion tokens.
// For embedding models, only InputPer1K is used (output is 0).
var modelPricing = map[string]struct {
	InputPer1K  float64
	OutputPer1K float64
}{
	"qwen-max":          {0.002, 0.006},
	"qwen-plus":         {0.001, 0.003},
	"qwen-turbo":        {0.0005, 0.0015},
	"text-embedding-v4": {0.0001, 0},
	"text-embedding-v3": {0.0001, 0},
}

// computeCost calculates the estimated cost for a given model and token counts.
// Returns 0 if the model is not in the pricing table.
func computeCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := modelPricing[model]
	if !ok {
		return 0
	}
	inputCost := (float64(inputTokens) / 1000.0) * p.InputPer1K
	outputCost := (float64(outputTokens) / 1000.0) * p.OutputPer1K
	return inputCost + outputCost
}
