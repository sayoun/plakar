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

package restore

import (
	"flag"
	"fmt"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/cmd/plakar/subcommands"
	"github.com/PlakarKorp/plakar/cmd/plakar/utils"
	"github.com/PlakarKorp/plakar/repository"
	"github.com/PlakarKorp/plakar/snapshot"
	"github.com/PlakarKorp/plakar/snapshot/exporter"
)

func init() {
	subcommands.Register("restore", parse_cmd_restore)
}

func parse_cmd_restore(ctx *appcontext.AppContext, repo *repository.Repository, args []string) (subcommands.Subcommand, error) {
	var opt_name string
	var opt_category string
	var opt_environment string
	var opt_perimeter string
	var opt_job string
	var opt_tag string

	var pullPath string
	var pullRebase bool
	var opt_concurrency uint64
	var opt_quiet bool

	flags := flag.NewFlagSet("restore", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [OPTIONS] [SNAPSHOT[:PATH]]...\n", flags.Name())
		fmt.Fprintf(flags.Output(), "\nOPTIONS:\n")
		flags.PrintDefaults()
	}

	flags.Uint64Var(&opt_concurrency, "concurrency", uint64(ctx.MaxConcurrency), "maximum number of parallel tasks")
	flags.StringVar(&opt_name, "name", "", "filter by name")
	flags.StringVar(&opt_category, "category", "", "filter by category")
	flags.StringVar(&opt_environment, "environment", "", "filter by environment")
	flags.StringVar(&opt_perimeter, "perimeter", "", "filter by perimeter")
	flags.StringVar(&opt_job, "job", "", "filter by job")
	flags.StringVar(&opt_tag, "tag", "", "filter by tag")

	flags.StringVar(&pullPath, "to", ctx.CWD, "base directory where pull will restore")
	flags.BoolVar(&pullRebase, "rebase", false, "strip pathname when pulling")
	flags.BoolVar(&opt_quiet, "quiet", false, "do not print progress")
	flags.Parse(args)

	if flags.NArg() != 0 {
		if opt_name != "" || opt_category != "" || opt_environment != "" || opt_perimeter != "" || opt_job != "" || opt_tag != "" {
			ctx.GetLogger().Warn("snapshot specified, filters will be ignored")
		}
	}

	return &Restore{
		RepositoryLocation: repo.Location(),
		RepositorySecret:   ctx.GetSecret(),

		OptName:        opt_name,
		OptCategory:    opt_category,
		OptEnvironment: opt_environment,
		OptPerimeter:   opt_perimeter,
		OptJob:         opt_job,
		OptTag:         opt_tag,

		Path:        pullPath,
		Rebase:      pullRebase,
		Concurrency: opt_concurrency,
		Quiet:       opt_quiet,
		Snapshots:   flags.Args(),
	}, nil
}

type Restore struct {
	RepositoryLocation string
	RepositorySecret   []byte

	OptName        string
	OptCategory    string
	OptEnvironment string
	OptPerimeter   string
	OptJob         string
	OptTag         string

	Path        string
	Rebase      bool
	Concurrency uint64
	Quiet       bool
	Snapshots   []string
}

func (cmd *Restore) Name() string {
	return "restore"
}

func (cmd *Restore) Execute(ctx *appcontext.AppContext, repo *repository.Repository) (int, error) {
	go eventsProcessorStdio(ctx, cmd.Quiet)

	var snapshots []string
	if len(cmd.Snapshots) == 0 {
		locateOptions := utils.NewDefaultLocateOptions()
		locateOptions.MaxConcurrency = ctx.MaxConcurrency
		locateOptions.SortOrder = utils.LocateSortOrderAscending
		locateOptions.Latest = true

		if cmd.OptName != "" {
			locateOptions.Name = cmd.OptName
		}
		if cmd.OptCategory != "" {
			locateOptions.Category = cmd.OptCategory
		}
		if cmd.OptEnvironment != "" {
			locateOptions.Environment = cmd.OptEnvironment
		}
		if cmd.OptPerimeter != "" {
			locateOptions.Perimeter = cmd.OptPerimeter
		}
		if cmd.OptJob != "" {
			locateOptions.Job = cmd.OptJob
		}
		if cmd.OptTag != "" {
			locateOptions.Tag = cmd.OptTag
		}

		snapshotIDs, err := utils.LocateSnapshotIDs(repo, locateOptions)
		if err != nil {
			return 1, fmt.Errorf("ls: could not fetch snapshots list: %w", err)
		}
		for _, snapshotID := range snapshotIDs {
			snapshots = append(snapshots, fmt.Sprintf("%x:/", snapshotID))
		}
	} else {
		snapshots = cmd.Snapshots
	}

	if len(snapshots) == 0 {
		return 1, fmt.Errorf("no snapshots found")
	} else if len(snapshots) > 1 {
		return 1, fmt.Errorf("multiple snapshots found, please specify one")
	}

	var exporterInstance exporter.Exporter
	var err error
	exporterInstance, err = exporter.NewExporter(cmd.Path)
	if err != nil {
		return 1, err
	}
	defer exporterInstance.Close()

	opts := &snapshot.RestoreOptions{
		MaxConcurrency: cmd.Concurrency,
		Rebase:         cmd.Rebase,
	}

	for _, snapPath := range snapshots {
		fmt.Println("Will restore", snapPath)
		prefix, pattern := utils.ParseSnapshotID(snapPath)
		snap, err := utils.OpenSnapshotByPrefix(repo, prefix)
		if err != nil {
			return 1, err
		}
		snap.Restore(exporterInstance, exporterInstance.Root(), pattern, opts)
		snap.Close()
	}
	return 0, nil
}
