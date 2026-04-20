package config

import (
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	defaultVaultRoot                  = "./vault"
	defaultRuntimeProfile             = "real"
	defaultTelegramSourceMode         = "live"
	defaultHTTPBind                   = "127.0.0.1"
	defaultHTTPPort                   = 8080
	defaultRunMode                    = "incremental"
	defaultBatchMode                  = "streaming"
	defaultBatchSize                  = 32
	defaultEmbedModelID               = "all-MiniLM-L6-v2-go"
	defaultEmbedModelPath             = "./models/all-MiniLM-L6-v2.onnx"
	defaultEmbedModelProfile          = "bert_tokenized_mean_pooling"
	defaultEmbedVectorDimension       = 384
	defaultDedupSimilarityThreshold   = 0.95
	defaultClusterSimilarityThreshold = 0.80
)

type Config struct {
	VaultRoot, ManifestPath, HTTPBind, TelegramAPIID, TelegramAPIHash, TelegramChatID, RuntimeProfile, TelegramSourceMode, TelegramSessionPath, TelegramProxyAddr, RunMode, BatchMode, ONNXRuntimeSharedLibrary, ONNXInputName, ONNXOutputName, EmbedModelID, EmbedModelPath, EmbedModelProfile string
	HTTPPort, BatchSize, EmbedVectorDimension                                                                                                                                                                                                                                int
	DedupSimilarityThreshold, ClusterSimilarityThreshold                                                                                                                                                                                                                     float64
}

//nolint:gocyclo
func Load() (Config, error) {
	vaultRoot := getOrDefault("VAULT_ROOT", defaultVaultRoot)
	manifestPath := strings.TrimSpace(os.Getenv("MANIFEST_PATH"))
	if manifestPath == "" {
		manifestPath = filepath.Join(vaultRoot, "_meta", "manifest.json")
	}
	httpBind := getOrDefault("HTTP_BIND", defaultHTTPBind)
	httpPort, err := intEnv("HTTP_PORT", defaultHTTPPort)
	if err != nil {
		return Config{}, err
	}
	if err := validatePort("HTTP_PORT", httpPort); err != nil {
		return Config{}, err
	}
	telegramAPIID, err := requiredEnv("TELEGRAM_API_ID")
	if err != nil {
		return Config{}, err
	}
	telegramAPIHash, err := requiredEnv("TELEGRAM_API_HASH")
	if err != nil {
		return Config{}, err
	}
	telegramChatID, err := requiredEnv("TELEGRAM_CHAT_ID")
	if err != nil {
		return Config{}, err
	}
	runtimeProfile := getOrDefault("RUNTIME_PROFILE", defaultRuntimeProfile)
	if err := validateEnum("RUNTIME_PROFILE", runtimeProfile, defaultRuntimeProfile); err != nil {
		return Config{}, err
	}
	telegramSourceMode := getOrDefault("TELEGRAM_SOURCE_MODE", defaultTelegramSourceMode)
	if err := validateEnum("TELEGRAM_SOURCE_MODE", telegramSourceMode, defaultTelegramSourceMode); err != nil {
		return Config{}, err
	}
	if err := validateRuntimeProfileSourceMode(runtimeProfile, telegramSourceMode); err != nil {
		return Config{}, err
	}
	telegramSessionPath := strings.TrimSpace(os.Getenv("TELEGRAM_SESSION_PATH"))
	if telegramSessionPath == "" {
		telegramSessionPath = filepath.Join(vaultRoot, "_meta", "telegram.session.json")
	}
	telegramProxyAddr := strings.TrimSpace(os.Getenv("TELEGRAM_PROXY_ADDR"))
	if err := validateProxyAddr("TELEGRAM_PROXY_ADDR", telegramProxyAddr); err != nil {
		return Config{}, err
	}
	runMode := getOrDefault("RUN_MODE", defaultRunMode)
	if err := validateEnum("RUN_MODE", runMode, defaultRunMode, "full_rebuild"); err != nil {
		return Config{}, err
	}
	batchMode := getOrDefault("BATCH_MODE", defaultBatchMode)
	if err := validateEnum("BATCH_MODE", batchMode, defaultBatchMode, "post_scan"); err != nil {
		return Config{}, err
	}
	batchSize, err := intEnv("BATCH_SIZE", defaultBatchSize)
	if err != nil {
		return Config{}, err
	}
	onnxRuntimeSharedLibrary := strings.TrimSpace(os.Getenv("ONNXRUNTIME_SHARED_LIBRARY"))
	onnxInputName := strings.TrimSpace(os.Getenv("ONNX_INPUT_NAME"))
	onnxOutputName := strings.TrimSpace(os.Getenv("ONNX_OUTPUT_NAME"))
	embedModelID := getOrDefault("EMBED_MODEL_ID", defaultEmbedModelID)
	embedModelPath := getOrDefault("EMBED_MODEL_PATH", defaultEmbedModelPath)
	embedModelProfile := getOrDefault("EMBED_MODEL_PROFILE", defaultEmbedModelProfile)
	if err := validateEmbedModelProfile(embedModelProfile); err != nil {
		return Config{}, fmt.Errorf("EMBED_MODEL_PROFILE: %w", err)
	}
	embedVectorDimension, err := intEnv("EMBED_VECTOR_DIMENSION", defaultEmbedVectorDimension)
	if err != nil {
		return Config{}, err
	}
	dedupSimilarityThreshold, err := floatEnv("DEDUP_SIMILARITY_THRESHOLD", defaultDedupSimilarityThreshold)
	if err != nil {
		return Config{}, err
	}
	if err := validateThreshold("DEDUP_SIMILARITY_THRESHOLD", dedupSimilarityThreshold); err != nil {
		return Config{}, err
	}
	clusterSimilarityThreshold, err := floatEnv("CLUSTER_SIMILARITY_THRESHOLD", defaultClusterSimilarityThreshold)
	if err != nil {
		return Config{}, err
	}
	if err := validateThreshold("CLUSTER_SIMILARITY_THRESHOLD", clusterSimilarityThreshold); err != nil {
		return Config{}, err
	}

	return Config{
		VaultRoot:                  vaultRoot,
		ManifestPath:               manifestPath,
		HTTPBind:                   httpBind,
		HTTPPort:                   httpPort,
		TelegramAPIID:              telegramAPIID,
		TelegramAPIHash:            telegramAPIHash,
		TelegramChatID:             telegramChatID,
		RuntimeProfile:             runtimeProfile,
		TelegramSourceMode:         telegramSourceMode,
		TelegramSessionPath:        telegramSessionPath,
		TelegramProxyAddr:          telegramProxyAddr,
		RunMode:                    runMode,
		BatchMode:                  batchMode,
		BatchSize:                  batchSize,
		ONNXRuntimeSharedLibrary:   onnxRuntimeSharedLibrary,
		ONNXInputName:              onnxInputName,
		ONNXOutputName:             onnxOutputName,
		EmbedModelID:               embedModelID,
		EmbedModelPath:             embedModelPath,
		EmbedModelProfile:          embedModelProfile,
		EmbedVectorDimension:       embedVectorDimension,
		DedupSimilarityThreshold:   dedupSimilarityThreshold,
		ClusterSimilarityThreshold: clusterSimilarityThreshold,
	}, nil
}

func getOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s: required environment variable is not set", key)
	}
	return value, nil
}

func validateEnum(key, value string, allowed ...string) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	return fmt.Errorf("%s: invalid value %q (allowed: %s)", key, value, strings.Join(allowed, ", "))
}

func intEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s: must be greater than 0, got %d", key, n)
	}
	return n, nil
}

func floatEnv(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func validateThreshold(key string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return fmt.Errorf("%s: must be greater than 0 and at most 1, got %v", key, value)
	}
	return nil
}

func validatePort(key string, value int) error {
	if value > 65535 {
		return fmt.Errorf("%s: must be in range 1..65535, got %d", key, value)
	}
	return nil
}

func validateRuntimeProfileSourceMode(runtimeProfile, telegramSourceMode string) error {
	if runtimeProfile != defaultRuntimeProfile {
		return fmt.Errorf("RUNTIME_PROFILE: invalid value %q", runtimeProfile)
	}
	if telegramSourceMode != defaultTelegramSourceMode {
		return fmt.Errorf("TELEGRAM_SOURCE_MODE: invalid value %q", telegramSourceMode)
	}
	return nil
}

func validateEmbedModelProfile(profile string) error {
	trimmed := strings.TrimSpace(profile)
	switch trimmed {
	case "bert_tokenized_mean_pooling", "string_input_direct":
		return nil
	case "":
		return fmt.Errorf("model_profile: must not be empty")
	default:
		return fmt.Errorf("model_profile: unsupported value %q (allowed: bert_tokenized_mean_pooling, string_input_direct)", trimmed)
	}
}

func validateProxyAddr(key, value string) error {
	if value == "" {
		return nil
	}
	host, portRaw, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("%s: must be in host:port format, got %q", key, value)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("%s: host must not be empty", key)
	}
	port, err := strconv.Atoi(strings.TrimSpace(portRaw))
	if err != nil {
		return fmt.Errorf("%s: invalid port in %q", key, value)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s: port must be in range 1..65535, got %d", key, port)
	}
	return nil
}
