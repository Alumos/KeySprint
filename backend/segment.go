package main

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/go-ego/gse"
)

const (
	segmentMaxRunes = 8000
	segmentCacheMax = 64
)

var (
	segmenterOnce sync.Once
	segmenter     gse.Segmenter
	segmenterErr  error
)

var segmentCache = struct {
	sync.Mutex
	items map[[32]byte][]int
}{items: make(map[[32]byte][]int, segmentCacheMax)}

// segmenterInstance loads the embedded simplified-Chinese jieba dictionary on
// first use. Loading costs ~300ms, so classrooms that never enable the word
// segmentation option never pay for it.
func segmenterInstance() (gse.Segmenter, error) {
	segmenterOnce.Do(func() {
		segmenter, segmenterErr = gse.NewEmbed("zh_s")
	})
	return segmenter, segmenterErr
}

func startsWithHan(text string) bool {
	r, _ := utf8.DecodeRuneInString(text)
	return unicode.Is(unicode.Han, r)
}

// wordStarts returns the rune offsets where a new Chinese word begins. Only a
// token starting with a Han character opens a word, so punctuation and spaces
// stay attached to the preceding word instead of forming visual blocks.
func wordStarts(text string) ([]int, error) {
	if text == "" {
		return []int{}, nil
	}
	total := utf8.RuneCountInString(text)
	if total > segmentMaxRunes {
		return nil, errors.New("待分词文本过长")
	}
	key := sha256.Sum256([]byte(text))
	segmentCache.Lock()
	if cached, ok := segmentCache.items[key]; ok {
		segmentCache.Unlock()
		return cached, nil
	}
	segmentCache.Unlock()

	seg, err := segmenterInstance()
	if err != nil {
		return nil, errors.New("分词词典初始化失败")
	}
	segs := seg.Segment([]byte(text))
	if len(segs) == 0 {
		return []int{0}, nil
	}
	starts := make([]int, 0, len(segs)/2+1)
	runePos, bytePos := 0, 0
	for _, item := range segs {
		start, end := item.Start(), item.End()
		if start < bytePos || end <= start || end > len(text) {
			return nil, errors.New("分词结果异常")
		}
		if start > bytePos {
			runePos += utf8.RuneCountInString(text[bytePos:start])
		}
		part := text[start:end]
		if len(starts) == 0 || startsWithHan(part) {
			if len(starts) == 0 || starts[len(starts)-1] != runePos {
				starts = append(starts, runePos)
			}
		}
		runePos += utf8.RuneCountInString(part)
		bytePos = end
	}
	if bytePos < len(text) {
		runePos += utf8.RuneCountInString(text[bytePos:])
	}
	if runePos != total {
		return nil, errors.New("分词结果长度不一致")
	}
	if starts[0] != 0 {
		starts = append([]int{0}, starts...)
	}

	cached := append([]int(nil), starts...)
	segmentCache.Lock()
	if len(segmentCache.items) >= segmentCacheMax {
		segmentCache.items = make(map[[32]byte][]int, segmentCacheMax)
	}
	segmentCache.items[key] = cached
	segmentCache.Unlock()
	return starts, nil
}

// segmentText is public but requires a live competition code, mirroring the
// public article endpoint: the student page transforms the article locally,
// then asks the server to resolve word boundaries for exactly that text.
func (a *app) segmentText(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	code := cleanCode(in.Code)
	if code == "" {
		writeError(w, http.StatusForbidden, "缺少比赛编号")
		return
	}
	var status string
	if err := a.db.QueryRow("SELECT status FROM competitions WHERE access_code=?", code).Scan(&status); err != nil || (status != "waiting" && status != "running") {
		writeError(w, http.StatusForbidden, "比赛不存在或已结束")
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"starts": []int{}})
		return
	}
	if utf8.RuneCountInString(in.Text) > segmentMaxRunes {
		writeError(w, http.StatusBadRequest, "待分词文本不能超过 8000 字")
		return
	}
	starts, err := wordStarts(in.Text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "分词失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"starts": starts})
}
