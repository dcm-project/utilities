//go:build e2e

package e2e_test

import "testing"

func TestNetworkTestNamespace(t *testing.T) {
	t.Run("configured namespace", func(t *testing.T) {
		t.Setenv("SP_NETWORK_NAMESPACE", "dcm-network-e2e")

		if got := networkTestNamespace(); got != "dcm-network-e2e" {
			t.Errorf("networkTestNamespace() = %q, want %q", got, "dcm-network-e2e")
		}
	})

	t.Run("empty namespace defaults to default", func(t *testing.T) {
		t.Setenv("SP_NETWORK_NAMESPACE", "")

		if got := networkTestNamespace(); got != "default" {
			t.Errorf("networkTestNamespace() = %q, want %q", got, "default")
		}
	})
}
