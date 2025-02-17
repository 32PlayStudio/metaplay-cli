/*
 * Copyright Metaplay. All rights reserved.
 */
package cmd

import (
	"os"

	"github.com/metaplay/cli/internal/tui"
	"github.com/metaplay/cli/pkg/envapi"
	"github.com/metaplay/cli/pkg/helmutil"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Remove the Metaplay game server deployment from target environment.
type removeGameServerOpts struct {
	UsePositionalArgs

	argEnvironment string
}

func init() {
	o := removeGameServerOpts{}

	args := o.Arguments()
	args.AddStringArgumentOpt(&o.argEnvironment, "ENVIRONMENT", "Target environment name or id, eg, 'tough-falcons'.")

	cmd := &cobra.Command{
		Use:     "game-server ENVIRONMENT [flags]",
		Aliases: []string{"server"},
		Short:   "Remove the game server deployment from the target environment",
		Run:     runCommand(&o),
		Long: renderLong(&o, `
			PREVIEW: This command is in preview and subject to change! It also still lacks some
			key functionality.

			Remove the BotClient deployment from the target environment.

			{Arguments}
		`),
		Example: trimIndent(`
			# Remove botclients from environment tough-falcons.
			metaplay remove botclients tough-falcons
		`),
	}

	removeCmd.AddCommand(cmd)
}

func (o *removeGameServerOpts) Prepare(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *removeGameServerOpts) Run(cmd *cobra.Command) error {
	// Try to resolve the project & auth provider.
	project, err := tryResolveProject()
	if err != nil {
		return err
	}
	authProvider := getAuthProvider(project)

	// Ensure the user is logged in.
	tokenSet, err := tui.RequireLoggedIn(cmd.Context(), authProvider)
	if err != nil {
		return err
	}

	// Resolve environment.
	envConfig, err := resolveEnvironment(project, tokenSet, o.argEnvironment)
	if err != nil {
		return err
	}

	// Create TargetEnvironment.
	targetEnv := envapi.NewTargetEnvironment(tokenSet, envConfig.StackDomain, envConfig.HumanID)

	// Get kubeconfig to access the environment.
	kubeconfigPayload, err := targetEnv.GetKubeConfigWithEmbeddedCredentials()
	log.Debug().Msgf("Resolved kubeconfig to access environment")

	// Configure Helm.
	actionConfig, err := helmutil.NewActionConfig(kubeconfigPayload, envConfig.GetKubernetesNamespace())
	if err != nil {
		log.Error().Msgf("Failed to initialize Helm config: %v", err)
		os.Exit(1)
	}

	// Resolve all deployed game server Helm releases.
	helmReleases, err := helmutil.HelmListReleases(actionConfig, metaplayGameServerChartName)
	if len(helmReleases) == 0 {
		log.Error().Msgf("No game server deployment found")
		os.Exit(0)
	}

	// Uninstall all Helm releases (multiple releases should not happen but are possible).
	for _, release := range helmReleases {
		log.Info().Msgf("Remove release %s...", release.Name)

		err := helmutil.UninstallRelease(actionConfig, release)
		if err != nil {
			log.Error().Msgf("Failed to uninstall Helm release %s: %v", release.Name, err)
			os.Exit(1)
		}
	}

	log.Info().Msgf("Successfully removed game server deployment")
	return nil
}
