package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Environment variable names. Every knob this server has is read here and
// nowhere else, so the whole configuration surface is greppable in one file.
const (
	// EnvPort sets the HTTP listen port.
	EnvPort = "STUDY_PORT"

	// EnvHome names the directory that holds the .env file and the data tree.
	//
	// It is read from the process environment only, never from the .env file: it
	// decides which file is read, so it cannot come from inside it. Empty means
	// the working directory, which is what a checked-out repository uses; a binary
	// installed elsewhere sets it so its paths do not follow the shell.
	EnvHome = "STUDY_HOME"

	// EnvEnvFile names the configuration file explicitly, and has the same
	// process-environment-only rule as EnvHome. It defaults to <EnvHome>/.env.
	EnvEnvFile = "STUDY_ENV_FILE"

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

	// EnvEmbeddingModel selects the model the semantic stage embeds with. It is a
	// separate knob from EnvLLMModel because the two model roles are unrelated:
	// one transcribes a page, the other turns text into a vector.
	EnvEmbeddingModel = "STUDY_EMBEDDING_MODEL"

	// EnvOpenRouterBaseURL overrides the API root. It is the only way to point a
	// run at a proxy, a gateway, or a test double.
	EnvOpenRouterBaseURL = "STUDY_OPENROUTER_BASE_URL"
)

// DotEnvFile is the file Load reads before falling back to the process
// environment. It is optional.
const DotEnvFile = ".env"

// Defaults, used when the environment does not override them.
const (
	DefaultPort = 8080

	// DefaultMaxUploadBytes bounds a single artifact at 100 MiB. The largest real
	// book on hand is 14 MiB, so this leaves room while still bounding disk use.
	DefaultMaxUploadBytes = 100 << 20

	// DefaultLLMModel is the vision-capable model verified against the live
	// models API: it is present, and its input_modalities include image.
	DefaultLLMModel = "qwen/qwen3.5-flash-02-23"

	// DefaultEmbeddingModel is the multilingual embedding model chosen from the
	// measured ranking in the step-5 log (L4), not from a name: eight candidates
	// were scored on 16 Spanish queries against the real English corpus, and this
	// one won on every aggregate (@1, @5, MRR). It is 3072-dimensional.
	DefaultEmbeddingModel = "google/gemini-embedding-2"

	// DefaultOpenRouterBaseURL is the public OpenRouter API root.
	DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

// DefaultDatabaseDSN is the database file a run uses when nothing overrides it.
//
// It lives under the root, and the root is the working directory unless STUDY_HOME
// says otherwise, so a checked-out repository keeps writing to its own data/
// directory while a binary run from anywhere else writes to the root it was given.
func DefaultDatabaseDSN(root string) string {
	return "file:" + filepath.Join(root, "data", "study.db")
}

// DefaultStorageRoot is the artifact store a run uses when nothing overrides it.
func DefaultStorageRoot(root string) string {
	return filepath.Join(root, "data", "artifacts")
}

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
	// Root is the directory the run reads .env from and writes data/ under. It is
	// always absolute, so it can be printed and used as is. It is the working
	// directory unless STUDY_HOME (or the -home flag) says otherwise.
	Root string

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

	// EmbeddingModel is the model identifier sent with every embedding request.
	// Its slug is stored beside every vector, so a change is detectable rather
	// than silently compared against the old model's vectors.
	EmbeddingModel string

	// OpenRouterBaseURL is the API root. It has an http or https scheme and a
	// host, both checked at load time so a typo aborts the run instead of
	// failing at page 200.
	OpenRouterBaseURL string
}

// Options selects where a run reads its configuration from.
//
// The zero value is the layout the repository uses: the working directory holds
// .env and data/. A non-empty Home moves both, which is what an installed binary
// needs so its paths do not follow whichever directory the shell happens to be in.
type Options struct {
	// Home overrides STUDY_HOME.
	Home string

	// EnvFile overrides STUDY_ENV_FILE. A relative path is resolved against the
	// working directory, not against Home: it is what the operator typed.
	EnvFile string
}

// Load resolves configuration for the repository layout.
//
// The .env file under the root is read first and applied only for variables the
// process environment has not already set, so a real environment always wins over
// the file. A missing file is not an error.
func Load() (*Config, error) {
	return LoadWith(Options{})
}

// EnvLookup reports the value of a variable and whether it was set at all.
// It matches os.LookupEnv.
type EnvLookup func(key string) (string, bool)

// LoadWith resolves configuration from options, the process environment, and a
// .env file.
func LoadWith(options Options) (*Config, error) {
	return load(options, os.LookupEnv)
}

// LoadWithFrom resolves configuration from options and an injected environment
// source. Tests need it because the selectors must be observable without touching
// the process environment, and because an environment variable set to the empty
// string deliberately beats the file.
func LoadWithFrom(options Options, lookup EnvLookup) (*Config, error) {
	return load(options, lookup)
}

// LoadFrom resolves configuration using path as the .env file and lookup as the
// environment source. Passing a fake lookup keeps callers and tests fully
// hermetic: nothing here touches global state through this entry point.
//
// The root stays the working directory, so a test that only cares about the file's
// contents gets absolute defaults it can still predict.
func LoadFrom(path string, lookup EnvLookup) (*Config, error) {
	return load(Options{EnvFile: path}, lookup)
}

// resolveRoot picks the directory a run reads .env from and writes data/ under:
// the explicit option, then STUDY_HOME, then the working directory. Both selector
// variables are read from the process environment only, because they decide which
// file is read and cannot come from inside it.
func resolveRoot(explicit string, lookup EnvLookup) (string, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		if value, ok := lookup(EnvHome); ok {
			raw = strings.TrimSpace(value)
		}
	}
	if raw == "" {
		raw = "."
	}

	root, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %q is not a usable directory: %w", EnvHome, raw, err)
	}

	// The default database DSN is "file:" followed by this path, so a '?' or '#' in
	// the root would be read by SQLite as the start of the query string instead of as
	// part of the path: the database would open somewhere the caller did not name.
	// Refusing here makes the cause visible instead of leaving a mystery.
	if strings.ContainsAny(root, "?#") {
		return "", fmt.Errorf("%s: %q cannot be a root: '?' and '#' would be read as DSN parameters",
			EnvHome, root)
	}

	return root, nil
}

// resolveEnvFile picks the configuration file: the explicit option, then
// STUDY_ENV_FILE, then <root>/.env. An explicit relative path is resolved against
// the working directory, because that is what the operator typed.
func resolveEnvFile(explicit, root string, lookup EnvLookup) (string, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		if value, ok := lookup(EnvEnvFile); ok {
			raw = strings.TrimSpace(value)
		}
	}
	if raw == "" {
		return filepath.Join(root, DotEnvFile), nil
	}

	path, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %q is not a usable path: %w", EnvEnvFile, raw, err)
	}
	return path, nil
}

// RootIsTheWorkingDirectory reports whether Root is where the process was started.
// That is the repository layout, the one the previous behaviour covered, so it is
// the case that needs nothing printed about it.
func (c *Config) RootIsTheWorkingDirectory() bool {
	workingDirectory, err := os.Getwd()
	return err == nil && c.Root == workingDirectory
}

func load(options Options, lookup EnvLookup) (*Config, error) {
	root, err := resolveRoot(options.Home, lookup)
	if err != nil {
		return nil, err
	}

	envFile, err := resolveEnvFile(options.EnvFile, root, lookup)
	if err != nil {
		return nil, err
	}

	fileVars, err := readDotEnv(envFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", envFile, err)
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
		dsn = DefaultDatabaseDSN(root)
	}

	storageRoot := strings.TrimSpace(resolve(EnvStorageRoot))
	if storageRoot == "" {
		storageRoot = DefaultStorageRoot(root)
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

	embeddingModel := strings.TrimSpace(resolve(EnvEmbeddingModel))
	if embeddingModel == "" {
		embeddingModel = DefaultEmbeddingModel
	}

	baseURL, err := ParseOpenRouterBaseURL(resolve(EnvOpenRouterBaseURL))
	if err != nil {
		return nil, err
	}

	return &Config{
		Root:               root,
		Port:               port,
		DatabaseDSN:        dsn,
		StorageRoot:        storageRoot,
		MaxUploadBytes:     maxUploadBytes,
		CORSAllowedOrigins: origins,
		OpenRouterAPIKey:   apiKey,
		LLMModel:           model,
		EmbeddingModel:     embeddingModel,
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
