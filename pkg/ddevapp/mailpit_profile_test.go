package ddevapp_test

import (
	"os"
	"testing"

	"github.com/ddev/ddev/pkg/ddevapp"
	"github.com/ddev/ddev/pkg/fileutil"
	"github.com/ddev/ddev/pkg/globalconfig"
	"github.com/ddev/ddev/pkg/testcommon"
	asrt "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMailpitComposeProfile tests that mailpit compose file is generated correctly
// and controlled by the UseHardenedImages flag
func TestMailpitComposeProfile(t *testing.T) {
	assert := asrt.New(t)
	testcommon.ClearDockerEnv()

	origDir, _ := os.Getwd()
	testDir := testcommon.CreateTmpDir(t.Name())

	app, err := ddevapp.NewApp(testDir, false)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = os.Chdir(origDir)
		assert.NoError(err)
		err = app.Stop(true, false)
		assert.NoError(err)
		_ = os.RemoveAll(testDir)
		err = globalconfig.ReadGlobalConfig()
		require.NoError(t, err)
		globalconfig.DdevGlobalConfig.UseHardenedImages = false
		err = globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
		require.NoError(t, err)
	})

	// Test with hardened images disabled (default)
	err = globalconfig.ReadGlobalConfig()
	require.NoError(t, err)
	globalconfig.DdevGlobalConfig.UseHardenedImages = false
	err = globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
	require.NoError(t, err)

	err = app.WriteConfig()
	require.NoError(t, err)

	err = app.WriteDockerComposeYAML()
	require.NoError(t, err)

	// Mailpit compose file should exist when hardened images are disabled
	mailpitComposePath := app.GetConfigPath("docker-compose.mailpit.yaml")
	assert.True(fileutil.FileExists(mailpitComposePath), "mailpit compose file should exist when hardened images are disabled")

	// Read and verify the content contains mailpit service and profile
	mailpitContent, err := fileutil.ReadFileIntoString(mailpitComposePath)
	require.NoError(t, err)
	assert.Contains(mailpitContent, "mailpit:", "compose file should contain mailpit service")
	assert.Contains(mailpitContent, "profiles:", "compose file should contain profiles")
	assert.Contains(mailpitContent, "- mailpit", "compose file should contain mailpit profile")

	// Test with hardened images enabled
	err = globalconfig.ReadGlobalConfig()
	require.NoError(t, err)
	globalconfig.DdevGlobalConfig.UseHardenedImages = true
	err = globalconfig.WriteGlobalConfig(globalconfig.DdevGlobalConfig)
	require.NoError(t, err)

	err = app.WriteDockerComposeYAML()
	require.NoError(t, err)

	// Mailpit compose file should NOT exist when hardened images are enabled
	assert.False(fileutil.FileExists(mailpitComposePath), "mailpit compose file should not exist when hardened images are enabled")
}
