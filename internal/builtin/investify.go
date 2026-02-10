package builtin

import (
	_ "embed"
	"encoding/json"
)

//go:embed investify_responses.json
var investifyResponsesRaw []byte

//go:embed investify_keywords.json
var investifyKeywordsRaw []byte

//go:embed ListOfBrokersPSX.pdf
var listOfBrokersPSXPDFRaw []byte

type InvestifyReplies map[string]map[string]string

type DelayRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type KeywordEntry struct {
	Keywords   []string   `json:"keywords"`
	ReplyID    string     `json:"reply_id"`
	Language   string     `json:"language"`
	DelayRange DelayRange `json:"delay_range"`
}

type keywordFile struct {
	Responses []KeywordEntry `json:"responses"`
}

func LoadInvestifyReplies() (InvestifyReplies, error) {
	var replies InvestifyReplies
	if err := json.Unmarshal(investifyResponsesRaw, &replies); err != nil {
		return nil, err
	}
	return replies, nil
}

func LoadInvestifyKeywordEntries() ([]KeywordEntry, error) {
	var payload keywordFile
	if err := json.Unmarshal(investifyKeywordsRaw, &payload); err != nil {
		return nil, err
	}
	return payload.Responses, nil
}

func LoadBrokersPDF() ([]byte, string) {
	return listOfBrokersPSXPDFRaw, "ListOfBrokersPSX.pdf"
}
