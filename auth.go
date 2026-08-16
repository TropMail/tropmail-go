package tropmail

import (
	"errors"
	"fmt"
	"os"
	"regexp"
)

var (
	apiKeyPattern     = regexp.MustCompile(`^[A-Za-z0-9]{32}$`)
	apiKeyLivePattern = regexp.MustCompile(`^tm_live_[A-Za-z0-9]{32}$`)
)

// ErrInvalidAPIKey is returned when an API key fails local format validation.
var ErrInvalidAPIKey = errors.New("tropmail: API key must be 32 alphanumeric characters, optionally prefixed with tm_live_")

// ValidateAPIKey checks that key matches issued TropMail secrets.
func ValidateAPIKey(key string) error {
	if apiKeyPattern.MatchString(key) || apiKeyLivePattern.MatchString(key) {
		return nil
	}
	return fmt.Errorf("%w (got length %d)", ErrInvalidAPIKey, len(key))
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
