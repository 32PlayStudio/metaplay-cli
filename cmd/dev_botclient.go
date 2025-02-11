/*
 * Copyright Metaplay. All rights reserved.
 */
package cmd

import (
	"fmt"
	"os"

	"github.com/metaplay/cli/internal/tui"
	"github.com/metaplay/cli/pkg/envapi"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type devBotClientOpts struct {
	flagEnvironment string

	extraArgs []string
}

func init() {
	o := devBotClientOpts{}

	cmd := &cobra.Command{
		Use:   "botclient [flags] [-- EXTRA_ARGS]",
		Short: "Run BotClient locally (againts local or remote server)",
		Run:   runCommand(&o),
		Long: trimIndent(`
			Run simulated bots against the locally running server, or a cloud environment.

			Arguments:
			- EXTRA_ARGS gets passed to the 'dotnet run' executing the BotClient project.

			Related commands:
			- 'metaplay dev server' runs the game server locally.
			- 'metaplay dev dashboard' runs the LiveOps Dashboard locally.
			- 'metaplay build botclient' builds the BotClient project.
		`),
		Example: trimIndent(`
			# Run bots against the locally running server.
			metaplay dev botclient

			# Run bots against the 'tough-falcons' cloud environment.
			metaplay dev botclient -e tough-falcons

			# Pass additional arguments to 'dotnet run' of the BotClient project.
			metaplay dev botclient -- -MaxBots=5 -MaxBotId=20
		`),
	}

	devCmd.AddCommand(cmd)

	flags := cmd.Flags()
	flags.StringVarP(&o.flagEnvironment, "environment", "e", "", "Environment (from metaplay-project.yaml) to run the bots against.")
}

func (o *devBotClientOpts) Prepare(cmd *cobra.Command, args []string) error {
	o.extraArgs = args

	return nil
}

func (o *devBotClientOpts) Run(cmd *cobra.Command) error {
	// Load project config.
	project, err := resolveProject()
	if err != nil {
		return err
	}

	// Resolve target environment flags (if specified)
	targetEnvFlags := []string{}
	if o.flagEnvironment != "" {
		// Ensure the user is logged in
		tokenSet, err := tui.RequireLoggedIn(cmd.Context())
		if err != nil {
			return err
		}

		// Resolve target environment from metaplay-project.yaml.
		envConfig, err := project.Config.FindEnvironmentConfig(o.flagEnvironment)
		if err != nil {
			return err
		}

		// Create TargetEnvironment.
		targetEnv := envapi.NewTargetEnvironment(tokenSet, envConfig.StackDomain, envConfig.HumanID)

		// Fetch environment info.
		envInfo, err := targetEnv.GetDetails()
		if err != nil {
			return err
		}

		// Construct flags for botclient.
		targetEnvFlags = []string{
			fmt.Sprintf("--Bot:ServerHost=%s", envInfo.Deployment.ServerHostname),
			"--Bot:EnableTls=true",
		}

		log.Debug().Msgf("Flags to run against environment %s: %v", o.flagEnvironment, targetEnvFlags)
	}

	// Check for .NET SDK installation and required version (based on SDK version).
	if err := checkDotnetSdkVersion(project.VersionMetadata.MinDotnetSdkVersion); err != nil {
		return err
	}

	// Resolve botclient path.
	botClientPath := project.GetBotClientDir()

	// Build the BotClient project
	if err := execChildInteractive(botClientPath, "dotnet", []string{"build"}); err != nil {
		log.Error().Msgf("Failed to build the BotClient .NET project: %s", err)
		os.Exit(1)
	}

	// Run the project without rebuilding
	botRunFlags := append([]string{"run", "--no-build"}, targetEnvFlags...)
	botRunFlags = append(botRunFlags, o.extraArgs...)
	if err := execChildInteractive(botClientPath, "dotnet", botRunFlags); err != nil {
		log.Error().Msgf("BotClient exited with error: %s", err)
		os.Exit(1)
	}

	// BotClients terminated normally
	log.Info().Msgf("BotClient terminated normally")
	return nil
}
