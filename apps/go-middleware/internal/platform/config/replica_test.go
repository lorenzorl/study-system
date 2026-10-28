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

// redactionFormats is the whole matrix, not a sample. `%#v` is the one that gets
// forgotten: it prints the Go representation with field names and types, so a
// future nested struct would be dumped by a `%#v` of the parent even when the
// leaf is a Stringer. `%q` reaches Stringer too but through a different verb
// path, so it is cheap to pin rather than assume.
var redactionFormats = []string{"%v", "%+v", "%#v", "%s", "%q"}

func TestReplica_StringNeverPrintsTheSecret(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(replicaVars()))
	require.NoError(t, err)

	for _, format := range redactionFormats {
		rendered := fmt.Sprintf(format, cfg.Replica)
		assert.NotContains(t, rendered, replicaSecret, "format %s leaked the secret", format)
	}

	// The parent struct must not leak it either: a run that logs its whole
	// configuration is the realistic way this value escapes.
	for _, format := range redactionFormats {
		rendered := fmt.Sprintf(format, *cfg)
		assert.NotContains(t, rendered, replicaSecret, "format %s of the whole Config leaked the secret", format)
	}
}

// endpointUser is a key smuggled inside STUDY_R2_ENDPOINT as URL userinfo. It is
// a separate leak path from SecretAccessKey: the value never travels through the
// field the redaction was written for, so a redaction that only covers
// SecretAccessKey misses it. The repository already fixed this class once —
// 4b92c33, "never persist the credentials in a fetched URL".
const endpointUser = "urlkey"

func endpointWithCredentials() string {
	return "https://" + endpointUser + ":" + replicaSecret + "@example.test"
}

func TestReplica_StringRedactsCredentialsEmbeddedInTheEndpoint(t *testing.T) {
	vars := replicaVars()
	vars[config.EnvR2Endpoint] = endpointWithCredentials()

	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(vars))
	require.NoError(t, err, "Load must not reject it; the command reports it")

	for _, format := range redactionFormats {
		rendered := fmt.Sprintf(format, cfg.Replica)
		assert.NotContains(t, rendered, replicaSecret, "format %s leaked the endpoint secret", format)
		assert.NotContains(t, rendered, endpointUser, "format %s leaked the endpoint user", format)
	}
}

func TestReplica_ValidateRejectsAnEndpointCarryingCredentials(t *testing.T) {
	vars := replicaVars()
	vars[config.EnvR2Endpoint] = endpointWithCredentials()

	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "absent.env"), fakeEnv(vars))
	require.NoError(t, err)

	err = cfg.Replica.Validate()
	require.Error(t, err, "a credential in the URL is a configuration error")
	assert.Contains(t, err.Error(), config.EnvR2Endpoint, "the error must name the variable")
	assert.NotContains(t, err.Error(), replicaSecret, "the error must not echo the credential")
	assert.NotContains(t, err.Error(), endpointUser, "the error must not echo the credential")
}
