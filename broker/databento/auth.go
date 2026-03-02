package databento

import (
	"os"
	"strings"
)

type ApiKey string

// MustLoadDefaultKey loads the API key from the default location and panics on error.
func MustLoadDefaultKey() ApiKey {
	key, err := LoadKey(os.ExpandEnv("$HOME/.databento.key"))
	if err != nil {
		panic(err)
	}
	return key
}

// LoadKey reads Databento API key from file.
func LoadKey(path string) (ApiKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ApiKey(strings.TrimSpace(string(data))), nil
}
