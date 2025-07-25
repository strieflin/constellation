/*
Copyright (c) Edgeless Systems GmbH

SPDX-License-Identifier: BUSL-1.1
*/

/*
This package provides a CLI tool to interact with the Constellation versions API.

The tool can be used to request information from the API, but also for admin tasks.
All actions require an authentication against AWS with the common permissions.
Andministrative tasks like adding or removing versions require further AWS permissions
as well as permissions to GCP and Azure.

The CLI is commonly used in the CI pipeline. Most actions shouldn't be executed manually
by a developer. Notice that there is no synchronization on API operations.
*/
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/edgelesssys/constellation/v2/internal/constants"
	"github.com/spf13/cobra"
)

func main() {
	if err := execute(); err != nil {
		os.Exit(1)
	}
}

func execute() error {
	rootCmd := newRootCmd()
	ctx, cancel := signalContext(context.Background(), os.Interrupt)
	defer cancel()
	return rootCmd.ExecuteContext(ctx)
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:              "versionsapi",
		Short:            "Interact with the Constellation versions API",
		Long:             "Interact with the Constellation versions API.",
		PersistentPreRun: preRunRoot,
	}

	rootCmd.SetOut(os.Stdout)

	rootCmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().String("region", "eu-central-1", "AWS region of the API S3 bucket")
	rootCmd.PersistentFlags().String("bucket", "cdn-constellation-backend", "S3 bucket name of the API")
	rootCmd.PersistentFlags().String("distribution-id", constants.CDNDefaultDistributionID, "CloudFront distribution ID of the API")

	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newLatestCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newRemoveCmd())

	return rootCmd
}

// signalContext returns a context that is canceled on the handed signal.
// The signal isn't watched after its first occurrence. Call the cancel
// function to ensure the internal goroutine is stopped and the signal isn't
// watched any longer.
func signalContext(ctx context.Context, sig os.Signal) (context.Context, context.CancelFunc) {
	sigCtx, stop := signal.NotifyContext(ctx, sig)
	done := make(chan struct{}, 1)
	stopDone := make(chan struct{}, 1)

	go func() {
		defer func() { stopDone <- struct{}{} }()
		defer stop()
		select {
		case <-sigCtx.Done():
			fmt.Println(" Signal caught. Press ctrl+c again to terminate the program immediately.")
		case <-done:
		}
	}()

	cancelFunc := func() {
		done <- struct{}{}
		<-stopDone
	}

	return sigCtx, cancelFunc
}

func preRunRoot(cmd *cobra.Command, _ []string) {
	cmd.SilenceUsage = true
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
