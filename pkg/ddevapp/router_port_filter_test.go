package ddevapp

import (
	"testing"

	"github.com/ddev/ddev/pkg/globalconfig"
	"github.com/stretchr/testify/require"
)

// TestFilterAllowedPublicPorts tests the FilterAllowedPublicPorts function
// for proper port filtering when router_bind_all_interfaces is enabled.
func TestFilterAllowedPublicPorts(t *testing.T) {
	// Save and restore original config
	origTraefikMonitorPort := globalconfig.DdevGlobalConfig.TraefikMonitorPort
	t.Cleanup(func() {
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = origTraefikMonitorPort
	})

	// Set up test configuration
	globalconfig.DdevGlobalConfig.TraefikMonitorPort = "10999"

	testCases := []struct {
		name          string
		inputPorts    []string
		expectedPorts []string
	}{
		{
			name:          "Standard HTTP and HTTPS ports are allowed",
			inputPorts:    []string{"80", "443"},
			expectedPorts: []string{"80", "443"},
		},
		{
			name:          "Traefik monitor port is blocked",
			inputPorts:    []string{"80", "443", "10999"},
			expectedPorts: []string{"80", "443"},
		},
		{
			name:          "Project-configured ports are allowed",
			inputPorts:    []string{"80", "443", "8025", "8026", "9200"},
			expectedPorts: []string{"80", "443", "8025", "8026", "9200"},
		},
		{
			name:          "Mixed ports with Traefik monitor blocked",
			inputPorts:    []string{"80", "443", "8025", "10999", "9200"},
			expectedPorts: []string{"80", "443", "8025", "9200"},
		},
		{
			name:          "Only Traefik monitor port",
			inputPorts:    []string{"10999"},
			expectedPorts: []string{},
		},
		{
			name:          "Empty port list",
			inputPorts:    []string{},
			expectedPorts: []string{},
		},
		{
			name:          "Custom Traefik monitor port",
			inputPorts:    []string{"80", "443", "11999"},
			expectedPorts: []string{"80", "443", "11999"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FilterAllowedPublicPorts(tc.inputPorts)
			require.Equal(t, tc.expectedPorts, result, "Port filtering did not match expected result")
		})
	}

	// Test with custom Traefik monitor port
	t.Run("Custom Traefik monitor port is blocked", func(t *testing.T) {
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = "11999"
		inputPorts := []string{"80", "443", "11999", "8025"}
		expectedPorts := []string{"80", "443", "8025"}
		result := FilterAllowedPublicPorts(inputPorts)
		require.Equal(t, expectedPorts, result)
	})
}
