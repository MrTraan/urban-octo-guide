package player

import (
	"fmt"
	"regexp"
)

// Limits kept small and explicit; they are the whole input contract.
const (
	maxPlayerIDLen    = 64
	maxDisplayNameLen = 32
	maxEventTypeLen   = 64

	defaultLimit = 20
	maxLimit     = 100

	defaultWindowDays = 7
	maxWindowDays     = 90
)

var countryRe = regexp.MustCompile(`^[A-Z]{2}$`)

func validatePlayerID(id string) error {
	return validateLen("player_id", id, maxPlayerIDLen)
}

func validateDisplayName(name string) error {
	return validateLen("display_name", name, maxDisplayNameLen)
}

func validateEventType(t string) error {
	return validateLen("type", t, maxEventTypeLen)
}

// validateCountry accepts ISO 3166-1 alpha-2 codes such as "FR".
func validateCountry(c string) error {
	if !countryRe.MatchString(c) {
		return fmt.Errorf("country must be a 2-letter uppercase ISO code, got %q", c)
	}
	return nil
}

func validateLen(field, value string, max int) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > max {
		return fmt.Errorf("%s must be at most %d characters", field, max)
	}
	return nil
}

// clamp turns a client-provided count into a safe one: 0 means "use the default".
func clamp(v int32, def, max int) int {
	if v <= 0 {
		return def
	}
	if int(v) > max {
		return max
	}
	return int(v)
}
