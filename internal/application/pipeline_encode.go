package application

import (
	"context"
	"github.com/flexer2006/tco/internal/domain"
)

func (o *Orchestrator) encode(ctx context.Context, messages []domain.RawCanonicalMessage) ([]dedupClusterInput, error) {
	if len(messages) == 0 {
		return []dedupClusterInput{}, nil
	}
	result := make([]dedupClusterInput, 0, len(messages))
	if o.Policy.BatchMode() == domain.Streaming {
		for start := 0; start < len(messages); start += o.Policy.BatchSize() {
			end := min(start+o.Policy.BatchSize(), len(messages))
			texts := make([]string, end-start)
			for i := range end - start {
				texts[i] = messages[start+i].Text()
			}
			vectors, err := o.Encoder.Encode(ctx, texts)
			if err != nil {
				return nil, err
			}
			for i := range vectors {
				result = append(result, dedupClusterInput{RawMessage: messages[start+i], Embedding: vectors[i]})
			}
		}
		return result, nil
	}
	texts := make([]string, len(messages))
	for i := range messages {
		texts[i] = messages[i].Text()
	}
	for start := 0; start < len(texts); start += o.Policy.BatchSize() {
		end := min(start+o.Policy.BatchSize(), len(texts))
		vectors, err := o.Encoder.Encode(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		for i := range vectors {
			result = append(result, dedupClusterInput{RawMessage: messages[start+i], Embedding: vectors[i]})
		}
	}
	return result, nil
}
