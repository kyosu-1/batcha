package batcha

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// CLI builds and returns the root cobra command.
func CLI() *cobra.Command {
	root := &cobra.Command{
		Use:   "batcha",
		Short: "Declarative AWS Batch Job Definition deployment tool",
	}

	root.AddCommand(
		initCmd(),
		registerCmd(),
		renderCmd(),
		diffCmd(),
		statusCmd(),
		runCmd(),
		versionCmd(),
	)
	return root
}

func initCmd() *cobra.Command {
	var (
		jobDefName string
		region     string
		outputDir  string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate config and job definition from an existing AWS Batch definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Init(cmd.Context(), InitOption{
				JobDefinitionName: jobDefName,
				Region:            region,
				OutputDir:         outputDir,
			})
		},
	}
	cmd.Flags().StringVar(&jobDefName, "job-definition-name", "", "Name of the AWS Batch job definition to fetch")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (falls back to AWS_REGION)")
	cmd.Flags().StringVar(&outputDir, "output", ".", "Output directory for generated files")
	_ = cmd.MarkFlagRequired("job-definition-name")
	return cmd
}

func registerCmd() *cobra.Command {
	var (
		configPath string
		dryRun     bool
	)
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register an AWS Batch Job Definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			app, err := New(ctx, configPath)
			if err != nil {
				return err
			}
			return app.Register(ctx, RegisterOption{DryRun: dryRun})
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config YAML file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Render template and print JSON without registering")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func renderCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Render and print the job definition template",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			app, err := New(ctx, configPath)
			if err != nil {
				return err
			}
			return app.Render(ctx)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config YAML file")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func diffCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show differences between local and remote job definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			app, err := New(ctx, configPath)
			if err != nil {
				return err
			}
			return app.Diff(ctx)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config YAML file")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func statusCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current status of the job definition on AWS",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			app, err := New(ctx, configPath)
			if err != nil {
				return err
			}
			return app.Status(ctx)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config YAML file")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func runCmd() *cobra.Command {
	var (
		configPath string
		jobQueue   string
		jobName    string
		params     []string
		wait       bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Submit a job using the latest active job definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			app, err := New(ctx, configPath)
			if err != nil {
				return err
			}
			paramMap := make(map[string]string)
			for _, p := range params {
				k, v, ok := strings.Cut(p, "=")
				if !ok {
					return fmt.Errorf("invalid parameter format %q, expected key=value", p)
				}
				paramMap[k] = v
			}
			return app.Run(ctx, RunOption{
				JobQueue:   jobQueue,
				JobName:    jobName,
				Parameters: paramMap,
				Wait:       wait,
			})
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config YAML file")
	cmd.Flags().StringVar(&jobQueue, "job-queue", "", "AWS Batch job queue name")
	cmd.Flags().StringVar(&jobName, "job-name", "", "Job name (defaults to job definition name)")
	cmd.Flags().StringArrayVar(&params, "parameter", nil, "Parameter overrides (key=value, repeatable)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the job to complete")
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("job-queue")
	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("batcha %s\n", Version)
		},
	}
}

// Run executes the CLI with signal handling.
func Run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd := CLI()
	cmd.SetContext(ctx)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	if err := cmd.ExecuteContext(ctx); err != nil {
		if _, ok := err.(*DiffError); ok {
			return 1
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	return 0
}
