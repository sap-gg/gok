package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sap-gg/gok/internal/logging"
)

var cfgFile string

const (
	LogLevelKey   = "log.level"
	LogFormatKey  = "log.format"
	LogNoColorKey = "log.no_color"
)

var rootCmd = &cobra.Command{
	Use:   "gok",
	Short: "Landscape Renderer for Minecraft",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		configPath, configErr := initConfig()
		logging.Init(nil)
		if configErr != nil { // handle error after logging is initialized
			return configErr
		}
		if configPath != "" {
			log.Info().Msgf("using config file: %s", configPath)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		log.Info().Msg("Hello from gok!")
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		log.Error().Err(err).Msg("command execution failed")
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default is $HOME/.gok.yaml)")

	rootCmd.PersistentFlags().String("log-level", "info", "log level: debug, info, warn, error")
	_ = viper.BindPFlag(LogLevelKey, rootCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.PersistentFlags().String("log-format", "console", "log format: console, json")
	_ = viper.BindPFlag(LogFormatKey, rootCmd.PersistentFlags().Lookup("log-format"))

	rootCmd.PersistentFlags().Bool("no-color", false, "disable color output")
	_ = viper.BindPFlag(LogNoColorKey, rootCmd.PersistentFlags().Lookup("no-color"))

	viper.SetEnvPrefix("GOK")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv() // read in environment variables that match

	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
}

func initConfig() (string, error) {
	// reads in config file and ENV variables if set.
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// search order: current dir, $HOME, XDG config
		viper.AddConfigPath(".")

		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
		}

		config, err := os.UserConfigDir()
		if err == nil {
			viper.AddConfigPath(config + "/gok")
		}

		viper.SetConfigType("yaml")
		viper.SetConfigName(".gok")
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		var notFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &notFoundError) {
			return "", err
		}
	} else {
		return viper.ConfigFileUsed(), nil
	}

	return "", nil
}
