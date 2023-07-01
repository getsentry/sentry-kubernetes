package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/rs/zerolog"
)

var truthyStrings map[string]struct{} = map[string]struct{}{
	"yes":  {},
	"true": {},
	"1":    {},
}

func isTruthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
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

func removeDuplicates(slice []string) []string {
	res := make([]string, 0, len(slice))
	seen := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		if _, found := seen[s]; !found {
			res = append(res, s)
		}
	}
	return res
}

func getLoggerWithTag(ctx context.Context, key string, value string) (context.Context, *zerolog.Logger) {
	logger := (zerolog.Ctx(ctx).With().
		Str(key, value).
		Logger())
	ctx = logger.WithContext(ctx)
	return ctx, &logger
}
