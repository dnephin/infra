package cmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/lensesio/tableprinter"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/infrahq/infra/api"
	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/internal/cmd/cliopts"
	"github.com/infrahq/infra/internal/logging"
)

// Run the main CLI command with the given args. The args should not contain
// the name of the binary (ex: os.Args[1:]).
func Run(ctx context.Context, args ...string) error {
	cli := newCLI(ctx)
	cmd := NewRootCmd(cli)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func mustBeLoggedIn() error {
	if _, ok := os.LookupEnv("INFRA_ACCESS_KEY"); ok {
		// user doesn't need to log in if supplying an access key
		return nil
	}

	config, err := currentHostConfig()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return Error{Message: "Not logged in; run 'infra login' before running this command"}
		}
		return fmt.Errorf("getting host config: %w", err)
	}

	// Check expired before checking isLoggedin, since if we check isLoggedIn first, we will never know if it's expired
	if config.isExpired() {
		return Error{Message: "Session expired; run 'infra login' to start a new session"}
	}

	if !config.isLoggedIn() {
		return Error{Message: "Not logged in; run 'infra login' before running this command"}
	}
	return nil
}

func printTable(data interface{}, out io.Writer) {
	table := tableprinter.New(out)

	table.HeaderAlignment = tableprinter.AlignLeft
	table.AutoWrapText = false
	table.DefaultAlignment = tableprinter.AlignLeft
	table.CenterSeparator = ""
	table.ColumnSeparator = ""
	table.RowSeparator = ""
	table.HeaderLine = false
	table.BorderBottom = false
	table.BorderLeft = false
	table.BorderRight = false
	table.BorderTop = false
	table.Print(data)
}

// Creates a new API Client from the current config
func defaultAPIClient() (*api.Client, error) {
	config, err := currentHostConfig()
	if err != nil {
		return nil, err
	}

	server := config.Host
	var accessKey string
	if !config.isExpired() {
		accessKey = config.AccessKey
	}

	if envAccessKey, ok := os.LookupEnv("INFRA_ACCESS_KEY"); ok {
		accessKey = envAccessKey
	}

	if len(accessKey) == 0 {
		if config.isExpired() {
			return nil, Error{Message: "Access key is expired, please `infra login` again", OriginalError: ErrAccessKeyExpired}
		}
		return nil, Error{Message: "Missing access key, must `infra login` or set INFRA_ACCESS_KEY in your environment", OriginalError: ErrAccessKeyMissing}
	}

	if envServer, ok := os.LookupEnv("INFRA_SERVER"); ok {
		server = envServer
	}

	return apiClient(server, accessKey, httpTransportForHostConfig(config)), nil
}

func apiClient(host string, accessKey string, transport *http.Transport) *api.Client {
	return &api.Client{
		Name:      "cli",
		Version:   internal.Version,
		URL:       "https://" + host,
		AccessKey: accessKey,
		HTTP: http.Client{
			Timeout:   60 * time.Second,
			Transport: transport,
		},
		OnUnauthorized: logoutCurrent,
	}
}

func logoutCurrent() {
	config, err := readConfig()
	if err != nil {
		logging.Debugf("logging out: read config: %s", err)
		return
	}

	var host *ClientHostConfig
	for i := range config.Hosts {
		if config.Hosts[i].Current {
			host = &config.Hosts[i]
			break
		}
	}

	if host == nil {
		return
	}

	host.AccessKey = ""
	host.Expires = api.Time{}
	host.UserID = 0
	host.Name = ""

	if err := writeConfig(config); err != nil {
		logging.Debugf("logging out: write config: %s", err)
		return
	}
}

func httpTransportForHostConfig(config *ClientHostConfig) *http.Transport {
	pool, err := x509.SystemCertPool()
	if err != nil {
		logging.Warnf("Failed to load trusted certificates from system: %v", err)
		pool = x509.NewCertPool()
	}

	if config.TrustedCertificate != "" {
		ok := pool.AppendCertsFromPEM([]byte(config.TrustedCertificate))
		if !ok {
			logging.Warnf("Failed to read trusted certificates for server")
		}
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			//nolint:gosec // We may purposely set insecureskipverify via a flag
			InsecureSkipVerify: config.SkipTLSVerify,
			RootCAs:            pool,
		},
	}
}

const (
	groupCore       = "group-core"
	groupManagement = "group-management"
	groupOther      = "group-other"
)

func NewRootCmd(cli *CLI) *cobra.Command {
	cobra.EnableCommandSorting = false

	rootCmd := &cobra.Command{
		Use:               "infra",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return rootPreRun(cmd.Flags())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.AddGroup(
		&cobra.Group{
			ID:    groupCore,
			Title: "Core commands:",
		},
		&cobra.Group{
			ID:    groupManagement,
			Title: "Management commands:",
		},
		&cobra.Group{
			ID:    groupOther,
			Title: "Other commands:",
		})

	rootCmd.AddCommand(
		// Core commands
		newLoginCmd(cli),
		newLogoutCmd(cli),
		newListCmd(cli),
		newUseCmd(cli),

		// Management commands
		newDestinationsCmd(cli),
		newGrantsCmd(cli),
		newUsersCmd(cli),
		newGroupsCmd(cli),
		newKeysCmd(cli),
		newProvidersCmd(cli),

		// Other commands
		newInfoCmd(cli),
		newVersionCmd(cli),

		// Hidden commands
		newTokensCmd(cli),
		newServerCmd(),
		newConnectorCmd(),
		newAgentCmd())

	rootCmd.PersistentFlags().String("log-level", "info", "Show logs when running the command [error, warn, info, debug]")
	rootCmd.PersistentFlags().Bool("help", false, "Display help")

	rootCmd.SetHelpCommandGroupID(groupOther)
	rootCmd.AddCommand(newAboutCmd())
	rootCmd.AddCommand(newCompletionsCmd())
	rootCmd.SetUsageTemplate(usageTemplate())
	return rootCmd
}

func rootPreRun(flags *pflag.FlagSet) error {
	if err := cliopts.DefaultsFromEnv("INFRA", flags); err != nil {
		return err
	}
	logLevel, err := flags.GetString("log-level")
	if err != nil {
		return err
	}
	if err := logging.SetLevel(logLevel); err != nil {
		return err
	}
	return nil
}

func addNonInteractiveFlag(flags *pflag.FlagSet, bind *bool) {
	isNonInteractiveMode := os.Stdin == nil || !term.IsTerminal(int(os.Stdin.Fd()))
	flags.BoolVar(bind, "non-interactive", isNonInteractiveMode, "Disable all prompts for input")
}

func addFormatFlag(flags *pflag.FlagSet, bind *string) {
	flags.StringVar(bind, "format", "", "Output format [json|yaml]")
}

func usageTemplate() string {
	return `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{end}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
}
