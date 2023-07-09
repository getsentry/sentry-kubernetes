package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog"
	globalLogger "github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
)

func configureLogging() {
	levelMap := map[string]zerolog.Level{
		"trace":    zerolog.TraceLevel,
		"debug":    zerolog.DebugLevel,
		"info":     zerolog.InfoLevel,
		"warn":     zerolog.WarnLevel,
		"error":    zerolog.ErrorLevel,
		"fatal":    zerolog.FatalLevel,
		"panic":    zerolog.PanicLevel,
		"disabled": zerolog.Disabled,
	}
	logLevelRaw := strings.ToLower(os.Getenv("SENTRY_K8S_LOG_LEVEL"))

	var logLevel zerolog.Level
	logLevel, ok := levelMap[logLevelRaw]
	if !ok {
		logLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(logLevel)
	globalLogger.Logger = globalLogger.Output(zerolog.ConsoleWriter{Out: os.Stdout})
}

func main() {
	configureLogging()
	initSentrySDK()
	defer sentry.Flush(time.Second)
	checkCommonEnhancerPatterns()
	prepareEventFilters()

	config, err := getClusterConfig()
	if err != nil {
		globalLogger.Fatal().Msgf("Config init error: %s", err)
	}

	setKubernetesSentryContext(config)
	setGlobalSentryTags()
	runIntegrations()

	watchAllNamespaces, namespaces, err := getNamespacesToWatch()
	if err != nil {
		globalLogger.Fatal().Msgf("Cannot parse namespaces to watch: %s", err)
	}

	if watchAllNamespaces {
		namespaces = []string{v1.NamespaceAll}
	}

	ctx := globalLogger.Logger.WithContext(context.Background())
	startEventWatchers(ctx, config, namespaces)
	startPodWatchers(ctx, config, namespaces)

	// Sleep forever
	select {}
}
