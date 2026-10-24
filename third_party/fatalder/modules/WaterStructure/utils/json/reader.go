package sjson

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSONType represents the type of the next JSON value in the stream.
type JSONType byte

const (
	JSONInvalid JSONType = iota
	JSONNull
	JSONBool
	JSONNumber
	JSONString
	JSONArray
	JSONObject
)

func (t JSONType) String() string {
	switch t {
	case JSONNull:
		return "JSON_Null"
	case JSONBool:
		return "JSON_Bool"
	case JSONNumber:
		return "JSON_Number"
	case JSONString:
		return "JSON_String"
	case JSONArray:
		return "JSON_Array"
	case JSONObject:
		return "JSON_Object"
	default:
		return "JSON_Invalid"
	}
}

// JSONReader provides a streaming interface for reading JSON data in a similar fashion to NBT's TagReader.
// It exposes helpers for iterating through objects and arrays, peeking at upcoming value types, and skipping
// unwanted values without decoding the entire structure into memory at once.
type JSONReader struct {
	dec     *json.Decoder
	peeked  bool
	peekTok json.Token
}

// NewJSONReader creates a JSONReader backed by the provided io.Reader.
func NewJSONReader(r io.Reader) *JSONReader {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	return &JSONReader{dec: decoder}
}

// Offset reports the number of bytes read from the input.
func (r *JSONReader) Offset() int64 {
	if r == nil || r.dec == nil {
		return 0
	}
	return r.dec.InputOffset()
}

// BeginObject expects the next token to be an object start ('{').
func (r *JSONReader) BeginObject() error {
	return r.expectDelim('{')
}

// BeginArray expects the next token to be an array start ('[').
func (r *JSONReader) BeginArray() error {
	return r.expectDelim('[')
}

// NextObjectKey reads the next key inside an object. The returned boolean is true when the object has finished.
func (r *JSONReader) NextObjectKey() (string, bool, error) {
	tok, err := r.nextToken()
	if err != nil {
		return "", false, err
	}
	if delim, ok := tok.(json.Delim); ok {
		if delim == '}' {
			return "", true, nil
		}
		return "", false, UnexpectedTokenError{Expected: "字符串键或 '}'", Token: tok}
	}
	key, ok := tok.(string)
	if !ok {
		return "", false, UnexpectedTokenError{Expected: "字符串键", Token: tok}
	}
	return key, false, nil
}

// HasNextArrayValue checks if there are more values inside the current array.
func (r *JSONReader) HasNextArrayValue() (bool, error) {
	tok, err := r.nextToken()
	if err != nil {
		return false, err
	}
	if delim, ok := tok.(json.Delim); ok && delim == ']' {
		return false, nil
	}
	r.unreadToken(tok)
	return true, nil
}

// PeekType inspects the upcoming JSON value type without consuming it.
func (r *JSONReader) PeekType() (JSONType, error) {
	tok, err := r.nextToken()
	if err != nil {
		return JSONInvalid, err
	}
	r.unreadToken(tok)
	return jsonTypeFromToken(tok)
}

// ReadValue reads the next value and returns it together with its JSONType.
func (r *JSONReader) ReadValue() (interface{}, JSONType, error) {
	tok, err := r.nextToken()
	if err != nil {
		return nil, JSONInvalid, err
	}
	return r.consumeValue(tok)
}

// ReadObject reads the next value and ensures it is a JSON object, returning the decoded map.
func (r *JSONReader) ReadObject() (map[string]interface{}, error) {
	val, typ, err := r.ReadValue()
	if err != nil {
		return nil, err
	}
	if typ != JSONObject {
		return nil, UnexpectedTypeError{Expected: JSONObject, Actual: typ}
	}
	return val.(map[string]interface{}), nil
}

// ReadArray reads the next value and ensures it is a JSON array, returning the decoded slice.
func (r *JSONReader) ReadArray() ([]interface{}, error) {
	val, typ, err := r.ReadValue()
	if err != nil {
		return nil, err
	}
	if typ != JSONArray {
		return nil, UnexpectedTypeError{Expected: JSONArray, Actual: typ}
	}
	return val.([]interface{}), nil
}

// ReadString reads the next JSON string value.
func (r *JSONReader) ReadString() (string, error) {
	tok, err := r.nextToken()
	if err != nil {
		return "", err
	}
	s, ok := tok.(string)
	if !ok {
		return "", UnexpectedTokenError{Expected: "字符串", Token: tok}
	}
	return s, nil
}

// ReadBool reads the next JSON boolean value.
func (r *JSONReader) ReadBool() (bool, error) {
	tok, err := r.nextToken()
	if err != nil {
		return false, err
	}
	b, ok := tok.(bool)
	if !ok {
		return false, UnexpectedTokenError{Expected: "布尔值", Token: tok}
	}
	return b, nil
}

// ReadNumber reads the next JSON number token, returning it as json.Number.
func (r *JSONReader) ReadNumber() (json.Number, error) {
	tok, err := r.nextToken()
	if err != nil {
		return "", err
	}
	switch v := tok.(type) {
	case json.Number:
		return v, nil
	case float64:
		return json.Number(fmt.Sprintf("%g", v)), nil
	default:
		return "", UnexpectedTokenError{Expected: "数字", Token: tok}
	}
}

// ReadInt64 reads the next JSON number as an int64.
func (r *JSONReader) ReadInt64() (int64, error) {
	num, err := r.ReadNumber()
	if err != nil {
		return 0, err
	}
	val, err := num.Int64()
	if err != nil {
		return 0, fmt.Errorf("无效的 int64 数值: %w", err)
	}
	return val, nil
}

// ReadFloat64 reads the next JSON number as a float64.
func (r *JSONReader) ReadFloat64() (float64, error) {
	num, err := r.ReadNumber()
	if err != nil {
		return 0, err
	}
	val, err := num.Float64()
	if err != nil {
		return 0, fmt.Errorf("无效的 float64 数值: %w", err)
	}
	return val, nil
}

// ReadNull consumes the next JSON null value.
func (r *JSONReader) ReadNull() error {
	tok, err := r.nextToken()
	if err != nil {
		return err
	}
	if tok != nil {
		return UnexpectedTokenError{Expected: "空值", Token: tok}
	}
	return nil
}

// SkipValue skips over the next JSON value without allocating memory for it.
func (r *JSONReader) SkipValue() error {
	tok, err := r.nextToken()
	if err != nil {
		return err
	}
	return r.skipFromToken(tok)
}

func (r *JSONReader) readObjectValue() (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for {
		tok, err := r.nextToken()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok {
			if delim == '}' {
				return result, nil
			}
			return nil, UnexpectedTokenError{Expected: "字符串键或 '}'", Token: tok}
		}
		key, ok := tok.(string)
		if !ok {
			return nil, UnexpectedTokenError{Expected: "字符串键", Token: tok}
		}
		value, _, err := r.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("读取键 %q 的值失败: %w", key, err)
		}
		result[key] = value
	}
}

func (r *JSONReader) readArrayValue() ([]interface{}, error) {
	result := make([]interface{}, 0)
	for {
		tok, err := r.nextToken()
		if err != nil {
			return nil, err
		}
		if delim, ok := tok.(json.Delim); ok {
			if delim == ']' {
				return result, nil
			}
			r.unreadToken(tok)
			value, _, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			result = append(result, value)
			continue
		}
		r.unreadToken(tok)
		value, _, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
}

func (r *JSONReader) consumeValue(tok json.Token) (interface{}, JSONType, error) {
	switch v := tok.(type) {
	case json.Delim:
		if v == '{' {
			obj, err := r.readObjectValue()
			if err != nil {
				return nil, JSONInvalid, err
			}
			return obj, JSONObject, nil
		}
		if v == '[' {
			arr, err := r.readArrayValue()
			if err != nil {
				return nil, JSONInvalid, err
			}
			return arr, JSONArray, nil
		}
		return nil, JSONInvalid, UnexpectedTokenError{Expected: "值", Token: tok}
	case string:
		return v, JSONString, nil
	case json.Number:
		return v, JSONNumber, nil
	case float64:
		return json.Number(fmt.Sprintf("%g", v)), JSONNumber, nil
	case bool:
		return v, JSONBool, nil
	case nil:
		return nil, JSONNull, nil
	default:
		return nil, JSONInvalid, UnexpectedTokenError{Expected: "支持的值", Token: tok}
	}
}

func (r *JSONReader) skipFromToken(tok json.Token) error {
	switch v := tok.(type) {
	case json.Delim:
		if v == '{' {
			for {
				tok, err := r.nextToken()
				if err != nil {
					return err
				}
				if delim, ok := tok.(json.Delim); ok && delim == '}' {
					return nil
				}
				if _, ok := tok.(string); !ok {
					return UnexpectedTokenError{Expected: "字符串键", Token: tok}
				}
				if err := r.SkipValue(); err != nil {
					return err
				}
			}
		}
		if v == '[' {
			for {
				tok, err := r.nextToken()
				if err != nil {
					return err
				}
				if delim, ok := tok.(json.Delim); ok && delim == ']' {
					return nil
				}
				r.unreadToken(tok)
				if err := r.SkipValue(); err != nil {
					return err
				}
			}
		}
		return UnexpectedTokenError{Expected: "值", Token: tok}
	default:
		return nil
	}
}

func (r *JSONReader) expectDelim(expected rune) error {
	tok, err := r.nextToken()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || rune(delim) != expected {
		return UnexpectedTokenError{Expected: fmt.Sprintf("%q", expected), Token: tok}
	}
	return nil
}

func (r *JSONReader) nextToken() (json.Token, error) {
	if r.peeked {
		tok := r.peekTok
		r.peekTok = nil
		r.peeked = false
		return tok, nil
	}
	return r.dec.Token()
}

func (r *JSONReader) unreadToken(tok json.Token) {
	if r.peeked {
		panic("jsonreader: 仅支持一个令牌的预读")
	}
	r.peekTok = tok
	r.peeked = true
}

func jsonTypeFromToken(tok json.Token) (JSONType, error) {
	switch v := tok.(type) {
	case json.Delim:
		if v == '{' {
			return JSONObject, nil
		}
		if v == '[' {
			return JSONArray, nil
		}
		return JSONInvalid, UnexpectedTokenError{Expected: "值", Token: tok}
	case string:
		return JSONString, nil
	case json.Number, float64:
		return JSONNumber, nil
	case bool:
		return JSONBool, nil
	case nil:
		return JSONNull, nil
	default:
		return JSONInvalid, UnexpectedTokenError{Expected: "值", Token: tok}
	}
}

// UnexpectedTokenError indicates that a different JSON token was encountered than expected.
type UnexpectedTokenError struct {
	Expected string
	Token    json.Token
}

func (e UnexpectedTokenError) Error() string {
	return fmt.Sprintf("jsonreader: 期望 %s, 实际为 %v", e.Expected, describeToken(e.Token))
}

// UnexpectedTypeError indicates that a JSON value was not of the requested type.
type UnexpectedTypeError struct {
	Expected JSONType
	Actual   JSONType
}

func (e UnexpectedTypeError) Error() string {
	return fmt.Sprintf("jsonreader: 期望 %s, 实际为 %s", e.Expected, e.Actual)
}

func describeToken(tok json.Token) string {
	switch v := tok.(type) {
	case nil:
		return "空值"
	case bool:
		return fmt.Sprintf("布尔值(%t)", v)
	case string:
		return fmt.Sprintf("字符串(%q)", v)
	case json.Number:
		return fmt.Sprintf("数字(%s)", string(v))
	case float64:
		return fmt.Sprintf("数字(%g)", v)
	case json.Delim:
		return fmt.Sprintf("分隔符(%q)", rune(v))
	default:
		return fmt.Sprintf("%T", tok)
	}
}
