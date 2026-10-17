package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Environment variable names. Every knob this server has is read here and
// nowhere else, so the whole configuration surface is greppable in one file.
const (
	// EnvPort sets the HTTP listen port.
	EnvPort = "STUDY_PORT"

	// EnvDatabaseDSN sets the SQLite data source name.
	EnvDatabaseDSN = "STUDY_DB_DSN"

	// EnvStorageRoot sets the directory holding stored artifacts.
	EnvStorageRoot = "STUDY_STORAGE_ROOT"

	// EnvMaxUploadBytes sets the largest single artifact the store accepts.
	EnvMaxUploadBytes = "STUDY_MAX_UPLOAD_BYTES"

	// EnvCORSOrigins sets the comma-separated list of origins allowed to read
	// API responses. The literal "*" restores a permissive wildcard policy.
	EnvCORSOrigins = "STUDY_CORS_ORIGINS"

	// EnvOpenRouterAPIKey holds the bearer token the ingestion pipeline uses.
	//
	// It is read here but required only by the ingestion command, not by Load:
	// the API server never transcribes anything, and requiring the key here would
	// stop the server booting for a credential it does not use. The command fails
	// before page 1 when the key is missing.
	EnvOpenRouterAPIKey = "STUDY_OPENROUTER_API_KEY"

	// EnvLLMModel selects the model the ingestion pipeline transcribes with.
	EnvLLMModel = "STUDY_LLM_MODEL"

	// EnvOpenRouterBaseURL overrides the API root. It is the only way to point a
	// run at a proxy, a gateway, or a test double.
	EnvOpenRouterBaseURL = "STUDY_OPENROUTER_BASE_URL"
)

// DotEnvFile is the file Load reads before falling back to the process
// environment. It is optional.
const DotEnvFile = ".env"

// Defaults, used when the environment does not override them.
const (
	DefaultPort        = 8080
	DefaultDatabaseDSN = "file:data/study.db"

	// DefaultStorageRoot is relative to the working directory, like the database.
	DefaultStorageRoot = "data/artifacts"

	// DefaultMaxUploadBytes bounds a single artifact at 100 MiB. The largest real
	// book on hand is 14 MiB, so this leaves room while still bounding disk use.
	DefaultMaxUploadBytes = 100 << 20

	// DefaultLLMModel is the vision-capable model verified against the live
	// models API: it is present, and its input_modalities include image.
	DefaultLLMModel = "qwen/qwen3.5-flash-02-23"

	// DefaultOpenRouterBaseURL is the public OpenRouter API root.
	DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

// DefaultCORSOrigins covers the clients this API is built to serve:
//
//   - Obsidian desktop renders the plugin from an app:// origin.
//   - Obsidian mobile (Capacitor) uses capacitor:// on iOS and http://localhost
//     on Android.
//   - The browser dev harness (dev/index.html via Vite) runs on localhost.
//
// These have NOT been verified against a real device. If a legitimate client is
// rejected, set EnvCORSOrigins (see the Configuration section of the README)
// rather than widening the default.
var DefaultCORSOrigins = []string{
	"app://obsidian.md",
	"capacitor://localhost",
	"http://localhost",
	"http://localhost:5173",
	"http://127.0.0.1",
	"http://127.0.0.1:5173",
}

// Config is the fully resolved, validated configuration for the server.
type Config struct {
	// Port is the HTTP listen port, 1-65535.
	Port int

	// DatabaseDSN is the SQLite data source name, non-empty.
	DatabaseDSN string

	// StorageRoot is the directory holding stored artifacts, non-empty.
	StorageRoot string

	// MaxUploadBytes is the largest single artifact accepted, positive.
	MaxUploadBytes int64

	// CORSAllowedOrigins is the origin allowlist. It contains at least one
	// entry, and a single "*" entry means permissive.
	CORSAllowedOrigins []string

	// OpenRouterAPIKey is the bearer token for the ingestion model. Empty is a
	// valid configuration: only the ingestion command requires it.
	OpenRouterAPIKey string

	// LLMModel is the model identifier sent with every transcription request.
	LLMModel string

	// OpenRouterBaseURL is the API root. It has an http or https scheme and a
	// host, both checked at load time so a typo aborts the run instead of
	// failing at page 200.
	OpenRouterBaseURL string
}

// Load resolves configuration from the process environment.
//
// A .env file in the current directory is read first and applied only for
// variables the process environment has not already set, so a real environment
// always wins over the file. A missing file is not an error.
func Load() (*Config, error) {
	return load(DotEnvFile, os.LookupEnv)
}

// EnvLookup reports the value of a variable and whether it was set at all.
// It matches os.LookupEnv.
type EnvLookup func(key string) (string, bool)

// LoadFrom resolves configuration using path as the .env file and lookup as the
// environment source. Passing a fake lookup keeps callers and tests fully
// hermetic: nothing here touches global state through this entry point.
func LoadFrom(path string, lookup EnvLookup) (*Config, error) {
	return load(path, lookup)
}

func load(dotEnvPath string, lookup EnvLookup) (*Config, error) {
	fileVars, err := readDotEnv(dotEnvPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dotEnvPath, err)
	}

	// The environment wins: only fall back to the file for variables the
	// process did not already set.
	resolve := func(key string) string {
		if value, ok := lookup(key); ok {
			return value
		}
		return fileVars[key]
	}

	port, err := ParsePort(resolve(EnvPort))
	if err != nil {
		return nil, fmt.Errorf("port: %w", err)
	}

	dsn := strings.TrimSpace(resolve(EnvDatabaseDSN))
	if dsn == "" {
		dsn = DefaultDatabaseDSN
	}

	storageRoot := strings.TrimSpace(resolve(EnvStorageRoot))
	if storageRoot == "" {
		storageRoot = DefaultStorageRoot
	}

	maxUploadBytes, err := ParseMaxUploadBytes(resolve(EnvMaxUploadBytes))
	if err != nil {
		return nil, fmt.Errorf("max upload bytes: %w", err)
	}

	origins, err := resolveOrigins(resolve(EnvCORSOrigins))
	if err != nil {
		return nil, fmt.Errorf("cors origins: %w", err)
	}

	// The key stays optional here; the ingestion command is what requires it.
	apiKey := strings.TrimSpace(resolve(EnvOpenRouterAPIKey))

	model := strings.TrimSpace(resolve(EnvLLMModel))
	if model == "" {
		model = DefaultLLMModel
	}

	baseURL, err := ParseOpenRouterBaseURL(resolve(EnvOpenRouterBaseURL))
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:               port,
		DatabaseDSN:        dsn,
		StorageRoot:        storageRoot,
		MaxUploadBytes:     maxUploadBytes,
		CORSAllowedOrigins: origins,
		OpenRouterAPIKey:   apiKey,
		LLMModel:           model,
		OpenRouterBaseURL:  baseURL,
	}, nil
}

// ParseOpenRouterBaseURL validates the API root. An empty string yields
// DefaultOpenRouterBaseURL and trailing slashes are removed, so a caller never
// has to decide whether a path join needs one.
//
// The check is deliberately early: a malformed URL is otherwise discovered only
// when the first page is transcribed, which on a long book is hundreds of pages
// into a run. Every rejection names the variable, because that is the thing the
// user has to edit.
func ParseOpenRouterBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultOpenRouterBaseURL, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %q is not a URL", EnvOpenRouterBaseURL, raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s: %q must use the http or https scheme", EnvOpenRouterBaseURL, raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s: %q has no host", EnvOpenRouterBaseURL, raw)
	}

	return strings.TrimRight(raw, "/"), nil
}

// ParsePort validates and converts a port string. An empty string yields
// DefaultPort. It is exported so the command-line flag applies exactly the same
// rule as the environment variable, instead of duplicating the bounds check.
func ParsePort(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultPort, nil
	}

	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", EnvPort, raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s: %d is outside 1-65535", EnvPort, port)
	}
	return port, nil
}

// ParseMaxUploadBytes validates a byte-count string. An empty string yields
// DefaultMaxUploadBytes. It is exported so a command-line flag applies exactly
// the same rule as the environment variable, instead of duplicating it.
func ParseMaxUploadBytes(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultMaxUploadBytes, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", EnvMaxUploadBytes, raw)
	}
	if value < 1 {
		return 0, fmt.Errorf("%s: %d must be positive", EnvMaxUploadBytes, value)
	}
	return value, nil
}

func resolveOrigins(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), DefaultCORSOrigins...), nil
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	if len(origins) == 0 {
		return nil, fmt.Errorf("%s: no origins found; omit the variable for defaults, or use %q", EnvCORSOrigins, "*")
	}
	return origins, nil
}
