package cmd

import (
	"testing"

	"github.com/spf13/viper"
)

func TestValidateConfigRejectsInvalidPolicyRollout(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("policy-rollout", "invalid-mode")
	err := validateConfig()
	if err == nil {
		t.Fatal("expected validation error for invalid policy-rollout")
	}
}

func TestValidateConfigAcceptsCanaryRollout(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("policy-rollout", "canary")
	viper.Set("policy-canary-percent", 25)
	if err := validateConfig(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateConfigRejectsOutOfRangeCanaryPercent(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("policy-rollout", "canary")
	viper.Set("policy-canary-percent", 150)
	err := validateConfig()
	if err == nil {
		t.Fatal("expected validation error for policy-canary-percent")
	}
}
