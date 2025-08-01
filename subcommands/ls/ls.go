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

package ls

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os/user"
	"time"

	"github.com/PlakarKorp/kloset/objects"
	"github.com/PlakarKorp/kloset/repository"
	"github.com/PlakarKorp/kloset/snapshot"
	"github.com/PlakarKorp/kloset/snapshot/vfs"
	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/locate"
	"github.com/PlakarKorp/plakar/subcommands"
	"github.com/PlakarKorp/plakar/utils"
	"github.com/dustin/go-humanize"
)

func init() {
	subcommands.Register(func() subcommands.Subcommand { return &Ls{} }, subcommands.AgentSupport, "ls")
}

func (cmd *Ls) Parse(ctx *appcontext.AppContext, args []string) error {
	cmd.LocateOptions = locate.NewDefaultLocateOptions()

	flags := flag.NewFlagSet("ls", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [OPTIONS] [SNAPSHOT[:PATH]]\n", flags.Name())
		fmt.Fprintf(flags.Output(), "\nOPTIONS:\n")
		flags.PrintDefaults()
	}

	flags.BoolVar(&cmd.DisplayUUID, "uuid", false, "display uuid instead of short ID")
	flags.BoolVar(&cmd.Recursive, "recursive", false, "recursive listing")
	cmd.LocateOptions.InstallFlags(flags)

	flags.Parse(args)

	if flags.NArg() > 1 {
		return fmt.Errorf("too many arguments")
	}

	cmd.RepositorySecret = ctx.GetSecret()
	cmd.Path = flags.Arg(0)

	return nil
}

type Ls struct {
	subcommands.SubcommandBase

	LocateOptions *locate.LocateOptions
	Recursive     bool
	DisplayUUID   bool
	Path          string
}

func (cmd *Ls) Execute(ctx *appcontext.AppContext, repo *repository.Repository) (int, error) {
	if cmd.Path == "" {
		if err := cmd.list_snapshots(ctx, repo); err != nil {
			return 1, err
		}
		return 0, nil
	}

	if err := cmd.list_snapshot(ctx, repo, cmd.Path, cmd.Recursive); err != nil {
		return 1, err
	}
	return 0, nil
}

func (cmd *Ls) list_snapshots(ctx *appcontext.AppContext, repo *repository.Repository) error {
	cmd.LocateOptions.MaxConcurrency = ctx.MaxConcurrency
	cmd.LocateOptions.SortOrder = locate.LocateSortOrderDescending

	snapshotIDs, err := locate.LocateSnapshotIDs(repo, cmd.LocateOptions)
	if err != nil {
		return fmt.Errorf("ls: could not fetch snapshots list: %w", err)
	}

	for _, snapshotID := range snapshotIDs {
		snap, err := snapshot.Load(repo, snapshotID)
		if err != nil {
			return fmt.Errorf("ls: could not fetch snapshot: %w", err)
		}

		if !cmd.DisplayUUID {
			fmt.Fprintf(ctx.Stdout, "%s %10s%10s%10s %s\n",
				snap.Header.Timestamp.UTC().Format(time.RFC3339),
				hex.EncodeToString(snap.Header.GetIndexShortID()),
				humanize.Bytes(snap.Header.GetSource(0).Summary.Directory.Size+snap.Header.GetSource(0).Summary.Below.Size),
				snap.Header.Duration.Round(time.Second),
				utils.SanitizeText(snap.Header.GetSource(0).Importer.Directory))
		} else {
			indexID := snap.Header.GetIndexID()
			fmt.Fprintf(ctx.Stdout, "%s %3s%10s%10s %s\n",
				snap.Header.Timestamp.UTC().Format(time.RFC3339),
				hex.EncodeToString(indexID[:]),
				humanize.Bytes(snap.Header.GetSource(0).Summary.Directory.Size+snap.Header.GetSource(0).Summary.Below.Size),
				snap.Header.Duration.Round(time.Second),
				utils.SanitizeText(snap.Header.GetSource(0).Importer.Directory))
		}

		snap.Close()
	}
	return nil
}

func (cmd *Ls) list_snapshot(ctx *appcontext.AppContext, repo *repository.Repository, snapshotPath string, recursive bool) error {
	snap, pathname, err := locate.OpenSnapshotByPath(repo, snapshotPath)
	if err != nil {
		return err
	}
	defer snap.Close()

	pvfs, err := snap.Filesystem()
	if err != nil {
		return err
	}

	resolved := false
	return pvfs.WalkDir(pathname, func(path string, d *vfs.Entry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !resolved {
			// pathname might point to a symlink, so we
			// have to deal with physical vs logical path
			// in here.  This makes sure we fetch the
			// right physical path and do our logic on it.
			resolved = true
			pathname = d.Path()
		}
		if d.IsDir() && path == pathname {
			return nil
		}

		sb, err := d.Info()
		if err != nil {
			return err
		}

		var username, groupname string
		if finfo, ok := sb.Sys().(objects.FileInfo); ok {
			pwUserLookup, err := user.LookupId(fmt.Sprintf("%d", finfo.Uid()))
			username = fmt.Sprintf("%d", finfo.Uid())
			if err == nil {
				username = pwUserLookup.Username
			}

			grGroupLookup, err := user.LookupGroupId(fmt.Sprintf("%d", finfo.Gid()))
			groupname = fmt.Sprintf("%d", finfo.Gid())
			if err == nil {
				groupname = grGroupLookup.Name
			}
		}

		entryname := path
		if !recursive {
			entryname = d.Name()
		}

		var linkTarget string
		if sb.Mode()&fs.ModeSymlink != 0 {
			linkTarget = fmt.Sprintf(" -> %s", utils.SanitizeText(d.SymlinkTarget))
		}

		fmt.Fprintf(ctx.Stdout, "%s %s % 8s % 8s % 8s %s%s\n",
			sb.ModTime().UTC().Format(time.RFC3339),
			sb.Mode(),
			username,
			groupname,
			humanize.Bytes(uint64(sb.Size())),
			utils.SanitizeText(entryname),
			linkTarget)

		if !recursive && pathname != path && sb.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
}
