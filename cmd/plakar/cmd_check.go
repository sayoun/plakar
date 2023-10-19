/*
 * Copyright (c) 2021 Gilles Chehade <gilles@poolp.org>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package main

import (
	"flag"
	"log"

	"github.com/PlakarLabs/plakar/logger"
	"github.com/PlakarLabs/plakar/storage"
)

func init() {
	registerCommand("check", cmd_check)
}

func cmd_check(ctx Plakar, repository *storage.Repository, args []string) int {
	var enableFastCheck bool
	var opt_progress bool

	flags := flag.NewFlagSet("check", flag.ExitOnError)
	flags.BoolVar(&enableFastCheck, "fast", false, "enable fast checking (no checksum verification)")
	flags.BoolVar(&opt_progress, "progress", false, "display progress bar")
	flags.Parse(args)

	snapshots, err := getSnapshots(repository, flags.Args())
	if err != nil {
		log.Fatal(err)
	}

	if len(snapshots) == 0 {
		log.Fatal("check needs at least one snapshot ID")
	}

	failures := false
	for offset, snapshot := range snapshots {
		_, pattern := parseSnapshotID(flags.Args()[offset])

		ok, err := snapshot.Check(pattern, enableFastCheck)
		if err != nil {
			logger.Warn("%s", err)
		}

		if !ok {
			failures = true
		}
	}

	if failures {
		return 1
	}
	return 0
}
