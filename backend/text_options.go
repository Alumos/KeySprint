package main

import (
	"encoding/json"
	"strings"
)

// textOptions are the per-competition text switches a teacher selects when
// creating a competition. They are stored as one JSON object so a new switch
// never needs a schema migration; pinyin_enabled is still written alongside for
// older pages that only understand the original boolean.
type textOptions struct {
	Pinyin       bool `json:"pinyin"`
	StripSymbols bool `json:"stripSymbols"`
	UnifySymbols bool `json:"unifySymbols"`
	StripBreaks  bool `json:"stripBreaks"`
	SegmentWords bool `json:"segmentWords"`
}

func defaultTextOptions() textOptions {
	return textOptions{Pinyin: true}
}

// parseTextOptions decodes the stored JSON. Rows written before text options
// existed (or rows with a broken payload) fall back to the legacy column.
func parseTextOptions(raw string, pinyinEnabled int) textOptions {
	options := defaultTextOptions()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		options.Pinyin = pinyinEnabled != 0
		return options
	}
	var stored struct {
		Pinyin       *bool `json:"pinyin"`
		StripSymbols bool  `json:"stripSymbols"`
		UnifySymbols bool  `json:"unifySymbols"`
		StripBreaks  bool  `json:"stripBreaks"`
		SegmentWords bool  `json:"segmentWords"`
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		options.Pinyin = pinyinEnabled != 0
		return options
	}
	if stored.Pinyin == nil {
		options.Pinyin = pinyinEnabled != 0
	} else {
		options.Pinyin = *stored.Pinyin
	}
	options.StripSymbols = stored.StripSymbols
	options.UnifySymbols = stored.UnifySymbols
	options.StripBreaks = stored.StripBreaks
	options.SegmentWords = stored.SegmentWords
	return options
}

func (o textOptions) encode() string {
	encoded, err := json.Marshal(o)
	if err != nil {
		return ""
	}
	return string(encoded)
}
