package scanimport

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// errStop ends a streaming parse once MaxItems is reached (Truncated is
// already set).
var errStop = errors.New("scanimport: item cap reached")

// flex decodes a JSON string, number or bool into its text; null, objects and
// arrays decode to "". Scanners disagree on whether ids and codes are strings
// or numbers.
type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case len(b) == 0 || string(b) == "null" || b[0] == '{' || b[0] == '[':
		*f = ""
	case b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flex(s)
	default:
		*f = flex(b)
	}
	return nil
}

func (f flex) s() string { return string(f) }

// flexList decodes a string or an array of strings/numbers (a bare string is
// split on commas only for tags, by the caller; here it is one element).
type flexList []string

func (l *flexList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	*l = nil
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] != '[' {
		var f flex
		if err := f.UnmarshalJSON(b); err != nil {
			return err
		}
		if f != "" {
			*l = flexList{string(f)}
		}
		return nil
	}
	var items []flex
	if err := json.Unmarshal(b, &items); err != nil {
		return err
	}
	for _, f := range items {
		if f != "" {
			*l = append(*l, string(f))
		}
	}
	return nil
}

// decodeLoose decodes the next value into v. A value of the wrong type is
// consumed and reported as ok=false with a nil error; any other error (syntax,
// EOF) is returned.
func decodeLoose(dec *json.Decoder, v any) (ok bool, err error) {
	err = dec.Decode(v)
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return false, nil
	}
	return err == nil, err
}

func skipValue(dec *json.Decoder) error {
	var raw json.RawMessage
	return dec.Decode(&raw)
}

var (
	errNotObject = errors.New("scanimport: not a JSON object")
	errNotArray  = errors.New("scanimport: not a JSON array")
)

// walkObject consumes one JSON object, calling fn for each key; fn must
// consume the value (decodeLoose or skipValue). errNotObject is returned when
// the next token is not '{'.
func walkObject(dec *json.Decoder, fn func(key string) error) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return errNotObject
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := kt.(string)
		if err := fn(key); err != nil {
			return err
		}
	}
	_, err = dec.Token() // '}'
	return err
}

// walkArray consumes one JSON array, calling fn once per element; fn must
// consume the element. A 'null' array is accepted as empty.
func walkArray(dec *json.Decoder, fn func() error) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return nil
	}
	if tok != json.Delim('[') {
		if tok == json.Delim('{') || tok == json.Delim(']') {
			return errNotArray
		}
		return nil // scalar where an array was expected: ignore
	}
	for dec.More() {
		if err := fn(); err != nil {
			return err
		}
	}
	_, err = dec.Token() // ']'
	return err
}

// newXMLDecoder returns a decoder that accepts the single-byte charsets nmap
// and Burp may declare. It never resolves entities or external subsets.
func newXMLDecoder(r io.Reader) *xml.Decoder {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = func(label string, in io.Reader) (io.Reader, error) {
		switch label {
		case "iso-8859-1", "ISO-8859-1", "latin1", "us-ascii", "US-ASCII", "windows-1252":
			return &latin1Reader{r: in}, nil
		}
		return nil, fmt.Errorf("unsupported charset %q", label)
	}
	return dec
}

type latin1Reader struct {
	r   io.Reader
	buf []byte
}

func (l *latin1Reader) Read(p []byte) (int, error) {
	if len(p) < 4 {
		return 0, io.ErrShortBuffer
	}
	if cap(l.buf) < len(p)/2 {
		l.buf = make([]byte, len(p)/2)
	}
	n, err := l.r.Read(l.buf[:len(p)/2])
	out := 0
	for _, b := range l.buf[:n] {
		if b < 0x80 {
			p[out] = b
			out++
		} else {
			p[out], p[out+1] = 0xC0|b>>6, 0x80|b&0x3F
			out += 2
		}
	}
	return out, err
}
