package sjson

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONReader_StreamObject(t *testing.T) {
	data := `{"width":10,"height":20,"blocks":[1,true,null],"meta":{"name":"demo"},"ignored":42}`
	reader := NewJSONReader(strings.NewReader(data))
	if err := reader.BeginObject(); err != nil {
		t.Fatalf("begin object: %v", err)
	}
	var width, height int64
	var blocks []interface{}
	var metaName string
	for {
		key, done, err := reader.NextObjectKey()
		if err != nil {
			t.Fatalf("next key: %v", err)
		}
		if done {
			break
		}
		switch key {
		case "width":
			width, err = reader.ReadInt64()
			if err != nil {
				t.Fatalf("read width: %v", err)
			}
		case "height":
			height, err = reader.ReadInt64()
			if err != nil {
				t.Fatalf("read height: %v", err)
			}
		case "blocks":
			if err := reader.BeginArray(); err != nil {
				t.Fatalf("begin blocks: %v", err)
			}
			for {
				hasNext, err := reader.HasNextArrayValue()
				if err != nil {
					t.Fatalf("blocks has next: %v", err)
				}
				if !hasNext {
					break
				}
				typ, err := reader.PeekType()
				if err != nil {
					t.Fatalf("peek type: %v", err)
				}
				switch typ {
				case JSONNumber:
					v, err := reader.ReadInt64()
					if err != nil {
						t.Fatalf("read number: %v", err)
					}
					blocks = append(blocks, v)
				case JSONBool:
					v, err := reader.ReadBool()
					if err != nil {
						t.Fatalf("read bool: %v", err)
					}
					blocks = append(blocks, v)
				case JSONNull:
					if err := reader.ReadNull(); err != nil {
						t.Fatalf("read null: %v", err)
					}
					blocks = append(blocks, nil)
				default:
					t.Fatalf("unexpected block type: %v", typ)
				}
			}
		case "meta":
			if err := reader.BeginObject(); err != nil {
				t.Fatalf("begin meta: %v", err)
			}
			for {
				innerKey, done, err := reader.NextObjectKey()
				if err != nil {
					t.Fatalf("meta next key: %v", err)
				}
				if done {
					break
				}
				if innerKey != "name" {
					t.Fatalf("unexpected inner key %q", innerKey)
				}
				metaName, err = reader.ReadString()
				if err != nil {
					t.Fatalf("read meta name: %v", err)
				}
			}
		default:
			if err := reader.SkipValue(); err != nil {
				t.Fatalf("skip value %q: %v", key, err)
			}
		}
	}
	if width != 10 || height != 20 {
		t.Fatalf("unexpected dimensions %d %d", width, height)
	}
	if len(blocks) != 3 || blocks[0].(int64) != 1 || blocks[1].(bool) != true || blocks[2] != nil {
		t.Fatalf("unexpected blocks %v", blocks)
	}
	if metaName != "demo" {
		t.Fatalf("unexpected meta name %q", metaName)
	}
}

func TestJSONReader_ReadValues(t *testing.T) {
	data := `[{"a":1},["x",2],true,null,"str"]`
	reader := NewJSONReader(bytes.NewReader([]byte(data)))
	if err := reader.BeginArray(); err != nil {
		t.Fatalf("begin array: %v", err)
	}
	// object
	if has, err := reader.HasNextArrayValue(); err != nil || !has {
		t.Fatalf("array has first: %v", err)
	}
	val, typ, err := reader.ReadValue()
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if typ != JSONObject {
		t.Fatalf("expected object, got %v", typ)
	}
	obj := val.(map[string]interface{})
	if len(obj) != 1 || obj["a"].(json.Number).String() != "1" {
		t.Fatalf("unexpected object %v", obj)
	}
	// array
	if has, err := reader.HasNextArrayValue(); err != nil || !has {
		t.Fatalf("array has second: %v", err)
	}
	arr, err := reader.ReadArray()
	if err != nil {
		t.Fatalf("read inner array: %v", err)
	}
	if len(arr) != 2 || arr[0].(string) != "x" || arr[1].(json.Number).String() != "2" {
		t.Fatalf("unexpected inner array %v", arr)
	}
	// bool
	if has, err := reader.HasNextArrayValue(); err != nil || !has {
		t.Fatalf("array has third: %v", err)
	}
	vBool, err := reader.ReadBool()
	if err != nil || vBool != true {
		t.Fatalf("read bool: %v %v", vBool, err)
	}
	// null
	if has, err := reader.HasNextArrayValue(); err != nil || !has {
		t.Fatalf("array has fourth: %v", err)
	}
	if err := reader.ReadNull(); err != nil {
		t.Fatalf("read null: %v", err)
	}
	// string
	if has, err := reader.HasNextArrayValue(); err != nil || !has {
		t.Fatalf("array has fifth: %v", err)
	}
	str, err := reader.ReadString()
	if err != nil || str != "str" {
		t.Fatalf("read string: %v %v", str, err)
	}
	if has, err := reader.HasNextArrayValue(); err != nil || has {
		t.Fatalf("array should be finished: %v %v", has, err)
	}
}

func TestJSONReader_SkipValue(t *testing.T) {
	data := `{"skip":{"nested":[1,2,3]},"keep":5}`
	reader := NewJSONReader(strings.NewReader(data))
	if err := reader.BeginObject(); err != nil {
		t.Fatalf("begin object: %v", err)
	}
	key, done, err := reader.NextObjectKey()
	if err != nil || done || key != "skip" {
		t.Fatalf("unexpected first key %q %v", key, err)
	}
	if err := reader.SkipValue(); err != nil {
		t.Fatalf("skip nested value: %v", err)
	}
	key, done, err = reader.NextObjectKey()
	if err != nil || done || key != "keep" {
		t.Fatalf("unexpected second key %q %v", key, err)
	}
	val, err := reader.ReadInt64()
	if err != nil || val != 5 {
		t.Fatalf("read keep: %v %v", val, err)
	}
	key, done, err = reader.NextObjectKey()
	if err != nil || !done {
		t.Fatalf("object not finished: %v %v", done, err)
	}
}
