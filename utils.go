package main

import (
	"encoding/json"
	"strings"
)

var truthyStrings map[string]struct{} = map[string]struct{}{
	"yes":  struct{}{},
	"true": struct{}{},
	"1":    struct{}{},
}

func isTruthy(s string) bool {
	s = strings.ToLower(s)
	_, found := truthyStrings[s]
	return found
}

func prettyJson(obj any) (string, error) {
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
