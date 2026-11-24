package scanimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ffufResult struct {
	Input    map[string]flex `json:"input"`
	Status   int             `json:"status"`
	Length   int64           `json:"length"`
	Words    int             `json:"words"`
	Lines    int             `json:"lines"`
	URL      flex            `json:"url"`
	Redirect flex            `json:"redirectlocation"`
}

// parseFfuf reads ffuf -of json output, decoding results[] one element at a
// time. Each result URL becomes an http_service asset with the status and
// length seen; the fuzz input and redirect are untrusted detail. ffuf yields no
// findings. A file cut short keeps the results read before the cut.
func parseFfuf(c *collector, r io.Reader) error {
	c.res.Tool = ToolInfo{Name: "ffuf"}
	dec := json.NewDecoder(r)
	recognized := false
	idx := 0
	err := walkObject(dec, func(key string) error {
		if key != "results" {
			return skipValue(dec)
		}
		recognized = true
		return walkArray(dec, func() error {
			var res ffufResult
			ok, err := decodeLoose(dec, &res)
			if err != nil {
				return err
			}
			c.res.Records++
			if c.full() {
				return errStop
			}
			if ok {
				c.ffufResult(res, fmt.Sprintf("results[%d]", idx))
			} else {
				c.res.Skipped++
			}
			idx++
			return nil
		})
	})
	switch {
	case errors.Is(err, errStop):
		return nil
	case !recognized:
		return errors.New("scanimport: ffuf-json: no results array")
	case err != nil:
		c.res.Truncated = true
	}
	return nil
}

func (c *collector) ffufResult(r ffufResult, rec string) {
	u, _ := URL(r.URL.s())
	if u == "" {
		c.res.Skipped++
		return
	}
	var input []map[string]string
	for k, v := range r.Input {
		input = append(input, map[string]string{"keyword": k, "value": v.s()})
	}
	c.asset(Asset{Kind: KindHTTPService, URL: u, Status: httpStatus(r.Status), Length: max(r.Length, 0), Record: rec,
		Detail: buildDetail(map[string]any{"input": input, "words": r.Words, "lines": r.Lines,
			"redirectlocation": StripUserinfo(r.Redirect.s())})})
}
