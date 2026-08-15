package tropmail

import (
	"errors"
	"fmt"
	"os"
	"regexp"
)

const apiKeyLength = 32

var apiKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]{32}$`)

// ErrInvalidAPIKey is returned when an API key fails local format validation.
var ErrInvalidAPIKey = errors.New("tropmail: API key must be exactly 32 alphanumeric characters")

// ValidateAPIKey checks that key is exactly 32 [A-Za-z0-9] characters.
func ValidateAPIKey(key string) error {
	if !apiKeyPattern.MatchString(key) {
		return fmt.Errorf("%w (got length %d)", ErrInvalidAPIKey, len(key))
	}
	return nil
}

// APIKeyFromEnv reads TROPMAIL_API_KEY when key is empty.
func APIKeyFromEnv(key string) (string, error) {
	if key != "" {
		return key, ValidateAPIKey(key)
	}
	envKey := os.Getenv("TROPMAIL_API_KEY")
	if envKey == "" {
		return "", fmt.Errorf("tropmail: API key is required (pass to New or set TROPMAIL_API_KEY)")
	}
	if err := ValidateAPIKey(envKey); err != nil {
		return "", err
	}
	return envKey, nil
}
