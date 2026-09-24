package model

import (
	"bytes"
	"fmt"
	"math"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ParseSpec reads a declaration, rejecting unknown top-level and node keys.
func ParseSpec(data []byte) (Spec, error) {
	var s Spec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return Spec{}, fmt.Errorf("parse spec: %w", err)
	}
	if s.Region == "" {
		return Spec{}, fmt.Errorf("parse spec: region is required")
	}
	return s, nil
}

// MarshalSpec writes a declaration as YAML, with numbers in plain decimal.
func MarshalSpec(s Spec) ([]byte, error) {
	var root yaml.Node
	if err := root.Encode(s); err != nil {
		return nil, err
	}
	plainFloats(&root)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

// plainFloats rewrites 3e+07 as 30000000 so declarations stay readable.
func plainFloats(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!float" {
		if v, err := strconv.ParseFloat(n.Value, 64); err == nil && math.Abs(v) < 1e15 && (v == 0 || math.Abs(v) >= 1e-6) {
			n.Value = strconv.FormatFloat(v, 'f', -1, 64)
			if v == math.Trunc(v) {
				n.Tag = "!!int"
			}
		}
	}
	for _, c := range n.Content {
		plainFloats(c)
	}
}
