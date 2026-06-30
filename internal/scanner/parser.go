package scanner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParsedField represents a single key-value pair extracted from a structured file
type ParsedField struct {
	Key   string // dot-notation path (e.g., "database.password")
	Value string // the actual value
}

// CanParse reports whether the given filename is in a format ParseFile supports.
func CanParse(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if strings.HasPrefix(base, ".env") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".json", ".yaml", ".yml":
		return true
	}
	return false
}

// ParseFile reads a structured file and extracts all key-value pairs.
// Supports .env, .json, .yaml/.yml formats.
func ParseFile(path string) ([]ParsedField, error) {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))

	// .env files have no extension but start with ".env"
	if strings.HasPrefix(base, ".env") {
		return parseEnvFile(path)
	}

	switch ext {
	case ".json":
		return parseJSONFile(path)
	case ".yaml", ".yml":
		return parseYAMLFile(path)
	default:
		return nil, fmt.Errorf("unsupported format: %s", ext)
	}
}

func parseEnvFile(path string) ([]ParsedField, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var fields []ParsedField
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip "export " prefix
		line = strings.TrimPrefix(line, "export ")

		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if key != "" {
			fields = append(fields, ParsedField{Key: key, Value: val})
		}
	}
	return fields, s.Err()
}

func parseJSONFile(path string) ([]ParsedField, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// UseNumber keeps numeric literals exact (json.Number), so a value like
	// 1234567890123456789 isn't mangled to 1.2345678901234568e+18 — which would
	// both inject the wrong value and break substring redaction of the real one.
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	var fields []ParsedField
	flattenValue("", raw, &fields)

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Key < fields[j].Key
	})
	return fields, nil
}

func parseYAMLFile(path string) ([]ParsedField, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var fields []ParsedField
	flattenValue("", raw, &fields)

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Key < fields[j].Key
	})
	return fields, nil
}

func flattenValue(prefix string, v any, fields *[]ParsedField) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenValue(key, child, fields)
		}
	case []any:
		for i, child := range val {
			key := fmt.Sprintf("%s[%d]", prefix, i)
			flattenValue(key, child, fields)
		}
	default:
		if prefix != "" {
			*fields = append(*fields, ParsedField{
				Key:   prefix,
				Value: fmt.Sprintf("%v", val),
			})
		}
	}
}

// MaskValue returns a display-safe version of a value.
// Short values are fully masked, longer ones show prefix...suffix.
func MaskValue(val string) string {
	if val == "" {
		return "(empty)"
	}
	if len(val) < 8 {
		return "****"
	}
	return val[:4] + "..." + val[len(val)-3:]
}
