package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestNewRootCommandExecutesRunFuncAndBindsConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		expectRepo    string
		expectContext int
		expectTheme   string
	}{
		{
			name:          "binds repo path, context lines, and theme",
			args:          []string{"--repo-path", "/tmp/repo", "--context-lines", "2", "--theme", "light"},
			expectRepo:    "/tmp/repo",
			expectContext: 2,
			expectTheme:   "light",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := viper.New()
			called := false

			cmd, err := NewRootCommand(cfg, func() error {
				called = true
				return nil
			})
			require.NoError(t, err)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(tt.args)

			err = cmd.Execute()
			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, tt.expectRepo, cfg.GetString("repo-path"))
			assert.Equal(t, tt.expectContext, cfg.GetInt("context-lines"))
			assert.Equal(t, tt.expectTheme, cfg.GetString("theme"))
			assert.Equal(t, string(core.DiffModeBranch), cfg.GetString("diff-mode"))
		})
	}
}

func TestNewRootCommandPlainExecutionOverridesStickyDiffConfig(t *testing.T) {
	t.Parallel()

	cfg := viper.New()
	cfg.Set("diff-mode", string(core.DiffModeCommit))
	cfg.Set("revision", "HEAD~1")

	cmd, err := NewRootCommand(cfg, func() error { return nil })
	require.NoError(t, err)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(nil)

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, string(core.DiffModeBranch), cfg.GetString("diff-mode"))
	assert.Empty(t, cfg.GetString("revision"))
}

func TestNewRootCommandSelectsInitialDiffMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantMode   core.DiffMode
		wantConfig map[string]string
	}{
		{name: "branch command", args: []string{"branch"}, wantMode: core.DiffModeBranch},
		{name: "working command", args: []string{"working"}, wantMode: core.DiffModeWorking},
		{name: "staged command", args: []string{"staged"}, wantMode: core.DiffModeStaged},
		{name: "local command", args: []string{"local"}, wantMode: core.DiffModeLocal},
		{name: "upstream command", args: []string{"upstream", "origin/main"}, wantMode: core.DiffModeUpstream, wantConfig: map[string]string{"upstream-ref": "origin/main"}},
		{name: "commit command", args: []string{"commit", "HEAD~1"}, wantMode: core.DiffModeCommit, wantConfig: map[string]string{"revision": "HEAD~1"}},
		{name: "range command", args: []string{"range", "main", "feature"}, wantMode: core.DiffModeRange, wantConfig: map[string]string{"base-revision": "main", "head-revision": "feature"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := viper.New()
			called := false
			cmd, err := NewRootCommand(cfg, func() error {
				called = true
				return nil
			})
			require.NoError(t, err)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(tt.args)

			err = cmd.Execute()
			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, string(tt.wantMode), cfg.GetString("diff-mode"))
			for key, want := range tt.wantConfig {
				assert.Equal(t, want, cfg.GetString(key))
			}
			for _, key := range []string{"upstream-ref", "revision", "base-revision", "head-revision"} {
				if _, ok := tt.wantConfig[key]; !ok {
					assert.Empty(t, cfg.GetString(key))
				}
			}
		})
	}
}

func TestNewRootCommandShowsModeHelpWhenRequiredArgsAreMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantMode core.DiffMode
		wantUse  string
	}{
		{name: "commit missing revision", args: []string{"commit"}, wantMode: core.DiffModeCommit, wantUse: "commit <revision>"},
		{name: "range missing revisions", args: []string{"range"}, wantMode: core.DiffModeRange, wantUse: "range <base> <head>"},
		{name: "range missing head", args: []string{"range", "main"}, wantMode: core.DiffModeRange, wantUse: "range <base> <head>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := viper.New()
			called := false
			cmd, err := NewRootCommand(cfg, func() error {
				called = true
				return nil
			})
			require.NoError(t, err)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(tt.args)

			err = cmd.Execute()
			require.NoError(t, err)
			assert.False(t, called)
			assert.Equal(t, string(tt.wantMode), cfg.GetString("diff-mode"))
			assert.Empty(t, cfg.GetString("revision"))
			assert.Empty(t, cfg.GetString("base-revision"))
			assert.Empty(t, cfg.GetString("head-revision"))
			assert.Contains(t, stdout.String(), "Usage:")
			assert.Contains(t, stdout.String(), tt.wantUse)
		})
	}
}

func TestNewRootCommandRejectsUnexpectedArgsForSimpleDiffModeCommands(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"branch", "working", "staged", "local"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			cfg := viper.New()
			cmd, err := NewRootCommand(cfg, func() error { return nil })
			require.NoError(t, err)

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{mode, "unexpected"})

			err = cmd.Execute()
			require.Error(t, err)
		})
	}
}
