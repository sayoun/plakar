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

package mount

import (
	"flag"
	"fmt"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/cmd/plakar/subcommands"
)

func init() {
	subcommands.Register("mount", parse_cmd_mount)
}

func parse_cmd_mount(ctx *appcontext.AppContext, args []string) (subcommands.Subcommand, error) {
	flags := flag.NewFlagSet("mount", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s PATH\n", flags.Name())
	}
	flags.Parse(args)

	if flags.NArg() != 1 {
		ctx.GetLogger().Error("need mountpoint")
		return nil, fmt.Errorf("need mountpoint")
	}
	return &Mount{
		RepositorySecret: ctx.GetSecret(),
		Mountpoint:       flags.Arg(0),
	}, nil
}

type Mount struct {
	RepositorySecret []byte

	Mountpoint string
}

func (cmd *Mount) Name() string {
	return "mount"
}
