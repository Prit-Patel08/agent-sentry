package cmd

import (
	"testing"

	"flowforge/internal/policy"
	"github.com/spf13/viper"
)

func withRolloutGlobals(t *testing.T) {
	t.Helper()
	oldShadow := shadowMode
	oldRollout := policyRollout
	oldPercent := policyCanaryPercent
	t.Cleanup(func() {
		shadowMode = oldShadow
		policyRollout = oldRollout
		policyCanaryPercent = oldPercent
		viper.Reset()
	})
}

func TestResolvePolicyRolloutConfigShadowCompatibility(t *testing.T) {
	withRolloutGlobals(t)
	viper.Reset()
	shadowMode = true
	policyRollout = ""
	policyCanaryPercent = -1

	mode, percent := resolvePolicyRolloutConfig()
	if mode != policy.RolloutShadow {
		t.Fatalf("expected shadow mode, got %s", mode)
	}
	if percent != 0 {
		t.Fatalf("expected canary percent 0, got %d", percent)
	}
}

func TestResolvePolicyRolloutConfigFromFlags(t *testing.T) {
	withRolloutGlobals(t)
	viper.Reset()
	shadowMode = false
	policyRollout = "canary"
	policyCanaryPercent = 35

	mode, percent := resolvePolicyRolloutConfig()
	if mode != policy.RolloutCanary {
		t.Fatalf("expected canary mode, got %s", mode)
	}
	if percent != 35 {
		t.Fatalf("expected canary percent 35, got %d", percent)
	}
}

func TestResolvePolicyRolloutConfigCanaryDefaultPercent(t *testing.T) {
	withRolloutGlobals(t)
	viper.Reset()
	shadowMode = false
	policyRollout = "canary"
	policyCanaryPercent = -1

	mode, percent := resolvePolicyRolloutConfig()
	if mode != policy.RolloutCanary {
		t.Fatalf("expected canary mode, got %s", mode)
	}
	if percent != 10 {
		t.Fatalf("expected default canary percent 10, got %d", percent)
	}
}

func TestResolvePolicyRolloutConfigFromViper(t *testing.T) {
	withRolloutGlobals(t)
	viper.Reset()
	shadowMode = false
	policyRollout = ""
	policyCanaryPercent = -1
	viper.Set("policy-rollout", "canary")
	viper.Set("policy-canary-percent", 22)

	mode, percent := resolvePolicyRolloutConfig()
	if mode != policy.RolloutCanary {
		t.Fatalf("expected canary mode from config, got %s", mode)
	}
	if percent != 22 {
		t.Fatalf("expected canary percent 22 from config, got %d", percent)
	}
}
