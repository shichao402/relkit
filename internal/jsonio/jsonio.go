package jsonio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

type object map[string]any

// MarshalCompact emits UTF-8 JSON with sorted keys and no extra whitespace.
func MarshalCompact(v any) ([]byte, error) {
	normalized, err := normalize(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeValue(&buf, normalized, 0, false); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalPretty emits UTF-8 JSON with sorted keys, two-space indentation, and a
// trailing newline.
func MarshalPretty(v any) ([]byte, error) {
	normalized, err := normalize(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeValue(&buf, normalized, 0, true); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func LoadBytes(data []byte, dest any) error {
	if bytes.HasPrefix(data, utf8BOM) {
		return fmt.Errorf("unexpected UTF-8 BOM")
	}
	return decode(data, dest)
}

func LoadBytesLenient(data []byte, dest any) error {
	if bytes.HasPrefix(data, utf8BOM) {
		data = data[len(utf8BOM):]
	}
	return decode(data, dest)
}

func LoadPath(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return LoadBytes(data, dest)
}

func LoadPathLenient(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return LoadBytesLenient(data, dest)
}

func WritePath(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func decode(data []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("unexpected trailing JSON content")
	}
	return nil
}

func normalize(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}

	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}

	if v.CanInterface() {
		switch typed := v.Interface().(type) {
		case json.RawMessage:
			var decoded any
			if err := LoadBytes([]byte(typed), &decoded); err != nil {
				return nil, err
			}
			return normalize(reflect.ValueOf(decoded))
		case json.Number:
			return typed, nil
		}
	}

	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.String:
		return v.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint(), nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	case reflect.Slice, reflect.Array:
		items := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := normalize(v.Index(i))
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return items, nil
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type %s", v.Type().Key())
		}
		result := make(object, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			item, err := normalize(iter.Value())
			if err != nil {
				return nil, err
			}
			result[iter.Key().String()] = item
		}
		return result, nil
	case reflect.Struct:
		return normalizeStruct(v)
	default:
		return nil, fmt.Errorf("unsupported JSON value kind %s", v.Kind())
	}
}

func normalizeStruct(v reflect.Value) (object, error) {
	t := v.Type()
	result := make(object)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}

		name, omitEmpty, skip := jsonFieldName(field)
		if skip {
			continue
		}
		value := v.Field(i)
		if omitEmpty && isEmpty(value) {
			continue
		}
		normalized, err := normalize(value)
		if err != nil {
			return nil, err
		}
		result[name] = normalized
	}
	return result, nil
}

func jsonFieldName(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return field.Name, false, false
	}

	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		name = field.Name
	} else {
		name = parts[0]
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	case reflect.Struct:
		return false
	default:
		return false
	}
}

func writeValue(buf *bytes.Buffer, v any, depth int, pretty bool) error {
	switch typed := v.(type) {
	case nil, bool, string, int64, uint64, float64, json.Number:
		return writeScalar(buf, typed)
	case int:
		return writeScalar(buf, int64(typed))
	case int8:
		return writeScalar(buf, int64(typed))
	case int16:
		return writeScalar(buf, int64(typed))
	case int32:
		return writeScalar(buf, int64(typed))
	case uint:
		return writeScalar(buf, uint64(typed))
	case uint8:
		return writeScalar(buf, uint64(typed))
	case uint16:
		return writeScalar(buf, uint64(typed))
	case uint32:
		return writeScalar(buf, uint64(typed))
	case []any:
		return writeArray(buf, typed, depth, pretty)
	case object:
		return writeObject(buf, typed, depth, pretty)
	default:
		return fmt.Errorf("unsupported normalized value type %T", typed)
	}
}

func writeScalar(buf *bytes.Buffer, v any) error {
	var scalar bytes.Buffer
	encoder := json.NewEncoder(&scalar)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return err
	}
	data := bytes.TrimSuffix(scalar.Bytes(), []byte{'\n'})
	_, err := buf.Write(data)
	return err
}

func writeArray(buf *bytes.Buffer, values []any, depth int, pretty bool) error {
	if len(values) == 0 {
		buf.WriteString("[]")
		return nil
	}
	buf.WriteByte('[')
	if pretty {
		buf.WriteByte('\n')
	}
	for i, value := range values {
		if pretty {
			writeIndent(buf, depth+1)
		}
		if err := writeValue(buf, value, depth+1, pretty); err != nil {
			return err
		}
		if i < len(values)-1 {
			buf.WriteByte(',')
		}
		if pretty {
			buf.WriteByte('\n')
		}
	}
	if pretty {
		writeIndent(buf, depth)
	}
	buf.WriteByte(']')
	return nil
}

func writeObject(buf *bytes.Buffer, values object, depth int, pretty bool) error {
	if len(values) == 0 {
		buf.WriteString("{}")
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buf.WriteByte('{')
	if pretty {
		buf.WriteByte('\n')
	}
	for i, key := range keys {
		if pretty {
			writeIndent(buf, depth+1)
		}
		if err := writeScalar(buf, key); err != nil {
			return err
		}
		if pretty {
			buf.WriteString(": ")
		} else {
			buf.WriteByte(':')
		}
		if err := writeValue(buf, values[key], depth+1, pretty); err != nil {
			return err
		}
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		if pretty {
			buf.WriteByte('\n')
		}
	}
	if pretty {
		writeIndent(buf, depth)
	}
	buf.WriteByte('}')
	return nil
}

func writeIndent(buf *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString("  ")
	}
}
