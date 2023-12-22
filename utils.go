package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/rs/zerolog"
	globalLogger "github.com/rs/zerolog/log"
)

var truthyStrings = map[string]struct{}{
	"yes":  {},
	"true": {},
	"1":    {},
}

func isTruthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	_, found := truthyStrings[s]
	return found
}

func prettyJSON(obj any) (string, error) {
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
	return getLoggerWithTags(ctx, map[string]string{key: value})
}

func getLoggerWithTags(ctx context.Context, tags map[string]string) (context.Context, *zerolog.Logger) {
	logger := zerolog.Ctx(ctx)
	if logger == nil ||
		logger == zerolog.DefaultContextLogger ||
		logger.GetLevel() == zerolog.Disabled {
		// Use the global logger if nothing was set on the context
		logger = &globalLogger.Logger
	}

	loggerContext := logger.With()
	for key, value := range tags {
		if key != "" {
			loggerContext = loggerContext.Str(key, value)
		}
	}

	extendedLogger := loggerContext.Logger()
	ctx = extendedLogger.WithContext(ctx)
	return ctx, &extendedLogger
}
