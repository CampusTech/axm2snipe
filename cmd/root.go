package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/CampusTech/abm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/CampusTech/axm2snipe/abmclient"
	"github.com/CampusTech/axm2snipe/config"
	"github.com/CampusTech/axm2snipe/notify"
	"github.com/CampusTech/axm2snipe/snipe"
	axmsync "github.com/CampusTech/axm2snipe/sync"
)

var (
	// Cfg is the global application configuration.
	Cfg *config.Config
	// ConfigFile is the path to the config file.
	ConfigFile string
	// Version is the application version, set from main.go.
	Version string

	verbose    bool
	debug      bool
	logFile    string
	logFormat  string
	logFileFD  *os.File // held open for the lifetime of the process
)

var log = logrus.New()

// LoadConfig loads config from YAML file with env var overrides, then applies
// CLI flag overrides for flags that were explicitly set.
func LoadConfig(cmd *cobra.Command) error {
	var err error
	Cfg, err = config.Load(ConfigFile)
	if err != nil {
		// Only error if the user explicitly specified --config
		if cmd.Flags().Changed("config") {
			return fmt.Errorf("loading config: %w", err)
		}
		// Default config file not found — create empty config
		Cfg = &config.Config{}
	}

	// CLI flag overrides
	applyBoolFlag(cmd, "dry-run", &Cfg.Sync.DryRun)
	applyBoolFlag(cmd, "force", &Cfg.Sync.Force)
	applyBoolFlag(cmd, "update-only", &Cfg.Sync.UpdateOnly)
	applyStringFlag(cmd, "cache-dir", &Cfg.Sync.CacheDir)

	// Compute effective log settings: config file provides defaults,
	// explicit CLI flags take precedence. Do not mutate the flag-backed globals.
	effectiveDebug := debug
	effectiveVerbose := verbose
	effectiveLogFile := logFile
	effectiveLogFormat := logFormat

	var unknownLogLevelMsg string
	if Cfg.Log.Level != "" && !cmd.Flags().Changed("debug") && !cmd.Flags().Changed("verbose") {
		switch strings.ToLower(Cfg.Log.Level) {
		case "debug":
			effectiveDebug = true
		case "info":
			effectiveVerbose = true
		case "warn", "warning":
			// default, no action needed
		default:
			unknownLogLevelMsg = fmt.Sprintf("Unknown log.level %q in config, using default (warn)", Cfg.Log.Level)
		}
	}
	if Cfg.Log.File != "" && !cmd.Flags().Changed("log-file") {
		effectiveLogFile = Cfg.Log.File
	}
	if Cfg.Log.Format != "" && !cmd.Flags().Changed("log-format") {
		effectiveLogFormat = Cfg.Log.Format
	}

	// Configure log level
	var level logrus.Level
	switch {
	case effectiveDebug:
		level = logrus.DebugLevel
	case effectiveVerbose:
		level = logrus.InfoLevel
	default:
		level = logrus.WarnLevel
	}
	setAllLogLevels(level)

	// Configure formatter
	var formatter logrus.Formatter
	switch strings.ToLower(effectiveLogFormat) {
	case "json":
		formatter = &logrus.JSONFormatter{}
	case "text", "":
		formatter = &logrus.TextFormatter{FullTimestamp: true}
	default:
		return fmt.Errorf("invalid log format %q: must be 'text' or 'json'", effectiveLogFormat)
	}
	setAllLogFormatters(formatter)

	// Reset outputs for each invocation, then optionally tee to a file.
	setAllLogOutputs(os.Stderr)
	if logFileFD != nil {
		_ = logFileFD.Close()
		logFileFD = nil
	}
	if effectiveLogFile != "" {
		f, err := os.OpenFile(effectiveLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		logFileFD = f
		setAllLogOutputs(io.MultiWriter(os.Stderr, f))
	}

	// Emit deferred warning after format/output are configured.
	if unknownLogLevelMsg != "" {
		log.Warn(unknownLogLevelMsg)
	}

	return nil
}

func setAllLogLevels(level logrus.Level) {
	log.SetLevel(level)
	abmclient.SetLogLevel(level)
	axmsync.SetLogLevel(level)
	notify.SetLogLevel(level)
	snipe.SetLogLevel(level)
}

func setAllLogFormatters(formatter logrus.Formatter) {
	log.SetFormatter(formatter)
	abmclient.SetLogFormatter(formatter)
	axmsync.SetLogFormatter(formatter)
	notify.SetLogFormatter(formatter)
	snipe.SetLogFormatter(formatter)
}

func setAllLogOutputs(output io.Writer) {
	log.SetOutput(output)
	abmclient.SetLogOutput(output)
	axmsync.SetLogOutput(output)
	notify.SetLogOutput(output)
	snipe.SetLogOutput(output)
}

func applyBoolFlag(cmd *cobra.Command, name string, dst *bool) {
	if cmd.Flags().Changed(name) {
		*dst, _ = cmd.Flags().GetBool(name)
	}
}

func applyStringFlag(cmd *cobra.Command, name string, dst *string) {
	if cmd.Flags().Changed(name) {
		*dst, _ = cmd.Flags().GetString(name)
	}
}

// contextWithSignal returns a context that is canceled on SIGINT/SIGTERM.
func contextWithSignal() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigCh:
			log.Infof("Received signal %v, shutting down...", sig)
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// newABMClient creates and returns a new ABM client from global config.
func newABMClient(ctx context.Context) (*abmclient.Client, error) {
	log.Infof("Connecting to Apple %s Manager...", appleManagerName())
	client, err := abmclient.NewClient(
		ctx,
		Cfg.ABM.ClientID,
		Cfg.ABM.KeyID,
		Cfg.ABM.PrivateKeyValue(),
		Cfg.ABM.OAuthScopeValue(),
		Cfg.ABM.APIBaseURLValue(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating ABM client: %w", err)
	}
	return client, nil
}

func newABMTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	assertion, err := abm.NewAssertion(ctx, Cfg.ABM.ClientID, Cfg.ABM.KeyID, Cfg.ABM.PrivateKeyValue())
	if err != nil {
		return nil, fmt.Errorf("creating ABM assertion: %w", err)
	}

	ts, err := abm.NewTokenSource(ctx, nil, Cfg.ABM.ClientID, assertion, Cfg.ABM.OAuthScopeValue())
	if err != nil {
		return nil, fmt.Errorf("creating ABM token source: %w", err)
	}

	return ts, nil
}

func appleManagerName() string {
	if Cfg != nil && Cfg.ABM.IsSchoolManager() {
		return "School"
	}
	return "Business"
}

// newSnipeClient creates and returns a new Snipe-IT client from global config.
func newSnipeClient() (*snipe.Client, error) {
	log.Info("Connecting to Snipe-IT...")
	client, err := snipe.NewClient(Cfg.SnipeIT.URL, Cfg.SnipeIT.APIKey, Cfg.Sync.RateLimit)
	if err != nil {
		return nil, fmt.Errorf("creating Snipe-IT client: %w", err)
	}
	client.DryRun = Cfg.Sync.DryRun
	return client, nil
}

// Execute builds the root command, registers subcommands, and runs.
func Execute() {
	rootCmd := &cobra.Command{
		Use:          "axm2snipe",
		Short:        "Sync devices from Apple Business/School Manager into Snipe-IT",
		Long:         "axm2snipe syncs devices from Apple Business Manager (ABM) / Apple School Manager (ASM) into Snipe-IT asset management.",
		Version:      Version,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return LoadConfig(cmd)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if logFileFD != nil {
				logFileFD.Close()
			}
		},
	}

	rootCmd.PersistentFlags().StringVar(&ConfigFile, "config", "settings.yaml", "Path to YAML config file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output (INFO level)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Debug output (DEBUG level)")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Append log output to this file (in addition to stderr)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "Log format: text or json")

	syncCmd := NewSyncCmd()
	downloadCmd := NewDownloadCmd()
	setupCmd := NewSetupCmd()
	testCmd := NewTestCmd()
	accessTokenCmd := NewAccessTokenCmd()
	requestCmd := NewRequestCmd()

	// --dry-run: sync, setup
	for _, cmd := range []*cobra.Command{syncCmd, setupCmd} {
		cmd.Flags().Bool("dry-run", false, "Simulate without making changes")
	}

	// --cache-dir: download, sync, setup
	for _, cmd := range []*cobra.Command{downloadCmd, syncCmd, setupCmd} {
		cmd.Flags().String("cache-dir", "", `Directory for cached API responses (default ".cache")`)
	}

	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(accessTokenCmd)
	rootCmd.AddCommand(requestCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
