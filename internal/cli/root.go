package cli

import (
	"fmt"
	"os"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/output"
	"github.com/ganjar/ecorouter/internal/version"
	"github.com/spf13/cobra"
)

var (
	cfgPath  string
	jsonOut  bool
	noColor  bool
	quiet    bool
	verbose  bool
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "eco",
		Short:         "EcoRouter — self-hosted LLM router (HTTPS + Bearer token)",
		Long:          `EcoRouter is a self-hosted reverse proxy for LLM API traffic.
Manage providers, routes, tokens, and savers from the terminal.
The daemon binds loopback-only; expose via Caddy (TLS) on the host.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			output.JSON = jsonOut
			output.NoColor = noColor
			output.Quiet = quiet
			output.Verbose = verbose
		},
	}

	root.PersistentFlags().StringVar(&cfgPath, "config", "", "config file (default: ~/.ecorouter/config.toml or /etc/ecorouter/config.toml)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "minimal output")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	root.AddCommand(
		newInitCmd(),
		newDoctorCmd(),
		newStatusCmd(),
		newVersionCmd(),
		newCompletionCmd(),
		newConfigCmd(),
		newProviderCmd(),
		newModelsCmd(),
		newRouteCmd(),
		newUseCmd(),
		newTokenCmd(),
		newAccessCmd(),
		newSaverCmd(),
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newLogsCmd(),
		newActivityCmd(),
		newStatsCmd(),
		newAuditCmd(),
	)

	return root
}

func loadConfig() (*config.Config, error) {
	path := cfgPath
	if path == "" {
		path = config.DefaultConfigPath()
	}
	return config.Load(path)
}

func requireConfig() (*config.Config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, exitErr(3, err)
	}
	return cfg, nil
}

func exitErr(code int, err error) error {
	return &exitError{code: code, err: err}
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Code() int     { return e.code }

func Execute() {
	root := NewRoot()
	if err := root.Execute(); err != nil {
		code := 1
		if ee, ok := err.(*exitError); ok {
			code = ee.code
		}
		if output.JSON {
			output.FailJSON("error", err.Error(), "")
		} else {
			output.Fail(err.Error(), "")
		}
		os.Exit(code)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, build date, Go version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if output.JSON {
				return output.PrintJSON(version.Detail())
			}
			fmt.Println(version.String())
			d := version.Detail()
			fmt.Printf("Go: %s\n", d["go_version"])
			return nil
		},
	}
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish]",
		Short:     "Generate shell completion script",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			default:
				return exitErr(2, fmt.Errorf("unsupported shell %q", args[0]))
			}
		},
	}
}
