package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
)

// fakeEnv builds a hermetic environment lookup, so no test depends on or
// mutates the real process environment.
func fakeEnv(vars map[string]string) config.EnvLookup {
	return func(key string) (string, bool) {
		value, ok := vars[key]
		return value, ok
	}
}

func emptyEnv() config.EnvLookup {
	return fakeEnv(map[string]string{})
}

// writeDotEnv writes content to a temporary .env file and returns its path.
func writeDotEnv(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// --- Load: defaults ---

func TestLoad_DefaultsWhenNothingIsSet(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), emptyEnv())
	require.NoError(t, err)

	assert.Equal(t, config.DefaultPort, cfg.Port)
	assert.Equal(t, config.DefaultDatabaseDSN(cfg.Root), cfg.DatabaseDSN)
	assert.Equal(t, config.DefaultStorageRoot(cfg.Root), cfg.StorageRoot)
	assert.Equal(t, int64(config.DefaultMaxUploadBytes), cfg.MaxUploadBytes)
	assert.Equal(t, config.DefaultCORSOrigins, cfg.CORSAllowedOrigins)
}

func TestLoad_MissingDotEnvFileIsNotAnError(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "does-not-exist"), emptyEnv())

	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestLoad_ReturnsACopyOfDefaultOrigins(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), emptyEnv())
	require.NoError(t, err)

	cfg.CORSAllowedOrigins[0] = "mutated"

	assert.NotEqual(t, "mutated", config.DefaultCORSOrigins[0],
		"mutating a resolved config must not corrupt the package default")
}

// --- Load: environment overrides ---

func TestLoad_PortFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvPort: "9999"}))
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.Port)
}

func TestLoad_StorageRootFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvStorageRoot: "/tmp/artifacts"}))
	require.NoError(t, err)

	assert.Equal(t, "/tmp/artifacts", cfg.StorageRoot)
}

func TestLoad_TrimsStorageRoot(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvStorageRoot: "  data/x  "}))
	require.NoError(t, err)

	assert.Equal(t, "data/x", cfg.StorageRoot)
}

func TestLoad_MaxUploadBytesFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvMaxUploadBytes: "1048576"}))
	require.NoError(t, err)

	assert.Equal(t, int64(1048576), cfg.MaxUploadBytes)
}

func TestLoad_RejectsUnusableMaxUploadBytes(t *testing.T) {
	for _, raw := range []string{"0", "-1", "notanumber", "1.5"} {
		t.Run(raw, func(t *testing.T) {
			_, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
				fakeEnv(map[string]string{config.EnvMaxUploadBytes: raw}))

			require.Error(t, err, "%q must be rejected", raw)
			assert.Contains(t, err.Error(), config.EnvMaxUploadBytes)
		})
	}
}

func TestParseMaxUploadBytes(t *testing.T) {
	empty, err := config.ParseMaxUploadBytes("")
	require.NoError(t, err)
	assert.Equal(t, int64(config.DefaultMaxUploadBytes), empty)

	value, err := config.ParseMaxUploadBytes(" 2048 ")
	require.NoError(t, err)
	assert.Equal(t, int64(2048), value)

	_, err = config.ParseMaxUploadBytes("0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

func TestLoad_DatabaseDSNFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvDatabaseDSN: "file:/tmp/custom.db"}))
	require.NoError(t, err)

	assert.Equal(t, "file:/tmp/custom.db", cfg.DatabaseDSN)
}

func TestLoad_BlankValuesFallBackToDefaults(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(map[string]string{
		config.EnvPort:        "   ",
		config.EnvDatabaseDSN: "  ",
		config.EnvCORSOrigins: "  ",
	}))
	require.NoError(t, err)

	assert.Equal(t, config.DefaultPort, cfg.Port)
	assert.Equal(t, config.DefaultDatabaseDSN(cfg.Root), cfg.DatabaseDSN)
	assert.Equal(t, config.DefaultCORSOrigins, cfg.CORSAllowedOrigins)
}

func TestLoad_TrimsDatabaseDSN(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvDatabaseDSN: "  file:x.db  "}))
	require.NoError(t, err)

	assert.Equal(t, "file:x.db", cfg.DatabaseDSN)
}

// --- Load: port validation ---

func TestLoad_RejectsNonNumericPort(t *testing.T) {
	_, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvPort: "http"}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvPort)
}

func TestLoad_RejectsOutOfRangePorts(t *testing.T) {
	for _, raw := range []string{"0", "-1", "65536", "99999"} {
		t.Run(raw, func(t *testing.T) {
			_, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
				fakeEnv(map[string]string{config.EnvPort: raw}))

			require.Error(t, err, "port %s must be rejected", raw)
			assert.Contains(t, err.Error(), "1-65535")
		})
	}
}

func TestLoad_AcceptsBoundaryPorts(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"1", 1},
		{"65535", 65535},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
				fakeEnv(map[string]string{config.EnvPort: tc.raw}))
			require.NoError(t, err)

			assert.Equal(t, tc.want, cfg.Port)
		})
	}
}

// --- Load: CORS origins ---

func TestLoad_CORSOriginsAreSplitAndTrimmed(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvCORSOrigins: " app://a , http://b ,,http://c "}))
	require.NoError(t, err)

	assert.Equal(t, []string{"app://a", "http://b", "http://c"}, cfg.CORSAllowedOrigins)
}

func TestLoad_CORSWildcardIsPreserved(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvCORSOrigins: "*"}))
	require.NoError(t, err)

	assert.Equal(t, []string{"*"}, cfg.CORSAllowedOrigins)
}

func TestLoad_RejectsOriginsThatAreOnlySeparators(t *testing.T) {
	_, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvCORSOrigins: " , , "}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvCORSOrigins)
}

// --- Load: .env file interaction ---

func TestLoad_ReadsValuesFromDotEnvFile(t *testing.T) {
	path := writeDotEnv(t, `
# server settings
STUDY_PORT=7777
STUDY_DB_DSN=file:from-file.db
STUDY_CORS_ORIGINS=http://from-file
`)

	cfg, err := config.LoadFrom(path, emptyEnv())
	require.NoError(t, err)

	assert.Equal(t, 7777, cfg.Port)
	assert.Equal(t, "file:from-file.db", cfg.DatabaseDSN)
	assert.Equal(t, []string{"http://from-file"}, cfg.CORSAllowedOrigins)
}

func TestLoad_RealEnvironmentBeatsDotEnvFile(t *testing.T) {
	path := writeDotEnv(t, "STUDY_PORT=1111\nSTUDY_DB_DSN=file:from-file.db\n")

	cfg, err := config.LoadFrom(path, fakeEnv(map[string]string{
		config.EnvPort: "2222",
	}))
	require.NoError(t, err)

	assert.Equal(t, 2222, cfg.Port, "environment must win over the file")
	assert.Equal(t, "file:from-file.db", cfg.DatabaseDSN,
		"variables the environment omits still come from the file")
}

func TestLoad_MalformedDotEnvFileIsReported(t *testing.T) {
	path := writeDotEnv(t, "STUDY_PORT=8080\nthis line has no equals sign\n")

	_, err := config.LoadFrom(path, emptyEnv())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
}

// --- ParseDotEnv ---

func TestParseDotEnv_IgnoresCommentsAndBlankLines(t *testing.T) {
	vars, err := config.ParseDotEnv(strings.NewReader(`
# a comment
   # an indented comment

KEY=value
`))

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"KEY": "value"}, vars)
}

func TestParseDotEnv_StripsExportPrefix(t *testing.T) {
	vars, err := config.ParseDotEnv(strings.NewReader("export KEY=value\n"))

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"KEY": "value"}, vars)
}

func TestParseDotEnv_UnquotesValues(t *testing.T) {
	vars, err := config.ParseDotEnv(strings.NewReader(
		"DOUBLE=\"a value\"\nSINGLE='another value'\nBARE=plain\n"))

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"DOUBLE": "a value",
		"SINGLE": "another value",
		"BARE":   "plain",
	}, vars)
}

func TestParseDotEnv_KeepsUnmatchedQuote(t *testing.T) {
	vars, err := config.ParseDotEnv(strings.NewReader("KEY=\"unclosed\n"))

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"KEY": `"unclosed`}, vars)
}

func TestParseDotEnv_PreservesValueAfterFirstEquals(t *testing.T) {
	vars, err := config.ParseDotEnv(strings.NewReader("DSN=file:x.db?mode=ro&cache=shared\n"))

	require.NoError(t, err)
	assert.Equal(t, "file:x.db?mode=ro&cache=shared", vars["DSN"])
}

func TestParseDotEnv_RejectsLinesWithoutEquals(t *testing.T) {
	_, err := config.ParseDotEnv(strings.NewReader("GOOD=1\nBAD\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
}

func TestParseDotEnv_RejectsEmptyKey(t *testing.T) {
	_, err := config.ParseDotEnv(strings.NewReader("=value\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty key")
}

func TestParseDotEnv_EmptyInputYieldsEmptyMap(t *testing.T) {
	vars, err := config.ParseDotEnv(strings.NewReader(""))

	require.NoError(t, err)
	assert.Empty(t, vars)
}

// --- Load: ingestion model settings ---

// The key is optional at Load so the API server can boot without an ingestion
// credential it never uses. The ingestion command is what requires it.
func TestLoad_LLMSettingsDefaultWhenNothingIsSet(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), emptyEnv())
	require.NoError(t, err)

	assert.Empty(t, cfg.OpenRouterAPIKey, "a missing model key must not stop the server booting")
	assert.Equal(t, config.DefaultLLMModel, cfg.LLMModel)
	assert.Equal(t, config.DefaultOpenRouterBaseURL, cfg.OpenRouterBaseURL)
}

func TestLoad_OpenRouterAPIKeyFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvOpenRouterAPIKey: "sk-or-test"}))
	require.NoError(t, err)

	assert.Equal(t, "sk-or-test", cfg.OpenRouterAPIKey)
}

func TestLoad_LLMModelFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvLLMModel: "meta/llama-vision"}))
	require.NoError(t, err)

	assert.Equal(t, "meta/llama-vision", cfg.LLMModel)
}

func TestLoad_OpenRouterBaseURLFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvOpenRouterBaseURL: "http://127.0.0.1:9999/gateway"}))
	require.NoError(t, err)

	assert.Equal(t, "http://127.0.0.1:9999/gateway", cfg.OpenRouterBaseURL)
}

func TestLoad_LLMSettingsFromDotEnvFile(t *testing.T) {
	path := writeDotEnv(t, `
STUDY_OPENROUTER_API_KEY=sk-or-from-file
STUDY_LLM_MODEL=file/model
STUDY_OPENROUTER_BASE_URL=https://file.example/api/v1
`)

	cfg, err := config.LoadFrom(path, emptyEnv())
	require.NoError(t, err)

	assert.Equal(t, "sk-or-from-file", cfg.OpenRouterAPIKey)
	assert.Equal(t, "file/model", cfg.LLMModel)
	assert.Equal(t, "https://file.example/api/v1", cfg.OpenRouterBaseURL)
}

func TestLoad_EnvironmentBeatsDotEnvFileForLLMSettings(t *testing.T) {
	path := writeDotEnv(t, `
STUDY_LLM_MODEL=file/model
STUDY_OPENROUTER_BASE_URL=https://file.example/api/v1
`)

	cfg, err := config.LoadFrom(path, fakeEnv(map[string]string{
		config.EnvLLMModel:          "env/model",
		config.EnvOpenRouterBaseURL: "https://env.example/api/v1",
	}))
	require.NoError(t, err)

	assert.Equal(t, "env/model", cfg.LLMModel, "environment must win over the file")
	assert.Equal(t, "https://env.example/api/v1", cfg.OpenRouterBaseURL)
}

// A malformed base URL is rejected at load time and names the variable, so a
// typo aborts the run instead of failing hundreds of pages later.
func TestLoad_RejectsMalformedBaseURL(t *testing.T) {
	cases := []string{
		"openrouter.ai/api/v1", // no scheme
		"ftp://openrouter.ai/api/v1",
		"https://", // no host
		"/api/v1",
		"://openrouter.ai",
	}

	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			_, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
				fakeEnv(map[string]string{config.EnvOpenRouterBaseURL: raw}))

			require.Error(t, err, "%q must be rejected", raw)
			assert.Contains(t, err.Error(), config.EnvOpenRouterBaseURL,
				"the message must name the variable the user has to edit")
		})
	}
}

func TestLoad_BlankLLMSettingsFallBackToDefaults(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(map[string]string{
		config.EnvLLMModel:          "   ",
		config.EnvOpenRouterBaseURL: "  ",
	}))
	require.NoError(t, err)

	assert.Equal(t, config.DefaultLLMModel, cfg.LLMModel)
	assert.Equal(t, config.DefaultOpenRouterBaseURL, cfg.OpenRouterBaseURL)
}

// --- ParseOpenRouterBaseURL ---

func TestParseOpenRouterBaseURL(t *testing.T) {
	empty, err := config.ParseOpenRouterBaseURL("")
	require.NoError(t, err)
	assert.Equal(t, config.DefaultOpenRouterBaseURL, empty)

	trimmed, err := config.ParseOpenRouterBaseURL("  https://openrouter.ai/api/v1/  ")
	require.NoError(t, err)
	assert.Equal(t, "https://openrouter.ai/api/v1", trimmed,
		"a trailing slash would produce a doubled separator on the request path")

	_, err = config.ParseOpenRouterBaseURL("not a url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvOpenRouterBaseURL)
}

// --- Load: embedding model settings ---

// The default is the model the measurement in the step-5 log chose, not the slug
// the earlier cost estimate proposed. Pinning the literal is the point: a name is
// not evidence, and a silent default change would undo a measured decision.
func TestLoad_EmbeddingModelDefaultsToTheMeasuredWinner(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), emptyEnv())
	require.NoError(t, err)

	assert.Equal(t, "google/gemini-embedding-2", config.DefaultEmbeddingModel)
	assert.Equal(t, config.DefaultEmbeddingModel, cfg.EmbeddingModel)
}

func TestLoad_EmbeddingModelFromEnvironment(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvEmbeddingModel: "baai/bge-m3"}))
	require.NoError(t, err)

	assert.Equal(t, "baai/bge-m3", cfg.EmbeddingModel)
}

func TestLoad_BlankEmbeddingModelFallsBackToTheDefault(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"),
		fakeEnv(map[string]string{config.EnvEmbeddingModel: "   "}))
	require.NoError(t, err)

	assert.Equal(t, config.DefaultEmbeddingModel, cfg.EmbeddingModel)
}

func TestLoad_EmbeddingModelFromDotEnvFile(t *testing.T) {
	path := writeDotEnv(t, "STUDY_EMBEDDING_MODEL=file/embedding\n")

	cfg, err := config.LoadFrom(path, emptyEnv())
	require.NoError(t, err)

	assert.Equal(t, "file/embedding", cfg.EmbeddingModel)
}

// noEnvironment reports every variable as unset. An environment variable that is
// set to the empty string still beats the file — deliberately — so a test that
// wants the file to speak must use a lookup, not t.Setenv.
func noEnvironment(string) (string, bool) { return "", false }

func environment(vars map[string]string) config.EnvLookup {
	return func(key string) (string, bool) {
		value, ok := vars[key]
		return value, ok
	}
}

func TestLoadWithFrom_RootDefaultsToTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := config.LoadWithFrom(config.Options{}, noEnvironment)
	require.NoError(t, err)

	workingDirectory, err := os.Getwd()
	require.NoError(t, err)

	assert.Equal(t, workingDirectory, cfg.Root)
	assert.Equal(t, config.DefaultDatabaseDSN(workingDirectory), cfg.DatabaseDSN)
	assert.Equal(t, config.DefaultStorageRoot(workingDirectory), cfg.StorageRoot)
	assert.True(t, cfg.RootIsTheWorkingDirectory())
}

func TestLoadWithFrom_HomeComesFromTheEnvironment(t *testing.T) {
	home := t.TempDir()

	cfg, err := config.LoadWithFrom(config.Options{}, environment(map[string]string{
		config.EnvHome: home,
	}))
	require.NoError(t, err)

	assert.Equal(t, home, cfg.Root)
	assert.Equal(t, config.DefaultDatabaseDSN(home), cfg.DatabaseDSN)
	assert.Equal(t, config.DefaultStorageRoot(home), cfg.StorageRoot)
	assert.False(t, cfg.RootIsTheWorkingDirectory())
}

func TestLoadWithFrom_ExplicitHomeBeatsTheEnvironment(t *testing.T) {
	explicit := t.TempDir()

	cfg, err := config.LoadWithFrom(
		config.Options{Home: explicit},
		environment(map[string]string{config.EnvHome: t.TempDir()}),
	)
	require.NoError(t, err)

	assert.Equal(t, explicit, cfg.Root)
}

func TestLoadWithFrom_ReadsTheDotEnvFromTheRoot(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(home, ".env"), []byte("STUDY_PORT=9911\n"), 0o600))

	cfg, err := config.LoadWithFrom(config.Options{Home: home}, noEnvironment)
	require.NoError(t, err)

	assert.Equal(t, 9911, cfg.Port)
}

func TestLoadWithFrom_ExplicitEnvFileBeatsTheRootOne(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(home, ".env"), []byte("STUDY_PORT=9911\n"), 0o600))

	custom := filepath.Join(t.TempDir(), "custom.env")
	require.NoError(t, os.WriteFile(custom, []byte("STUDY_PORT=9922\n"), 0o600))

	cfg, err := config.LoadWithFrom(
		config.Options{Home: home, EnvFile: custom}, noEnvironment)
	require.NoError(t, err)

	assert.Equal(t, 9922, cfg.Port)
}

func TestLoadWithFrom_RelativeEnvFileResolvesFromTheWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Chdir(home)
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "local.env"), []byte("STUDY_PORT=9933\n"), 0o600))

	cfg, err := config.LoadWithFrom(
		config.Options{Home: t.TempDir(), EnvFile: "local.env"}, noEnvironment)
	require.NoError(t, err)

	assert.Equal(t, 9933, cfg.Port,
		"a relative -env-file is what the operator typed, from where they typed it")
}
