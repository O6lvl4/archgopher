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
		plainFloat(n)
	}
	for _, c := range n.Content {
		plainFloats(c)
	}
}

// plainFloat writes a float scalar without an exponent, and as an int when it
// is whole, if its size makes that readable.
func plainFloat(n *yaml.Node) {
	v, err := strconv.ParseFloat(n.Value, 64)
	if err != nil || !readablePlain(v) {
		return
	}
	n.Value = strconv.FormatFloat(v, 'f', -1, 64)
	if v == math.Trunc(v) {
		n.Tag = "!!int"
	}
}

// readablePlain says whether v written out in full stays short: below 1e15
// and, unless zero, not below 1e-6.
func readablePlain(v float64) bool {
	return math.Abs(v) < 1e15 && (v == 0 || math.Abs(v) >= 1e-6)
}
