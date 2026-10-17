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
	assert.Equal(t, config.DefaultDatabaseDSN, cfg.DatabaseDSN)
	assert.Equal(t, config.DefaultStorageRoot, cfg.StorageRoot)
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
	assert.Equal(t, config.DefaultDatabaseDSN, cfg.DatabaseDSN)
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
