package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

func validateConfig() error {
	if err := validateFloatRange("max-cpu", 1.0, 100.0); err != nil {
		return err
	}
	if err := validateIntRange("poll-interval", 50, 60000); err != nil {
		return err
	}
	if err := validateIntRange("log-window", 2, 10000); err != nil {
		return err
	}
	if viper.IsSet("policy-rollout") {
		rollout := strings.ToLower(strings.TrimSpace(viper.GetString("policy-rollout")))
		switch rollout {
		case "shadow", "canary", "enforce":
		default:
			return fmt.Errorf("invalid config: policy-rollout must be one of shadow|canary|enforce")
		}
	}
	if err := validateIntRange("policy-canary-percent", 0, 100); err != nil {
		return err
	}

	if viper.IsSet("max-memory-mb") {
		mem := viper.GetFloat64("max-memory-mb")
		if mem < 0 {
			return fmt.Errorf("invalid config: max-memory-mb must be >= 0")
		}
	}
	if viper.IsSet("max-tokens-per-min") {
		rate := viper.GetFloat64("max-tokens-per-min")
		if rate < 0 {
			return fmt.Errorf("invalid config: max-tokens-per-min must be >= 0")
		}
	}

	return validateProfiles()
}

func validateProfiles() error {
	profiles := []string{"light", "standard", "heavy"}
	for _, p := range profiles {
		prefix := fmt.Sprintf("profiles.%s", p)
		if !viper.IsSet(prefix) {
			continue
		}

		if err := validateFloatRange(prefix+".max-cpu", 1.0, 100.0); err != nil {
			return err
		}
		if err := validateIntRange(prefix+".poll-interval", 50, 60000); err != nil {
			return err
		}
		if err := validateIntRange(prefix+".log-window", 2, 10000); err != nil {
			return err
		}
	}
	return nil
}

func validateFloatRange(key string, min, max float64) error {
	if !viper.IsSet(key) {
		return nil
	}
	val := viper.GetFloat64(key)
	if val < min || val > max {
		return fmt.Errorf("invalid config: %s must be between %.1f and %.1f", key, min, max)
	}
	return nil
}

func validateIntRange(key string, min, max int) error {
	if !viper.IsSet(key) {
		return nil
	}
	val := viper.GetInt(key)
	if val < min || val > max {
		return fmt.Errorf("invalid config: %s must be between %d and %d", key, min, max)
	}
	return nil
}
