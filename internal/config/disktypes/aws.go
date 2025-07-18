/*
Copyright (c) Edgeless Systems GmbH

SPDX-License-Identifier: BUSL-1.1
*/

package disktypes

// AWSDiskTypes is derived from:
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ebs-volume-types.html (Last updated: August 1st, 2023).
var AWSDiskTypes = []string{
	"gp2",
	"gp3",
	"st1",
	"sc1",
	"io1",
}
