package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// readDotEnv reads and parses a .env file. A file that does not exist yields an
// empty map and no error: the file is optional by design.
//
// path is supplied by the caller (Load pins it to DotEnvFile in the working
// directory; LoadFrom takes it as an argument). It is never derived from
// request data, so there is no untrusted-input path here.
func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	return ParseDotEnv(file)
}

// ParseDotEnv parses dotenv content from an io.Reader.
//
// Supported syntax, deliberately minimal:
//
//   - `KEY=value` lines.
//   - Blank lines and lines whose first non-space character is '#' are ignored.
//   - An optional `export ` prefix.
//   - Single or double quotes around a value are stripped.
//   - A missing '=' or an empty key is an error.
//
// Keys are returned as written; no substitution, interpolation, or multi-line
// continuation is performed.
func ParseDotEnv(r interface{ Read([]byte) (int, error) }) (map[string]string, error) {
	vars := map[string]string{}
	scanner := bufio.NewScanner(r)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")

		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE, got %q", lineNumber, line)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNumber)
		}

		vars[key] = unquote(strings.TrimSpace(value))
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vars, nil
}

// unquote strips one layer of matching single or double quotes.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}

	first, last := value[0], value[len(value)-1]
	if first == last && (first == '"' || first == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
