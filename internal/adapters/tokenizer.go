package adapters

import (
	"encoding/json"
	"errors"
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
		unkToken   string
		vocab      map[string]int
		unkTokenID int
		lowercase  bool
		splitRe    *regexp.Regexp
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
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}
	if tj.Model.Type != "WordPiece" && tj.Model.Type != "BertWordPiece" {
		if tj.Model.Type == "" && len(tj.Model.Vocab) == 0 {
			return nil, errors.New("tokenizer.json: model section not found or empty vocab")
		}
	}
	vocab := tj.Model.Vocab
	if len(vocab) == 0 {
		return nil, errors.New("tokenizer.json: vocab must not be empty")
	}
	unkToken := "[UNK]"
	if tj.Model.UnkToken != "" {
		unkToken = tj.Model.UnkToken
	}
	unkID, ok := vocab[unkToken]
	if !ok {
		return nil, fmt.Errorf("tokenizer.json: unk_token %q not found in vocab", unkToken)
	}
	lowercase := tj.Normalizer != nil && tj.Normalizer.Type == "Lowercase"
	splitRe := regexp.MustCompile(`[^\s\p{P}]+|\p{P}`)
	return &Tokenizer{
		vocab:      vocab,
		unkToken:   unkToken,
		unkTokenID: unkID,
		lowercase:  lowercase,
		splitRe:    splitRe,
	}, nil
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
		return nil, errors.New("vocab: must not be empty")
	}
	unkToken := "[UNK]"
	unkID, ok := vocab[unkToken]
	if !ok {
		return nil, fmt.Errorf("vocab: unk_token %q not found in vocab", unkToken)
	}
	splitRe := regexp.MustCompile(`[^\s\p{P}]+|\p{P}`)
	return &Tokenizer{
		vocab:      vocab,
		unkToken:   unkToken,
		unkTokenID: unkID,
		lowercase:  false,
		splitRe:    splitRe,
	}, nil
}

func (t *Tokenizer) encode(text string) (encodeResult, error) {
	if t == nil {
		return encodeResult{}, errors.New("tokenizer must not be nil")
	}
	tokens := t.basicTokenize(text)
	subTokens := t.wordpieceTokenize(tokens)
	const maxSubTokens = bertMaxSequenceLength - 2
	if len(subTokens) > maxSubTokens {
		subTokens = subTokens[:maxSubTokens]
	}
	maxLen := len(subTokens) + 2
	inputIDs := make([]int64, 0, maxLen)
	attentionMask := make([]int64, 0, maxLen)
	tokenTypeIDs := make([]int64, 0, maxLen)
	inputIDs = append(inputIDs, int64(t.vocab["[CLS]"]))
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
	inputIDs = append(inputIDs, int64(t.vocab["[SEP]"]))
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
		return nil, errors.New("tokenizer must not be nil")
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
	var outputTokens []string
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
