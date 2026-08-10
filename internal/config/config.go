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
	defaultHistoryMaxMessages         = 5000
	maxTCPPort                        = 65535
	envTelegramProxyAddr              = "TELEGRAM_PROXY_ADDR"
)

type Config struct {
	VaultRoot                  string
	ManifestPath               string
	HTTPBind                   string
	ControlPlaneToken          string
	TLSCertFile                string
	TLSKeyFile                 string
	TelegramAPIID              string
	TelegramAPIHash            string
	TelegramChatID             string
	RuntimeProfile             string
	TelegramSourceMode         string
	TelegramSessionPath        string
	TelegramProxyAddr          string
	RunMode                    string
	BatchMode                  string
	ONNXRuntimeSharedLibrary   string
	ONNXInputName              string
	ONNXOutputName             string
	EmbedModelID               string
	EmbedModelPath             string
	EmbedModelProfile          string
	DedupSimilarityThreshold   float64
	ClusterSimilarityThreshold float64
	HTTPPort                   int
	BatchSize                  int
	EmbedVectorDimension       int
	HistoryMaxMessages         int
	AllowInsecureBind          bool
	TelegramIncludeAllMessages bool
}

func Load() (Config, error) {
	vaultRoot := getOrDefault("VAULT_ROOT", defaultVaultRoot)

	manifestPath := strings.TrimSpace(os.Getenv("MANIFEST_PATH"))
	if manifestPath == "" {
		manifestPath = filepath.Join(vaultRoot, "_meta", "manifest.json")
	}

	httpCfg, err := loadHTTPConfig()
	if err != nil {
		return Config{}, err
	}

	telegramCfg, err := loadTelegramConfig(vaultRoot)
	if err != nil {
		return Config{}, err
	}

	runCfg, err := loadRunConfig()
	if err != nil {
		return Config{}, err
	}

	embedCfg, err := loadEmbedConfig()
	if err != nil {
		return Config{}, err
	}

	return Config{
		VaultRoot:                  vaultRoot,
		ManifestPath:               manifestPath,
		HTTPBind:                   httpCfg.bind,
		HTTPPort:                   httpCfg.port,
		AllowInsecureBind:          httpCfg.allowInsecureBind,
		ControlPlaneToken:          httpCfg.controlPlaneToken,
		TLSCertFile:                httpCfg.tlsCertFile,
		TLSKeyFile:                 httpCfg.tlsKeyFile,
		TelegramAPIID:              telegramCfg.apiID,
		TelegramAPIHash:            telegramCfg.apiHash,
		TelegramChatID:             telegramCfg.chatID,
		RuntimeProfile:             telegramCfg.runtimeProfile,
		TelegramSourceMode:         telegramCfg.sourceMode,
		TelegramSessionPath:        telegramCfg.sessionPath,
		TelegramProxyAddr:          telegramCfg.proxyAddr,
		HistoryMaxMessages:         telegramCfg.historyMaxMessages,
		TelegramIncludeAllMessages: telegramCfg.includeAllMessages,
		RunMode:                    runCfg.runMode,
		BatchMode:                  runCfg.batchMode,
		BatchSize:                  runCfg.batchSize,
		ONNXRuntimeSharedLibrary:   embedCfg.onnxRuntimeSharedLibrary,
		ONNXInputName:              embedCfg.onnxInputName,
		ONNXOutputName:             embedCfg.onnxOutputName,
		EmbedModelID:               embedCfg.modelID,
		EmbedModelPath:             embedCfg.modelPath,
		EmbedModelProfile:          embedCfg.modelProfile,
		EmbedVectorDimension:       embedCfg.vectorDimension,
		DedupSimilarityThreshold:   embedCfg.dedupSimilarityThreshold,
		ClusterSimilarityThreshold: embedCfg.clusterSimilarityThreshold,
	}, nil
}

type httpConfig struct {
	bind              string
	controlPlaneToken string
	tlsCertFile       string
	tlsKeyFile        string
	port              int
	allowInsecureBind bool
}

func loadHTTPConfig() (httpConfig, error) {
	httpBind := getOrDefault("HTTP_BIND", defaultHTTPBind)

	httpPort, err := intEnv("HTTP_PORT", defaultHTTPPort)
	if err != nil {
		return httpConfig{}, err
	}

	err = validatePort("HTTP_PORT", httpPort)
	if err != nil {
		return httpConfig{}, err
	}

	allowInsecureBind := boolEnv("ALLOW_INSECURE_BIND", false)

	err = validateHTTPBind("HTTP_BIND", httpBind, allowInsecureBind)
	if err != nil {
		return httpConfig{}, err
	}

	controlPlaneToken := strings.TrimSpace(os.Getenv("CONTROL_PLANE_TOKEN"))
	if allowInsecureBind && controlPlaneToken == "" {
		return httpConfig{}, ErrControlPlaneTokenRequired
	}

	if controlPlaneToken != "" && len(controlPlaneToken) < 16 {
		return httpConfig{}, ErrControlPlaneTokenTooShort
	}

	tlsCertFile := strings.TrimSpace(os.Getenv("CONTROL_PLANE_TLS_CERT_FILE"))
	tlsKeyFile := strings.TrimSpace(os.Getenv("CONTROL_PLANE_TLS_KEY_FILE"))

	err = validateControlPlaneTLS(allowInsecureBind, tlsCertFile, tlsKeyFile)
	if err != nil {
		return httpConfig{}, err
	}

	return httpConfig{
		bind:              httpBind,
		port:              httpPort,
		allowInsecureBind: allowInsecureBind,
		controlPlaneToken: controlPlaneToken,
		tlsCertFile:       tlsCertFile,
		tlsKeyFile:        tlsKeyFile,
	}, nil
}

func validateControlPlaneTLS(allowInsecureBind bool, certFile, keyFile string) error {
	certSet := certFile != ""
	keySet := keyFile != ""

	switch {
	case allowInsecureBind && (!certSet || !keySet):
		return ErrControlPlaneTLSRequired
	case certSet != keySet:
		return ErrControlPlaneTLSPairIncomplete
	case !certSet:
		return nil
	}

	err := requireRegularFile(certFile, ErrControlPlaneTLSCertMissing)
	if err != nil {
		return err
	}

	return requireRegularFile(keyFile, ErrControlPlaneTLSKeyMissing)
}

func requireRegularFile(path string, missing error) error {
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("%w: %q", missing, path)
	}

	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("%w: %q", missing, cleaned)
	}

	if info.IsDir() {
		return fmt.Errorf("%w: %q is a directory", missing, cleaned)
	}

	return nil
}

type telegramConfig struct {
	apiID, apiHash, chatID     string
	runtimeProfile, sourceMode string
	sessionPath, proxyAddr     string
	historyMaxMessages         int
	includeAllMessages         bool
}

func loadTelegramConfig(vaultRoot string) (telegramConfig, error) {
	telegramAPIID, err := requiredEnv("TELEGRAM_API_ID")
	if err != nil {
		return telegramConfig{}, err
	}

	telegramAPIHash, err := requiredEnv("TELEGRAM_API_HASH")
	if err != nil {
		return telegramConfig{}, err
	}

	telegramChatID, err := requiredEnv("TELEGRAM_CHAT_ID")
	if err != nil {
		return telegramConfig{}, err
	}

	runtimeProfile := getOrDefault("RUNTIME_PROFILE", defaultRuntimeProfile)

	err = validateEnum("RUNTIME_PROFILE", runtimeProfile, defaultRuntimeProfile)
	if err != nil {
		return telegramConfig{}, err
	}

	telegramSourceMode := getOrDefault("TELEGRAM_SOURCE_MODE", defaultTelegramSourceMode)

	err = validateEnum(
		"TELEGRAM_SOURCE_MODE",
		telegramSourceMode,
		defaultTelegramSourceMode,
	)
	if err != nil {
		return telegramConfig{}, err
	}

	err = validateRuntimeProfileSourceMode(runtimeProfile, telegramSourceMode)
	if err != nil {
		return telegramConfig{}, err
	}

	telegramSessionPath := strings.TrimSpace(os.Getenv("TELEGRAM_SESSION_PATH"))
	if telegramSessionPath == "" {
		telegramSessionPath = filepath.Join(vaultRoot, "_meta", "telegram.session.json")
	}

	telegramProxyAddr := strings.TrimSpace(os.Getenv("TELEGRAM_PROXY_ADDR"))

	err = validateProxyAddr(telegramProxyAddr)
	if err != nil {
		return telegramConfig{}, err
	}

	historyMaxMessages, err := intEnv("TELEGRAM_HISTORY_MAX_MESSAGES", defaultHistoryMaxMessages)
	if err != nil {
		return telegramConfig{}, err
	}

	return telegramConfig{
		apiID:              telegramAPIID,
		apiHash:            telegramAPIHash,
		chatID:             telegramChatID,
		runtimeProfile:     runtimeProfile,
		sourceMode:         telegramSourceMode,
		sessionPath:        telegramSessionPath,
		proxyAddr:          telegramProxyAddr,
		historyMaxMessages: historyMaxMessages,
		includeAllMessages: boolEnv("TELEGRAM_INCLUDE_ALL_MESSAGES", false),
	}, nil
}

type runConfig struct {
	runMode, batchMode string
	batchSize          int
}

func loadRunConfig() (runConfig, error) {
	runMode := getOrDefault("RUN_MODE", defaultRunMode)

	err := validateEnum("RUN_MODE", runMode, defaultRunMode, "full_rebuild")
	if err != nil {
		return runConfig{}, err
	}

	batchMode := getOrDefault("BATCH_MODE", defaultBatchMode)

	err = validateEnum("BATCH_MODE", batchMode, defaultBatchMode, "post_scan")
	if err != nil {
		return runConfig{}, err
	}

	batchSize, err := intEnv("BATCH_SIZE", defaultBatchSize)
	if err != nil {
		return runConfig{}, err
	}

	return runConfig{
		runMode:   runMode,
		batchMode: batchMode,
		batchSize: batchSize,
	}, nil
}

type embedConfig struct {
	onnxRuntimeSharedLibrary         string
	onnxInputName, onnxOutputName    string
	modelID, modelPath, modelProfile string
	dedupSimilarityThreshold         float64
	clusterSimilarityThreshold       float64
	vectorDimension                  int
}

func loadEmbedConfig() (embedConfig, error) {
	onnxRuntimeSharedLibrary := strings.TrimSpace(os.Getenv("ONNXRUNTIME_SHARED_LIBRARY"))
	onnxInputName := strings.TrimSpace(os.Getenv("ONNX_INPUT_NAME"))
	onnxOutputName := strings.TrimSpace(os.Getenv("ONNX_OUTPUT_NAME"))
	embedModelID := getOrDefault("EMBED_MODEL_ID", defaultEmbedModelID)
	embedModelPath := getOrDefault("EMBED_MODEL_PATH", defaultEmbedModelPath)

	embedModelProfile := getOrDefault("EMBED_MODEL_PROFILE", defaultEmbedModelProfile)

	err := validateEmbedModelProfile(embedModelProfile)
	if err != nil {
		return embedConfig{}, fmt.Errorf("EMBED_MODEL_PROFILE: %w", err)
	}

	embedVectorDimension, err := intEnv("EMBED_VECTOR_DIMENSION", defaultEmbedVectorDimension)
	if err != nil {
		return embedConfig{}, err
	}

	dedupSimilarityThreshold, err := floatEnv(
		"DEDUP_SIMILARITY_THRESHOLD",
		defaultDedupSimilarityThreshold,
	)
	if err != nil {
		return embedConfig{}, err
	}

	err = validateThreshold(
		"DEDUP_SIMILARITY_THRESHOLD",
		dedupSimilarityThreshold,
	)
	if err != nil {
		return embedConfig{}, err
	}

	clusterSimilarityThreshold, err := floatEnv(
		"CLUSTER_SIMILARITY_THRESHOLD",
		defaultClusterSimilarityThreshold,
	)
	if err != nil {
		return embedConfig{}, err
	}

	err = validateThreshold(
		"CLUSTER_SIMILARITY_THRESHOLD",
		clusterSimilarityThreshold,
	)
	if err != nil {
		return embedConfig{}, err
	}

	return embedConfig{
		onnxRuntimeSharedLibrary:   onnxRuntimeSharedLibrary,
		onnxInputName:              onnxInputName,
		onnxOutputName:             onnxOutputName,
		modelID:                    embedModelID,
		modelPath:                  embedModelPath,
		modelProfile:               embedModelProfile,
		vectorDimension:            embedVectorDimension,
		dedupSimilarityThreshold:   dedupSimilarityThreshold,
		clusterSimilarityThreshold: clusterSimilarityThreshold,
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
		return "", fmt.Errorf("%s: %w", key, ErrRequiredEnvNotSet)
	}

	return value, nil
}

func validateEnum(key, value string, allowed ...string) error {
	if slices.Contains(allowed, value) {
		return nil
	}

	return fmt.Errorf(
		"%s: %w %q (allowed: %s)",
		key,
		ErrInvalidEnumValue,
		value,
		strings.Join(allowed, ", "),
	)
}

func intEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%s: %w, got %d", key, ErrMustBeGreaterThanZero, parsed)
	}

	return parsed, nil
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
		return fmt.Errorf("%s: %w, got %v", key, ErrMustBeUnitInterval, value)
	}

	return nil
}

func validatePort(key string, value int) error {
	if value > maxTCPPort {
		return fmt.Errorf("%s: %w 1..%d, got %d", key, ErrPortOutOfRange, maxTCPPort, value)
	}

	return nil
}

func validateRuntimeProfileSourceMode(runtimeProfile, telegramSourceMode string) error {
	if runtimeProfile != defaultRuntimeProfile {
		return fmt.Errorf("%w %q", ErrInvalidRuntimeProfile, runtimeProfile)
	}

	if telegramSourceMode != defaultTelegramSourceMode {
		return fmt.Errorf("%w %q", ErrInvalidTelegramSourceMode, telegramSourceMode)
	}

	return nil
}

func validateEmbedModelProfile(profile string) error {
	trimmed := strings.TrimSpace(profile)
	switch trimmed {
	case "bert_tokenized_mean_pooling", "string_input_direct":
		return nil
	case "":
		return ErrModelProfileEmpty
	default:
		return fmt.Errorf(
			"%w %q (allowed: bert_tokenized_mean_pooling, string_input_direct)",
			ErrUnsupportedModelProfile,
			trimmed,
		)
	}
}

func validateProxyAddr(value string) error {
	if value == "" {
		return nil
	}

	host, portRaw, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("%s: %w, got %q", envTelegramProxyAddr, ErrHostPortFormat, value)
	}

	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("%s: %w", envTelegramProxyAddr, ErrHostEmpty)
	}

	port, err := strconv.Atoi(strings.TrimSpace(portRaw))
	if err != nil {
		return fmt.Errorf("%s: %w in %q", envTelegramProxyAddr, ErrInvalidPort, value)
	}

	if port <= 0 || port > maxTCPPort {
		return fmt.Errorf(
			"%s: port %w 1..%d, got %d",
			envTelegramProxyAddr,
			ErrPortOutOfRange,
			maxTCPPort,
			port,
		)
	}

	return nil
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func validateHTTPBind(key, value string, allowInsecure bool) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s: %w", key, ErrMustNotBeEmpty)
	}

	if allowInsecure {
		return nil
	}

	ip := net.ParseIP(trimmed)
	if ip == nil {
		if trimmed == "localhost" {
			return nil
		}

		return fmt.Errorf("%s: %w: %q", key, ErrNonLoopbackBind, trimmed)
	}

	if !ip.IsLoopback() {
		return fmt.Errorf("%s: %w: %q", key, ErrNonLoopbackBind, trimmed)
	}

	return nil
}
