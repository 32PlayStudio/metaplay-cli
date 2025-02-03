/*
 * Copyright Metaplay. All rights reserved.
 */
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/metaplay/cli/pkg/styles"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Build docker image for the project.
type buildDockerImageOpts struct {
	flagBuildEngine  string
	flagArchitecture string
	flagCommitID     string
	flagBuildNumber  string

	argImageName string
	extraArgs    []string
}

func init() {
	o := buildDockerImageOpts{}

	cmd := &cobra.Command{
		Use:   "docker-image [IMAGE:TAG] [flags] [-- EXTRA_ARGS]",
		Short: "Build a deployable Docker image for the cloud",
		Run:   runCommand(&o),
		Long: trimIndent(`
			Build a Docker image of your project to be deployed in the cloud.
			The built image contains both the game server (C# project) and the LiveOps
			Dashboard.

			Arguments:
			- IMAGE:TAG (optional) is the fully-qualified Docker image, e.g., 'mygame:1a27c25753' (default: '<projectID>:<timestamp>').
			- EXTRA_ARGS are passed to the Docker build as-is.

			Related commands:
			- 'metaplay deploy game-server ...' to push and deploy the game server image into a cloud environment.
			- 'metaplay image push ...' to push the built image into a target environment's registry.
		`),
		Example: trimIndent(`
			# Build Docker image locally to test that it builds.
			metaplay build docker-image mygame:364cff09

			# Build a project from another directory.
			metaplay -p ../MyProject build docker-image mygame:364cff09

			# Build docker image with commit ID and build number specified.
			metaplay build docker-image mygame:364cff09 --commit-id=1a27c25753 --build-number=123

			# Build using docker's BuildKit engine (in case buildx isn't available).
			metaplay build docker-image mygame:364cff09 --engine=buildkit

			# Build an image to be run on an arm64 machine.
			metaplay build docker-image mygame:364cff09 --platform=arm64

			# Pass extra arguments to the docker build.
			metaplay build docker-image mygame:364cff09 -- --build-arg FOO=BAR
		`),
	}

	buildCmd.AddCommand(cmd)

	flags := cmd.Flags()
	// flags.StringVarP(&o.flagImageTag, "image-tag", "t", "<project>:<timestamp>", "Docker image tag for build, eg, 'mygame:123456'")
	flags.StringVar(&o.flagBuildEngine, "engine", "", "Docker build engine to use ('buildx' or 'buildkit'), auto-detected if not specified")
	flags.StringVar(&o.flagArchitecture, "architecture", "amd64", "Architecture of build target, 'amd64' or 'arm64'")
	flags.StringVar(&o.flagCommitID, "commit-id", "", "Git commit SHA hash or similar, eg, '7d1ebc858b'")
	flags.StringVar(&o.flagBuildNumber, "build-number", "", "Number identifying this build, eg, '715'")
}

func (o *buildDockerImageOpts) Prepare(cmd *cobra.Command, args []string) error {
	// Handle image name.
	if len(args) == 0 {
		o.argImageName = "<projectID>:<timestamp>"
	} else {
		o.argImageName = args[0]
	}

	// Store extra args.
	if len(args) > 0 {
		o.extraArgs = args[1:]
	}

	return nil
}

func (o *buildDockerImageOpts) Run(cmd *cobra.Command) error {
	log.Info().Msg("")
	log.Info().Msg(styles.RenderTitle("Build Docker Image"))
	log.Info().Msg("")

	// Find & load the project config file.
	project, err := resolveProject()
	if err != nil {
		return err
	}

	// Resolve image name to use: fill in <timestamp> with current unix time
	// and <projectID> with the project's slug.
	imageName := strings.Replace(o.argImageName, "<timestamp>", fmt.Sprintf("%d", time.Now().Unix()), -1)
	imageName = strings.Replace(imageName, "<projectID>", project.config.ProjectHumanID, -1)

	if strings.HasSuffix(imageName, ":latest") {
		log.Error().Msg("Building docker image with 'latest' tag is not allowed. Use a commit hash or timestamp instead.")
		os.Exit(1)
	}

	// Log extra arguments.
	if len(o.extraArgs) > 0 {
		log.Debug().Msgf("Extra args to docker: %s", strings.Join(o.extraArgs, " "))
	}

	log.Info().Msgf("Project ID: %s", styles.RenderTechnical(project.config.ProjectHumanID))
	log.Info().Msgf("Image name: %s", styles.RenderTechnical(imageName))

	// Auto-detect git commit ID
	commitId := o.flagCommitID
	if commitId == "" {
		commitId = detectEnvVar([]string{
			"GIT_COMMIT", "GITHUB_SHA", "CI_COMMIT_SHA", "CIRCLE_SHA1", "TRAVIS_COMMIT",
			"BUILD_SOURCEVERSION", "BITBUCKET_COMMIT", "BUILD_VCS_NUMBER", "BUILDKITE_COMMIT", "DRONE_COMMIT_SHA",
			"SEMAPHORE_GIT_SHA",
		})
		if commitId != "" {
			log.Info().Msgf("%s %s %s", "Commit ID:", styles.RenderTechnical(commitId), styles.RenderMuted("(auto-detected)"))
		} else {
			commitId = "none" // default if not specified
			log.Info().Msgf("%s %s %s", "Commit ID:", styles.RenderTechnical(commitId), styles.RenderWarning("[failed to auto-detect; specify with --commit-id=<id>]"))
		}
	} else {
		log.Info().Msgf("%s %s", "Commit ID:", styles.RenderTechnical(commitId))
	}

	// Auto-detect build number
	buildNumber := o.flagBuildNumber
	if buildNumber == "" {
		buildNumber = detectEnvVar([]string{
			"BUILD_NUMBER", "GITHUB_RUN_NUMBER", "CI_PIPELINE_IID", "CIRCLE_BUILD_NUM", "TRAVIS_BUILD_NUMBER",
			"BUILD_BUILDNUMBER", "BITBUCKET_BUILD_NUMBER", "BUILDKITE_BUILD_NUMBER", "DRONE_BUILD_NUMBER",
			"SEMAPHORE_BUILD_NUMBER",
		})
		if buildNumber != "" {
			log.Info().Msgf("Build number: %s (auto-detected)", styles.RenderTechnical(buildNumber))
		} else {
			buildNumber = "none" // default if not specified
			log.Info().Msgf("Build number: %s %s", styles.RenderTechnical(buildNumber), styles.RenderWarning("[failed to auto-detect; specify with --commit-number=<number>]"))
		}
	} else {
		log.Info().Msgf("Build number: %s", styles.RenderTechnical(buildNumber))
	}

	// Resolve docker build root directory. All other paths need to be made relative to it.
	buildRootDir := project.getBuildRootDir()

	// Check that sdkRoot is a valid directory
	sdkRootPath := project.getSdkRootDir()
	if _, err := os.Stat(sdkRootPath); os.IsNotExist(err) {
		log.Error().Msgf("The Metaplay SDK directory '%s' does not exist.", sdkRootPath)
		os.Exit(2)
	}

	dockerFilePath := filepath.Join(sdkRootPath, "Dockerfile.server")
	if _, err := os.Stat(dockerFilePath); os.IsNotExist(err) {
		log.Error().Msgf("Cannot locate Dockerfile.server at %s.", dockerFilePath)
		os.Exit(2)
	}

	// Check project root directory.
	projectBackendDir := project.getBackendDir()
	if _, err := os.Stat(projectBackendDir); os.IsNotExist(err) {
		log.Error().Msgf("Unable to find project backend in '%s'.", projectBackendDir)
		os.Exit(2)
	}

	// Check SharedCode directory.
	sharedCodeDir := project.getSharedCodeDir()
	if _, err := os.Stat(sharedCodeDir); os.IsNotExist(err) {
		log.Error().Msgf("The shared code directory (%s) does not exist.", sharedCodeDir)
		os.Exit(2)
	}

	// Resolve target platform.
	validArchitectures := []string{"amd64", "arm64"}
	if !contains(validArchitectures, o.flagArchitecture) {
		log.Error().Msgf("Invalid architecture '%s'. Must be one of %v.", o.flagArchitecture, validArchitectures)
		os.Exit(2)
	}
	platform := fmt.Sprintf("linux/%s", o.flagArchitecture)
	log.Info().Msgf("Target platform: %s", styles.RenderTechnical(platform))

	// Check that docker is installed and running with a 5 second timeout
	log.Debug().Msgf("Check if docker is available")
	err = checkDockerAvailable()
	if err != nil {
		return err
	}

	// Resolve docker build engine
	log.Debug().Msg("Resolve docker build engine")
	buildEngine, err := resolveBuildEngine(o.flagBuildEngine)
	if err != nil {
		log.Error().Msgf("Failed to resolve docker build engine: %v", err)
		os.Exit(1)
	}
	log.Info().Msgf("Docker build engine: %s", styles.RenderTechnical(buildEngine))

	// Rebase paths to be relative to docker build root.
	rebasedSdkRoot, err := rebasePath(sdkRootPath, buildRootDir)
	if err != nil {
		log.Error().Msgf("Failed to resolve relative path to MetaplaySDK/ from build root: %v", err)
		os.Exit(2)
	}
	rebasedDockerFilePath, err := rebasePath(dockerFilePath, buildRootDir)
	if err != nil {
		log.Error().Msgf("Failed to resolve relative path to Dockerfile.server from build root: %v", err)
		os.Exit(2)
	}
	rebasedProjectRoot, err := rebasePath(project.relativeDir, buildRootDir)
	if err != nil {
		log.Error().Msgf("Failed to resolve relative path to project root from build root: %v", err)
		os.Exit(2)
	}

	// Rebase paths relative to project root dir (where metaplay-project.yaml is located).
	rebasedBackendDir, err := rebasePath(projectBackendDir, project.relativeDir)
	if err != nil {
		log.Error().Msgf("Failed to resolve relative path to project backend directory from project root: %v", err)
		os.Exit(2)
	}
	rebasedSharedCodeDir, err := rebasePath(sharedCodeDir, project.relativeDir)
	if err != nil {
		log.Error().Msgf("Failed to resolve relative path to project shared code directory from project root: %v", err)
		os.Exit(2)
	}

	// Silence docker's recomendation messages at end-of-build.
	var dockerEnv []string = os.Environ()
	dockerEnv = append(dockerEnv, "DOCKER_CLI_HINTS=false")

	// Handle build engine differences.
	var buildEngineArgs []string
	if buildEngine == "buildkit" {
		dockerEnv = append(dockerEnv, "DOCKER_BUILDKIT=1")
		buildEngineArgs = []string{"build"}
	} else if buildEngine == "buildx" {
		buildEngineArgs = []string{"buildx", "build", "--load"}
	} else {
		log.Panic().Msgf("Unsupported docker build engine: %s", buildEngine)
	}

	// Resolve .NET runtime version to build project for, expects '<major>.<minor>'.
	projectDotnetVersionSegments := project.config.DotnetRuntimeVersion.Segments()
	projectDotnetVersion := fmt.Sprintf("%d.%d", projectDotnetVersionSegments[0], projectDotnetVersionSegments[1])

	// Resolve final docker build invocation
	dockerArgs := append(
		buildEngineArgs,
		[]string{
			"--pull",
			"-t", imageName,
			"-f", filepath.ToSlash(rebasedDockerFilePath),
			"--platform", platform,
			"--build-arg", "SDK_ROOT=" + filepath.ToSlash(rebasedSdkRoot),
			"--build-arg", "PROJECT_ROOT=" + filepath.ToSlash(rebasedProjectRoot),
			"--build-arg", "BACKEND_DIR=" + filepath.ToSlash(rebasedBackendDir),
			"--build-arg", "SHARED_CODE_DIR=" + filepath.ToSlash(rebasedSharedCodeDir),
			"--build-arg", "DOTNET_VERSION=" + projectDotnetVersion,
			"--build-arg", fmt.Sprintf("PROJECT_ID=%s", project.config.ProjectHumanID),
			"--build-arg", fmt.Sprintf("BUILD_NUMBER=%s", buildNumber),
			"--build-arg", fmt.Sprintf("COMMIT_ID=%s", commitId),
		}...,
	)
	dockerArgs = append(dockerArgs, o.extraArgs...)
	dockerArgs = append(dockerArgs, ".")
	log.Info().Msg("")
	log.Info().Msgf(styles.RenderMuted("docker %s"), strings.Join(dockerArgs, " "))
	log.Info().Msg("")

	// Execute the docker build
	if err := executeCommand(buildRootDir, dockerEnv, "docker", dockerArgs...); err != nil {
		log.Error().Msgf("Docker build failed: %v", err)
		os.Exit(1)
	}

	log.Info().Msg("")
	log.Info().Msgf("✅ %s %s", styles.RenderSuccess("Successfully built docker image"), styles.RenderTechnical(imageName))
	log.Info().Msg("")
	log.Info().Msg("You can deploy the image to a cloud environment using:")
	log.Info().Msgf(styles.RenderTechnical("  metaplay deploy game-server ENVIRONMENT %s"), imageName)

	envsIDs := []string{}
	for _, env := range project.config.Environments {
		envsIDs = append(envsIDs, styles.RenderTechnical(env.HumanID))
	}
	log.Info().Msgf("Available environments: %s", strings.Join(envsIDs, ", "))

	return nil
}

func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func detectEnvVar(keys []string) string {
	for _, key := range keys {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
	}
	return ""
}

func resolveBuildEngine(engine string) (string, error) {
	validBuildEngines := []string{"buildx", "buildkit"}

	// If not specified, auto-detect
	if engine == "" {
		// Bitbucket doesn't support buildx, fall back to buildkit
		if _, exists := os.LookupEnv("BITBUCKET_PIPELINE_UUID"); exists {
			return "buildkit", nil
		}
		return "buildx", nil
	}

	// Check validity if specified
	for _, validEngine := range validBuildEngines {
		if engine == validEngine {
			return engine, nil
		}
	}

	return "", fmt.Errorf("invalid Docker build engine '%s', must be one of: %v", engine, validBuildEngines)
}

func checkCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %v", err)
	}
	return nil
}

// executeCommand runs a command with the given arguments in the specified working directory.
func executeCommand(workingDir string, env []string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = workingDir // Set the working directory
	return cmd.Run()
}

// rebasePath calculates a new path for `targetPath` such that it is relative
// to `newBaseDir` instead of current working directory.
func rebasePath(targetPath, newBaseDir string) (string, error) {
	// Resolve absolute directories of new base path & target path.
	absNewBaseDir, err := filepath.Abs(newBaseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute base path: %w", err)
	}
	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute target path: %w", err)
	}

	// Compute the relative path to the new base.
	relativePath, err := filepath.Rel(absNewBaseDir, absTargetPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve relative path: %w", err)
	}

	// log.Debug().Msgf("Rebase %s onto %s -> %s", targetPath, newBaseDir, relativePath)
	// log.Debug().Msgf("  absNewBaseDir=%s, absTargetPath=%s", absNewBaseDir, absTargetPath)

	return relativePath, nil
}

// Check if docker is available and running. Uses a short timeout as 'docker' invocation
// can sometimes hang indefinitely.
func checkDockerAvailable() error {
	done := make(chan error)
	go func() {
		done <- checkCommand("docker", "info")
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("docker is not available: %w. Ensure docker is installed and working.", err)
		}
	case <-time.After(5 * time.Second):
		return fmt.Errorf("docker availablity check timed out. Ensure docker is running and responsive.")
	}

	return nil
}
