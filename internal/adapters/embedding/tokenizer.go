package embedding

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var encodeSingle = func(t *Tokenizer, text string) (encodeResult, error) {
	return t.encode(text)
}

const bertMaxSequenceLength = 512

type (
	encodeResult struct {
		InputIDs, AttentionMask, TokenTypeIDs []int64
	}
	Tokenizer struct {
		unkToken                 string
		vocab                    map[string]int
		splitRe                  *regexp.Regexp
		unkTokenID, clsID, sepID int
		lowercase                bool
	}
	tokenizerJSON struct {
		Model struct {
			Type     string         `json:"type"`
			UnkToken string         `json:"unk_token"`
			Vocab    map[string]int `json:"vocab"`
		} `json:"model"`
		Normalizer *struct {
			Type string `json:"type"`
		} `json:"normalizer"`
		PreTokenizer *struct {
			Type string `json:"type"`
		} `json:"pre_tokenizer"`
	}
)

func NewTokenizer(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer file %q: %w", path, err)
	}

	return newTokenizerFromBytes(data)
}

func newTokenizerFromBytes(data []byte) (*Tokenizer, error) {
	var tj tokenizerJSON

	err := json.Unmarshal(data, &tj)
	if err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}

	if tj.Model.Type != "WordPiece" && tj.Model.Type != "BertWordPiece" {
		if tj.Model.Type == "" && len(tj.Model.Vocab) == 0 {
			return nil, ErrTokenizerModelSectionMissing
		}
	}

	vocab := tj.Model.Vocab
	if len(vocab) == 0 {
		return nil, ErrTokenizerVocabEmpty
	}

	unkToken := "[UNK]"
	if tj.Model.UnkToken != "" {
		unkToken = tj.Model.UnkToken
	}

	unkID, ok := vocab[unkToken]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrTokenizerUnkTokenMissing, unkToken)
	}

	clsID, ok := vocab["[CLS]"]
	if !ok {
		return nil, ErrTokenizerCLSMissing
	}

	sepID, ok := vocab["[SEP]"]
	if !ok {
		return nil, ErrTokenizerSEPMissing
	}

	lowercase := tj.Normalizer != nil && tj.Normalizer.Type == "Lowercase"
	splitRe := regexp.MustCompile(`[^\s\p{P}]+|\p{P}`)

	return new(Tokenizer{
		vocab:      vocab,
		unkToken:   unkToken,
		unkTokenID: unkID,
		clsID:      clsID,
		sepID:      sepID,
		lowercase:  lowercase,
		splitRe:    splitRe,
	}), nil
}

func NewTokenizerFromVocab(path string) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vocab file %q: %w", path, err)
	}

	return newTokenizerFromVocabBytes(data)
}

func newTokenizerFromVocabBytes(data []byte) (*Tokenizer, error) {
	vocab := make(map[string]int)
	lineIndex := 0

	for line := range strings.SplitSeq(string(data), "\n") {
		token := strings.TrimRight(line, "\r")
		if token == "" {
			lineIndex++

			continue
		}

		vocab[token] = lineIndex
		lineIndex++
	}

	if len(vocab) == 0 {
		return nil, ErrVocabEmpty
	}

	unkToken := "[UNK]"

	unkID, ok := vocab[unkToken]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrVocabUnkTokenMissing, unkToken)
	}

	clsID, ok := vocab["[CLS]"]
	if !ok {
		return nil, ErrVocabCLSMissing
	}

	sepID, ok := vocab["[SEP]"]
	if !ok {
		return nil, ErrVocabSEPMissing
	}

	splitRe := regexp.MustCompile(`[^\s\p{P}]+|\p{P}`)

	return new(Tokenizer{
		vocab:      vocab,
		unkToken:   unkToken,
		unkTokenID: unkID,
		clsID:      clsID,
		sepID:      sepID,
		lowercase:  false,
		splitRe:    splitRe,
	}), nil
}

func (t *Tokenizer) encode(text string) (encodeResult, error) {
	if t == nil {
		return encodeResult{}, ErrTokenizerNil
	}

	tokens := t.basicTokenize(text)
	subTokens := t.wordpieceTokenize(tokens)

	const (
		bertSpecialTokenCount = 2 // [CLS] and [SEP]
		maxSubTokens          = bertMaxSequenceLength - bertSpecialTokenCount
	)
	if len(subTokens) > maxSubTokens {
		subTokens = subTokens[:maxSubTokens]
	}

	maxLen := len(subTokens) + bertSpecialTokenCount
	inputIDs := make([]int64, 0, maxLen)
	attentionMask := make([]int64, 0, maxLen)
	tokenTypeIDs := make([]int64, 0, maxLen)

	inputIDs = append(inputIDs, int64(t.clsID))
	attentionMask = append(attentionMask, 1)
	tokenTypeIDs = append(tokenTypeIDs, 0)

	for _, subToken := range subTokens {
		id, ok := t.vocab[subToken]
		if !ok {
			id = t.unkTokenID
		}

		inputIDs = append(inputIDs, int64(id))
		attentionMask = append(attentionMask, 1)
		tokenTypeIDs = append(tokenTypeIDs, 0)
	}

	inputIDs = append(inputIDs, int64(t.sepID))
	attentionMask = append(attentionMask, 1)
	tokenTypeIDs = append(tokenTypeIDs, 0)

	return encodeResult{
		InputIDs:      inputIDs,
		AttentionMask: attentionMask,
		TokenTypeIDs:  tokenTypeIDs,
	}, nil
}

func (t *Tokenizer) encodeBatch(texts []string) ([]encodeResult, error) {
	if t == nil {
		return nil, ErrTokenizerNil
	}

	results := make([]encodeResult, 0, len(texts))
	for _, text := range texts {
		result, err := encodeSingle(t, text)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return results, nil
}

func (t *Tokenizer) basicTokenize(text string) []string {
	if t.lowercase {
		text = strings.ToLower(text)
	}

	return t.splitRe.FindAllString(text, -1)
}

func (t *Tokenizer) wordpieceTokenize(tokens []string) []string {
	outputTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len(token) == 0 {
			continue
		}

		if _, ok := t.vocab[token]; ok {
			outputTokens = append(outputTokens, token)

			continue
		}

		isBad := true
		start := 0

		var subTokens []string

		for start < len(token) {
			end := len(token)

			var curSubStr string

			found := false

			for start < end {
				candidate := token[start:end]
				if start > 0 {
					candidate = "##" + candidate
				}

				if _, ok := t.vocab[candidate]; ok {
					curSubStr = candidate
					found = true

					break
				}

				end--
			}

			if !found {
				isBad = true

				break
			}

			subTokens = append(subTokens, curSubStr)
			start = end
			isBad = false
		}

		if isBad {
			outputTokens = append(outputTokens, t.unkToken)
		} else {
			outputTokens = append(outputTokens, subTokens...)
		}
	}

	return outputTokens
}
