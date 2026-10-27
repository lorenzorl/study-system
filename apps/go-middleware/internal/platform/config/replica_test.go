package config_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
)

// replicaSecret is a stand-in for the bucket credential. It is the example
// credential published in the AWS Signature Version 4 documentation, which makes
// it recognisable to a reader as a fixture rather than a live key. It is
// distinctive on purpose: any test that prints a ReplicaSettings asserts this
// exact string is absent, so a redaction regression fails on the value and not
// on a shape.
const replicaSecret = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"

// replicaVars is a fully configured bucket destination.
func replicaVars() map[string]string {
	return map[string]string{
		config.EnvR2AccountID:       "0123456789abcdef0123456789abcdef",
		config.EnvR2Bucket:          "study-system",
		config.EnvR2AccessKeyID:     "AKIDEXAMPLE",
		config.EnvR2SecretAccessKey: replicaSecret,
	}
}

// --- S8: nothing that works today may require R2 to work ---

func TestLoad_ReplicaIsAbsentByDefault(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), emptyEnv())

	require.NoError(t, err)
	assert.True(t, cfg.Replica.IsZero(), "an unset replica must stay unset")
}

func TestLoad_PartialReplicaSettingsDoNotStopTheProcess(t *testing.T) {
	// Only the bucket is set. The server must still boot: the replica is used by
	// the backup command, and a half-configured bucket is that command's error to
	// report, not a reason for every other command to fail.
	env := fakeEnv(map[string]string{config.EnvR2Bucket: "study-system"})

	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), env)

	require.NoError(t, err)
	assert.Equal(t, "study-system", cfg.Replica.Bucket)

	err = cfg.Replica.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvR2AccountID,
		"the error must name the variable the user has to set")
	assert.Contains(t, err.Error(), config.EnvR2AccessKeyID)
	assert.NotContains(t, err.Error(), config.EnvR2Bucket,
		"a configured variable is not reported as missing")
}

// --- the settings are read, trimmed, and complete ---

func TestLoad_ReadsReplicaSettingsFromEnvironment(t *testing.T) {
	vars := replicaVars()
	vars[config.EnvR2Endpoint] = "  https://example.test  "
	vars[config.EnvR2Prefix] = "  blue  "

	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(vars))
	require.NoError(t, err)

	assert.Equal(t, vars[config.EnvR2AccountID], cfg.Replica.AccountID)
	assert.Equal(t, "study-system", cfg.Replica.Bucket)
	assert.Equal(t, "AKIDEXAMPLE", cfg.Replica.AccessKeyID)
	assert.Equal(t, replicaSecret, cfg.Replica.SecretAccessKey)
	assert.Equal(t, "https://example.test", cfg.Replica.Endpoint)
	assert.Equal(t, "blue", cfg.Replica.Prefix)
	assert.NoError(t, cfg.Replica.Validate())
}

func TestLoad_ReadsReplicaSettingsFromDotEnvFile(t *testing.T) {
	var file strings.Builder
	for key, value := range replicaVars() {
		file.WriteString(key + "=" + value + "\n")
	}
	path := writeDotEnv(t, file.String())

	cfg, err := config.LoadFrom(path, emptyEnv())
	require.NoError(t, err)

	assert.Equal(t, "study-system", cfg.Replica.Bucket)
	assert.Equal(t, replicaSecret, cfg.Replica.SecretAccessKey)
	assert.NoError(t, cfg.Replica.Validate())
}

func TestLoad_EnvironmentBeatsDotEnvFileForReplicaSettings(t *testing.T) {
	path := writeDotEnv(t, config.EnvR2Bucket+"=from-file\n")

	cfg, err := config.LoadFrom(path, fakeEnv(map[string]string{config.EnvR2Bucket: "from-env"}))
	require.NoError(t, err)

	assert.Equal(t, "from-env", cfg.Replica.Bucket)
}

// --- endpoint derivation ---

func TestReplica_EndpointDefaultsToTheAccountHost(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(replicaVars()))
	require.NoError(t, err)

	assert.Equal(t,
		"https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com",
		cfg.Replica.EffectiveEndpoint())
}

func TestReplica_EndpointOverrideWins(t *testing.T) {
	vars := replicaVars()
	vars[config.EnvR2Endpoint] = "http://127.0.0.1:9000"

	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(vars))
	require.NoError(t, err)

	assert.Equal(t, "http://127.0.0.1:9000", cfg.Replica.EffectiveEndpoint())
}

func TestReplica_ValidateRejectsAnUnusableEndpoint(t *testing.T) {
	vars := replicaVars()
	vars[config.EnvR2Endpoint] = "127.0.0.1:9000"

	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(vars))
	require.NoError(t, err, "Load must not reject it; the command reports it")

	err = cfg.Replica.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), config.EnvR2Endpoint)
}

// --- S9: the secret is never emitted ---

func TestReplica_StringNeverPrintsTheSecret(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(replicaVars()))
	require.NoError(t, err)

	for _, format := range []string{"%v", "%+v", "%s"} {
		rendered := fmt.Sprintf(format, cfg.Replica)
		assert.NotContains(t, rendered, replicaSecret, "format %s leaked the secret", format)
		assert.Contains(t, rendered, "study-system", "format %s should still identify the bucket", format)
	}

	// The parent struct must not leak it either: a run that logs its whole
	// configuration is the realistic way this value escapes.
	assert.NotContains(t, fmt.Sprintf("%+v", *cfg), replicaSecret)
}
