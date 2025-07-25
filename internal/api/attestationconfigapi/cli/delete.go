/*
Copyright (c) Edgeless Systems GmbH

SPDX-License-Identifier: BUSL-1.1
*/
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/edgelesssys/constellation/v2/internal/api/attestationconfigapi"
	"github.com/edgelesssys/constellation/v2/internal/api/attestationconfigapi/cli/client"
	"github.com/edgelesssys/constellation/v2/internal/attestation/variant"
	"github.com/edgelesssys/constellation/v2/internal/logger"
	"github.com/edgelesssys/constellation/v2/internal/staticupload"
	"github.com/spf13/cobra"
)

// newDeleteCmd creates the delete command.
func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete VARIANT KIND <version>",
		Short:   "Delete an object from the attestationconfig API",
		Long:    "Delete a specific object version from the config api. <version> is the name of the object to delete (without .json suffix)",
		Example: "COSIGN_PASSWORD=$CPW COSIGN_PRIVATE_KEY=$CKEY cli delete azure-sev-snp attestation-report 1.0.0",
		Args:    cobra.MatchAll(cobra.ExactArgs(3), arg0isAttestationVariant(), isValidKind(1)),
		PreRunE: envCheck,
		RunE:    runDelete,
	}

	recursivelyCmd := &cobra.Command{
		Use:     "recursive {aws-sev-snp|azure-sev-snp|azure-tdx|gcp-sev-snp}",
		Short:   "delete all objects from the API path constellation/v1/attestation/<csp>",
		Long:    "Delete all objects from the API path constellation/v1/attestation/<csp>",
		Example: "COSIGN_PASSWORD=$CPW COSIGN_PRIVATE_KEY=$CKEY cli delete recursive azure-sev-snp",
		Args:    cobra.MatchAll(cobra.ExactArgs(1), arg0isAttestationVariant()),
		RunE:    runRecursiveDelete,
	}

	cmd.AddCommand(recursivelyCmd)

	return cmd
}

func runDelete(cmd *cobra.Command, args []string) (retErr error) {
	log := logger.NewTextLogger(slog.LevelDebug).WithGroup("attestationconfigapi")

	deleteCfg, err := newDeleteConfig(cmd, ([3]string)(args[:3]))
	if err != nil {
		return fmt.Errorf("creating delete config: %w", err)
	}

	cfg := staticupload.Config{
		Bucket:         deleteCfg.bucket,
		Region:         deleteCfg.region,
		DistributionID: deleteCfg.distribution,
	}
	client, clientClose, err := client.New(cmd.Context(), cfg,
		[]byte(cosignPwd), []byte(privateKey), false, 1, log)
	if err != nil {
		return fmt.Errorf("create attestation client: %w", err)
	}
	defer func() {
		err := clientClose(cmd.Context())
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed to invalidate cache: %w", err))
		}
	}()

	return deleteEntry(cmd.Context(), client, deleteCfg)
}

func runRecursiveDelete(cmd *cobra.Command, args []string) (retErr error) {
	// newDeleteConfig expects 3 args, so we pass "all" for the version argument and "snp-report" as kind.
	args = append(args, "snp-report")
	args = append(args, "all")
	deleteCfg, err := newDeleteConfig(cmd, ([3]string)(args[:3]))
	if err != nil {
		return fmt.Errorf("creating delete config: %w", err)
	}

	log := logger.NewTextLogger(slog.LevelDebug).WithGroup("attestationconfigapi")
	client, closeFn, err := staticupload.New(cmd.Context(), staticupload.Config{
		Bucket:         deleteCfg.bucket,
		Region:         deleteCfg.region,
		DistributionID: deleteCfg.distribution,
	}, log)
	if err != nil {
		return fmt.Errorf("create static upload client: %w", err)
	}
	defer func() {
		err := closeFn(cmd.Context())
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed to close client: %w", err))
		}
	}()

	deletePath := path.Join(attestationconfigapi.AttestationURLPath, deleteCfg.variant.String())

	return deleteEntryRecursive(cmd.Context(), deletePath, client, deleteCfg)
}

type deleteConfig struct {
	variant         variant.Variant
	kind            objectKind
	version         string
	region          string
	bucket          string
	url             string
	distribution    string
	cosignPublicKey string
}

func newDeleteConfig(cmd *cobra.Command, args [3]string) (deleteConfig, error) {
	region, err := cmd.Flags().GetString("region")
	if err != nil {
		return deleteConfig{}, fmt.Errorf("getting region: %w", err)
	}

	bucket, err := cmd.Flags().GetString("bucket")
	if err != nil {
		return deleteConfig{}, fmt.Errorf("getting bucket: %w", err)
	}

	testing, err := cmd.Flags().GetBool("testing")
	if err != nil {
		return deleteConfig{}, fmt.Errorf("getting testing flag: %w", err)
	}
	apiCfg := getAPIEnvironment(testing)

	variant, err := variant.FromString(args[0])
	if err != nil {
		return deleteConfig{}, fmt.Errorf("invalid attestation variant: %q: %w", args[0], err)
	}
	kind := kindFromString(args[1])
	version := args[2]

	return deleteConfig{
		variant:         variant,
		kind:            kind,
		version:         version,
		region:          region,
		bucket:          bucket,
		url:             apiCfg.url,
		distribution:    apiCfg.distribution,
		cosignPublicKey: apiCfg.cosignPublicKey,
	}, nil
}

func deleteEntry(ctx context.Context, client *client.Client, cfg deleteConfig) error {
	if cfg.kind != attestationReport {
		return fmt.Errorf("kind %s not supported", cfg.kind)
	}

	return client.DeleteVersion(ctx, cfg.variant, cfg.version)
}

func deleteEntryRecursive(ctx context.Context, path string, client *staticupload.Client, cfg deleteConfig) error {
	resp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(cfg.bucket),
		Prefix: aws.String(path),
	})
	if err != nil {
		return err
	}

	// Delete all objects in the path.
	objIDs := make([]s3types.ObjectIdentifier, len(resp.Contents))
	for i, obj := range resp.Contents {
		objIDs[i] = s3types.ObjectIdentifier{Key: obj.Key}
	}
	if len(objIDs) > 0 {
		_, err = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(cfg.bucket),
			Delete: &s3types.Delete{
				Objects: objIDs,
				Quiet:   toPtr(true),
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func toPtr[T any](v T) *T {
	return &v
}
