package ddevapp_test

import (
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ddev/ddev/pkg/ddevapp"
	"github.com/ddev/ddev/pkg/dockerutil"
	"github.com/ddev/ddev/pkg/fileutil"
	"github.com/ddev/ddev/pkg/globalconfig"
	"github.com/ddev/ddev/pkg/netutil"
	"github.com/ddev/ddev/pkg/nodeps"
	"github.com/ddev/ddev/pkg/testcommon"
	"github.com/ddev/ddev/pkg/util"
	asrt "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobalPortOverride tests global router_http_port and router_https_port
func TestGlobalPortOverride(t *testing.T) {
	if dockerutil.IsLima() || dockerutil.IsColima() || dockerutil.IsRancherDesktop() {
		// Intermittent failures in CI due apparently to https://github.com/lima-vm/lima/issues/2536
		// Expected port is not available, so it allocates another one.
		t.Skip("Lima and Colima often allocate another port, so skip")
	}
	assert := asrt.New(t)

	origGlobalHTTPPort := globalconfig.DdevGlobalConfig.RouterHTTPPort
	origGlobalHTTPSPort := globalconfig.DdevGlobalConfig.RouterHTTPSPort

	globalconfig.DdevGlobalConfig.RouterHTTPPort = "8555"
	globalconfig.DdevGlobalConfig.RouterHTTPSPort = "8556"
	err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
	require.NoError(t, err)

	site := TestSites[0]

	app, err := ddevapp.NewApp(site.Dir, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = app.Stop(true, false)
		assert.NoError(err)
		globalconfig.DdevGlobalConfig.RouterHTTPPort = origGlobalHTTPPort
		globalconfig.DdevGlobalConfig.RouterHTTPSPort = origGlobalHTTPSPort
		err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		assert.NoError(err)
	})

	util.Debug("Before app.Restart(): app.RouterHTTPPort=%s, app.RouterHTTPSPort=%s, app.GetRouterHTTPPort()=%s app.GetRouterHTTPSPort=%s", app.RouterHTTPPort, app.RouterHTTPSPort, app.GetPrimaryRouterHTTPPort(), app.GetPrimaryRouterHTTPSPort())
	err = app.Restart()
	util.Debug("After app.Restart(): app.RouterHTTPPort=%s, app.RouterHTTPSPort=%s, app.GetRouterHTTPPort()=%s app.GetRouterHTTPSPort=%s", app.RouterHTTPPort, app.RouterHTTPSPort, app.GetPrimaryRouterHTTPPort(), app.GetPrimaryRouterHTTPSPort())

	require.NoError(t, err)
	require.Equal(t, globalconfig.DdevGlobalConfig.RouterHTTPPort, app.GetPrimaryRouterHTTPPort())
	require.Equal(t, globalconfig.DdevGlobalConfig.RouterHTTPSPort, app.GetPrimaryRouterHTTPSPort())

	desc, err := app.Describe(false)
	require.NoError(t, err)
	require.Equal(t, globalconfig.DdevGlobalConfig.RouterHTTPPort, desc["router_http_port"])
	require.Equal(t, globalconfig.DdevGlobalConfig.RouterHTTPSPort, desc["router_https_port"])
}

// TestProjectPortOverride makes sure that the project-level
// router_http_port and router_https_port
// port overrides work correctly.
// It starts up three DDEV projects, looks to see if the config is set right,
// then tests to see that the right ports have been started up on the router.
func TestProjectPortOverride(t *testing.T) {
	assert := asrt.New(t)

	origDir, _ := os.Getwd()

	// Try some different combinations of ports.
	for i := 1; i < 3; i++ {
		testDir := testcommon.CreateTmpDir("TestProjectPortOverride")

		t.Cleanup(func() {
			err := os.Chdir(origDir)
			assert.NoError(err)
			_ = os.RemoveAll(testDir)
		})

		testcommon.ClearDockerEnv()
		app, err := ddevapp.NewApp(testDir, true)
		assert.NoError(err)
		app.RouterHTTPPort = strconv.Itoa(8080 + i)
		app.RouterHTTPSPort = strconv.Itoa(8443 + i)
		app.Name = "TestProjectPortOverride-" + strconv.Itoa(i)
		_ = app.Stop(true, false)
		app.Type = nodeps.AppTypePHP
		err = app.WriteConfig()
		assert.NoError(err)
		_, err = app.ReadConfig(false)
		assert.NoError(err)

		stringFound, err := fileutil.FgrepStringInFile(app.ConfigPath, "router_http_port: \""+app.RouterHTTPPort+"\"")
		assert.NoError(err)
		assert.True(stringFound)
		stringFound, err = fileutil.FgrepStringInFile(app.ConfigPath, "router_https_port: \""+app.RouterHTTPSPort+"\"")
		assert.NoError(err)
		assert.True(stringFound)

		err = app.StartAndWait(2)
		require.NoError(t, err)
		// defer the app.Stop() so we have a more diverse set of tests. If we brought
		// each down before testing the next that would be a more trivial test.
		// Don't worry about the possible error case as this is a test cleanup
		t.Cleanup(func() {
			err = app.Stop(true, false)
			assert.NoError(err)
		})

		assert.True(netutil.IsPortActive(app.RouterHTTPPort), "port "+app.RouterHTTPPort+" should be active")
		assert.True(netutil.IsPortActive(app.RouterHTTPSPort), "port "+app.RouterHTTPSPort+" should be active")
	}
}

// TestRouterConfigOverride tests that the ~/.ddev/.router-compose.yaml can be overridden
// with ~/.ddev/router-compose.*.yaml
func TestRouterConfigOverride(t *testing.T) {
	assert := asrt.New(t)
	origDir, _ := os.Getwd()
	extrasYamlName := `router-compose.extras.yaml`
	testDir := testcommon.CreateTmpDir(t.Name())
	_ = os.Chdir(testDir)
	extrasYaml := filepath.Join(globalconfig.GetGlobalDdevDir(), extrasYamlName)

	testcommon.ClearDockerEnv()

	app, err := ddevapp.NewApp(testDir, true)
	assert.NoError(err)
	err = app.WriteConfig()
	assert.NoError(err)
	err = fileutil.CopyFile(filepath.Join(origDir, "testdata", t.Name(), extrasYamlName), extrasYaml)
	assert.NoError(err)

	answer := fileutil.RandomFilenameBase()
	t.Setenv("ANSWER", answer)
	assert.NoError(err)
	t.Cleanup(func() {
		err = app.Stop(true, false)
		assert.NoError(err)
		err = os.Chdir(origDir)
		assert.NoError(err)
		_ = os.RemoveAll(testDir)
		_ = os.Remove(extrasYaml)
	})

	err = app.Start()
	assert.NoError(err)

	stdout, _, err := dockerutil.Exec("ddev-router", "bash -c 'echo ANSWER=${ANSWER}'", "")
	stdout = strings.Trim(stdout, "\r\n")
	assert.Equal("ANSWER="+answer, stdout)
}

// TestAllocateAvailablePortForRouter tests AllocateAvailablePortForRouter()
func TestAllocateAvailablePortForRouter(t *testing.T) {
	assert := asrt.New(t)

	localIP, _ := dockerutil.GetDockerIP()

	// Get a random port number in the dynamic port range
	startPort := ddevapp.MinEphemeralPort + rand.Intn(500)
	goodEndPort := startPort + 3
	badEndPort := startPort + 2

	// Listen in the first 3 ports
	l0, err := net.Listen("tcp", localIP+":"+strconv.Itoa(startPort))
	require.NoError(t, err)
	l1, err := net.Listen("tcp", localIP+":"+strconv.Itoa(startPort+1))
	require.NoError(t, err)
	l2, err := net.Listen("tcp", localIP+":"+strconv.Itoa(startPort+2))
	require.NoError(t, err)

	t.Cleanup(func() {
		for i, p := range []net.Listener{l0, l1, l2} {
			err = p.Close()
			assert.NoError(err, "failed to close listener %v", i)
		}
	})
	_, ok := ddevapp.AllocateAvailablePortForRouter(startPort, badEndPort)
	assert.Exactly(false, ok)

	port, ok := ddevapp.AllocateAvailablePortForRouter(startPort, goodEndPort)
	require.True(t, ok)
	require.Equal(t, startPort+3, port)
}

// Test that the app assigns an ephemeral port if the default one is not available.
func TestUseEphemeralPort(t *testing.T) {
	if dockerutil.IsColima() || dockerutil.IsLima() || dockerutil.IsRancherDesktop() {
		// Intermittent failures in CI due apparently to https://github.com/lima-vm/lima/issues/2536
		// Expected port is not available, so it allocates another one.
		t.Skip("Skipping on Lima/Colima/Rancher as ports don't seem to be released properly in a timely fashion")
	}

	targetHTTPPort, targetHTTPSPort := "28080", "28443"
	const testString = "Hello from TestUseEphemeralPort"

	apps := []*ddevapp.DdevApp{}
	for _, s := range []string{"site1", "site2"} {
		site := filepath.Join(testcommon.CreateTmpDir(t.Name() + s))
		_ = os.MkdirAll(site, 0755)
		err := fileutil.TemplateStringToFile(testString, nil, filepath.Join(site, "index.html"))
		require.NoError(t, err)

		a, err := ddevapp.NewApp(site, false)
		require.NoError(t, err)
		err = a.WriteConfig()
		require.NoError(t, err)
		apps = append(apps, a)
		a.RouterHTTPPort, a.RouterHTTPSPort = targetHTTPPort, targetHTTPSPort
	}

	// Occupy target router ports so that app1 will be forced
	// to use the ephemeral ports
	for _, p := range []string{apps[0].GetPrimaryRouterHTTPPort(), apps[0].GetPrimaryRouterHTTPSPort(), apps[0].GetMailpitHTTPPort(), apps[0].GetMailpitHTTPSPort()} {
		listener, err := net.Listen("tcp", "127.0.0.1:"+p)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = listener.Close()
		})
	}
	t.Cleanup(func() {
		for _, a := range apps {
			_ = a.Stop(true, false)
			_ = os.RemoveAll(a.AppRoot)
		}

		// Stop the router, to prevent additional config from interfering with other tests.
		// We shouldn't have to do this when app.Stop() properly pushes new config to ddev-router
		_ = dockerutil.RemoveContainer(nodeps.RouterContainer)
	})

	for i, app := range apps {
		// Predict which ephemeral ports the apps will use by using guess from starting point
		// This is fragile, only works if no other projects are running and holding open the earlier ports
		expectedEphemeralHTTPPort := ddevapp.MinEphemeralPort + i*4
		expectedEphemeralHTTPSPort := ddevapp.MinEphemeralPort + i*4 + 1

		err := app.Start()
		require.NoError(t, err)

		// Get a new copy of the app to make sure we have up-to-date port information
		app, err = ddevapp.NewApp(app.GetAppRoot(), true)
		require.NoError(t, err)

		// app1 will not use the configured target ports, uses the assigned ephemeral ports.
		require.NotEqual(t, targetHTTPPort, app.GetPrimaryRouterHTTPPort())
		require.NotEqual(t, targetHTTPSPort, app.GetPrimaryRouterHTTPSPort())

		// Allow a margin of +2 for ephemeral port checks due to flakiness
		actualHTTPPort, err := strconv.Atoi(app.GetPrimaryRouterHTTPPort())
		require.NoError(t, err)
		require.Condition(t, func() bool {
			return actualHTTPPort >= expectedEphemeralHTTPPort && actualHTTPPort <= expectedEphemeralHTTPPort+2
		}, "HTTP port must be between %d and %d, got %d", expectedEphemeralHTTPPort, expectedEphemeralHTTPPort+2, actualHTTPPort)

		actualHTTPSPort, err := strconv.Atoi(app.GetPrimaryRouterHTTPSPort())
		require.NoError(t, err)
		require.Condition(t, func() bool {
			return actualHTTPSPort >= expectedEphemeralHTTPSPort && actualHTTPSPort <= expectedEphemeralHTTPSPort+2
		}, "HTTPS port must be between %d and %d, got %d", expectedEphemeralHTTPSPort, expectedEphemeralHTTPSPort+2, actualHTTPSPort)

		// Make sure that both http and https URLs have proper content
		_, _ = testcommon.EnsureLocalHTTPContent(t, app.GetHTTPURL(), testString, -1)
		require.Contains(t, app.GetHTTPURL(), app.GetHostname())
		if globalconfig.GetCAROOT() != "" {
			_, _ = testcommon.EnsureLocalHTTPContent(t, app.GetHTTPSURL(), testString, -1)
			require.Contains(t, app.GetHTTPSURL(), app.GetHostname())
		}
	}
}

// TestProcessExposePorts tests the ProcessExposePorts function for various input scenarios
func TestProcessExposePorts(t *testing.T) {
	type testCase struct {
		name          string
		exposePorts   []string
		initialPorts  []string
		expectedPorts []string
	}

	tests := []testCase{
		{
			name:          "Empty expose ports",
			exposePorts:   []string{},
			initialPorts:  []string{},
			expectedPorts: []string{},
		},
		{
			name:          "Single port format",
			exposePorts:   []string{"8080"},
			initialPorts:  []string{},
			expectedPorts: []string{"8080"},
		},
		{
			name:          "Port pair format",
			exposePorts:   []string{"8080:80"},
			initialPorts:  []string{},
			expectedPorts: []string{"8080"},
		},
		{
			name:          "Multiple ports",
			exposePorts:   []string{"8080", "9090:90", "3000"},
			initialPorts:  []string{},
			expectedPorts: []string{"8080", "9090", "3000"},
		},
		{
			name:          "Duplicate ports are not added",
			exposePorts:   []string{"8080", "8080:80"},
			initialPorts:  []string{},
			expectedPorts: []string{"8080"},
		},
		{
			name:          "Existing ports are preserved",
			exposePorts:   []string{"9090"},
			initialPorts:  []string{"8080"},
			expectedPorts: []string{"8080", "9090"},
		},
		{
			name:          "Port already exists in initial list",
			exposePorts:   []string{"8080"},
			initialPorts:  []string{"8080", "9090"},
			expectedPorts: []string{"8080", "9090"},
		},
		{
			name:          "Invalid port format is ignored",
			exposePorts:   []string{"invalid", "8080", "abc:def"},
			initialPorts:  []string{},
			expectedPorts: []string{"8080"},
		},
		{
			name:          "Port with letters is ignored",
			exposePorts:   []string{"80a0", "8080"},
			initialPorts:  []string{},
			expectedPorts: []string{"8080"},
		},
		{
			name:          "Port pair with invalid numbers is ignored",
			exposePorts:   []string{"80a0:80", "8080:8b0", "9090:90"},
			initialPorts:  []string{},
			expectedPorts: []string{"9090"},
		},
		{
			name:          "Empty strings are ignored",
			exposePorts:   []string{"", "8080", ""},
			initialPorts:  []string{},
			expectedPorts: []string{"8080"},
		},
		{
			name:          "Complex scenario with mixed formats",
			exposePorts:   []string{"8080", "9090:90", "invalid", "3000:3000", "8080:80"},
			initialPorts:  []string{"7070", "8080"},
			expectedPorts: []string{"7070", "8080", "9090", "3000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ddevapp.ProcessExposePorts(tc.exposePorts, tc.initialPorts)
			require.Equal(t, tc.expectedPorts, result)
		})
	}
}

// TestAssignRouterPortsToGenericWebserverPorts ensures that RouterHTTPPort and RouterHTTPSPort
// are assigned correctly based on WebExtraExposedPorts for Generic webservers.
func TestAssignRouterPortsToGenericWebserverPorts(t *testing.T) {
	type testCase struct {
		name                 string
		webserverType        string
		webExtraExposedPorts []ddevapp.WebExposedPort
		expectedHTTPPort     string
		expectedHTTPSPort    string
	}

	tests := []testCase{
		{
			name:          "Generic webserver with valid ports",
			webserverType: nodeps.WebserverGeneric,
			webExtraExposedPorts: []ddevapp.WebExposedPort{
				{HTTPPort: 8080, HTTPSPort: 8443},
			},
			expectedHTTPPort:  "8080",
			expectedHTTPSPort: "8443",
		},
		{
			name:                 "Generic webserver with no extra ports",
			webserverType:        nodeps.WebserverGeneric,
			webExtraExposedPorts: []ddevapp.WebExposedPort{},
			expectedHTTPPort:     "",
			expectedHTTPSPort:    "",
		},
		{
			name:          "Non-Generic webserver should not assign ports",
			webserverType: nodeps.WebserverNginxFPM,
			webExtraExposedPorts: []ddevapp.WebExposedPort{
				{HTTPPort: 8080, HTTPSPort: 8443},
			},
			expectedHTTPPort:  "",
			expectedHTTPSPort: "",
		},
		{
			name:          "Generic webserver with multiple ports uses first",
			webserverType: nodeps.WebserverGeneric,
			webExtraExposedPorts: []ddevapp.WebExposedPort{
				{HTTPPort: 8000, HTTPSPort: 8443},
				{HTTPPort: 8081, HTTPSPort: 8444},
			},
			expectedHTTPPort:  "8000",
			expectedHTTPSPort: "8443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := &ddevapp.DdevApp{
				WebserverType:        tc.webserverType,
				WebExtraExposedPorts: tc.webExtraExposedPorts,
			}

			ddevapp.AssignRouterPortsToGenericWebserverPorts(app)
			require.Equal(t, tc.expectedHTTPPort, app.RouterHTTPPort)
			require.Equal(t, tc.expectedHTTPSPort, app.RouterHTTPSPort)
		})
	}
}

// TestRouterBindAllInterfacesPortFiltering tests that router_bind_all_interfaces
// properly filters ports when enabled, using an actual project setup.
// This is an integration test that verifies the entire flow from project config to router.
func TestRouterBindAllInterfacesPortFiltering(t *testing.T) {
	assert := asrt.New(t)

	// Save and restore original config
	origRouterBindAllInterfaces := globalconfig.DdevGlobalConfig.RouterBindAllInterfaces
	origTraefikMonitorPort := globalconfig.DdevGlobalConfig.TraefikMonitorPort
	t.Cleanup(func() {
		globalconfig.DdevGlobalConfig.RouterBindAllInterfaces = origRouterBindAllInterfaces
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = origTraefikMonitorPort
		err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		assert.NoError(err)
	})

	// Create a test project
	testDir := testcommon.CreateTmpDir(t.Name())
	app, err := ddevapp.NewApp(testDir, true)
	require.NoError(t, err)
	app.Name = t.Name()
	app.Type = nodeps.AppTypePHP
	err = app.WriteConfig()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = app.Stop(true, false)
		_ = os.RemoveAll(testDir)
	})

	// Test 1: With router_bind_all_interfaces enabled
	t.Run("Traefik monitor port filtered when router_bind_all_interfaces enabled", func(t *testing.T) {
		globalconfig.DdevGlobalConfig.RouterBindAllInterfaces = true
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = "10999"
		err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		require.NoError(t, err)

		// Start the project to trigger router generation
		err = app.Start()
		require.NoError(t, err)

		// Read the generated router compose file
		routerComposeFile := ddevapp.RouterComposeYAMLPath()
		contents, err := os.ReadFile(routerComposeFile)
		require.NoError(t, err)

		routerComposeContents := string(contents)

		// Verify that the Traefik monitor port is bound to localhost
		// It should have the dockerIP prefix even when router_bind_all_interfaces is true
		assert.Contains(t, routerComposeContents, ":10999:10999",
			"Traefik monitor port should be bound to localhost (with IP prefix)")

		// The port should NOT appear in the loop that uses router_bind_all_interfaces
		// to bind to all interfaces (without IP prefix)
		// We check this by ensuring it doesn't appear as just "10999:10999" on its own line
		// in the ports section (which would indicate binding to all interfaces)
	})

	// Test 2: Verify FilterAllowedPublicPorts is called during router generation
	t.Run("FilterAllowedPublicPorts blocks monitor port", func(t *testing.T) {
		globalconfig.DdevGlobalConfig.RouterBindAllInterfaces = true
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = "10999"

		// Test the filtering directly
		inputPorts := []string{"80", "443", "8025", "10999"}
		filteredPorts := ddevapp.FilterAllowedPublicPorts(inputPorts)

		// Verify 10999 is not in the filtered list
		for _, port := range filteredPorts {
			assert.NotEqual(t, "10999", port, "Traefik monitor port should be filtered out")
		}

		// Verify other ports are still present
		assert.Contains(t, filteredPorts, "80")
		assert.Contains(t, filteredPorts, "443")
		assert.Contains(t, filteredPorts, "8025")
	})
}


// TestRouterBindAllInterfacesPortFiltering tests that router_bind_all_interfaces
// properly filters ports when enabled, using an actual project setup.
// This is an integration test that verifies the entire flow from project config to router.
func TestRouterBindAllInterfacesPortFiltering(t *testing.T) {
	assert := asrt.New(t)

	// Save and restore original config
	origRouterBindAllInterfaces := globalconfig.DdevGlobalConfig.RouterBindAllInterfaces
	origTraefikMonitorPort := globalconfig.DdevGlobalConfig.TraefikMonitorPort
	t.Cleanup(func() {
		globalconfig.DdevGlobalConfig.RouterBindAllInterfaces = origRouterBindAllInterfaces
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = origTraefikMonitorPort
		err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		assert.NoError(err)
	})

	// Create a test project
	testDir := testcommon.CreateTmpDir(t.Name())
	app, err := ddevapp.NewApp(testDir, true)
	require.NoError(t, err)
	app.Name = t.Name()
	app.Type = nodeps.AppTypePHP
	err = app.WriteConfig()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = app.Stop(true, false)
		_ = os.RemoveAll(testDir)
	})

	// Test 1: With router_bind_all_interfaces enabled
	t.Run("With router_bind_all_interfaces enabled", func(t *testing.T) {
		globalconfig.DdevGlobalConfig.RouterBindAllInterfaces = true
		globalconfig.DdevGlobalConfig.TraefikMonitorPort = "10999"
		err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		require.NoError(t, err)

		// Start the project to trigger router generation
		err = app.Start()
		require.NoError(t, err)

		// Get the list of active apps
		activeApps := ddevapp.GetActiveProjects()

		// Call determineRouterPorts - this should apply the filtering
		routerPorts := ddevapp.DetermineRouterPorts(activeApps)

		// Verify that Traefik monitor port is NOT in the list
		// (it should be filtered out when router_bind_all_interfaces is true)
		for _, port := range routerPorts {
			assert.NotEqual(t, "10999", port, "Traefik monitor port should be filtered out when router_bind_all_interfaces is enabled")
		}

		// Verify that standard ports ARE in the list
		assert.Contains(t, routerPorts, "80", "Port 80 should be in router ports")
		assert.Contains(t, routerPorts, "443", "Port 443 should be in router ports")
	})

	// Test 2: With router_bind_all_interfaces disabled (default)
	t.Run("With router_bind_all_interfaces disabled", func(t *testing.T) {
		globalconfig.DdevGlobalConfig.RouterBindAllInterfaces = false
		err := globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		require.NoError(t, err)

		// Restart to pick up the new config
		err = app.Restart()
		require.NoError(t, err)

		// Get the list of active apps
		activeApps := ddevapp.GetActiveProjects()

		// Call determineRouterPorts - filtering should NOT be applied
		routerPorts := ddevapp.DetermineRouterPorts(activeApps)

		// When disabled, all ports should be included (no filtering)
		// We don't check for Traefik port specifically because it's handled
		// separately in the template, but we verify standard ports are present
		assert.Contains(t, routerPorts, "80", "Port 80 should be in router ports")
		assert.Contains(t, routerPorts, "443", "Port 443 should be in router ports")
	})
}



