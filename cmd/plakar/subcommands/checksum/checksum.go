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

package checksum

import (
	"flag"
	"fmt"
	"io"
	"path"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/cmd/plakar/subcommands"
	"github.com/PlakarKorp/plakar/cmd/plakar/utils"
	"github.com/PlakarKorp/plakar/repository"
	"github.com/PlakarKorp/plakar/snapshot"
	"github.com/PlakarKorp/plakar/snapshot/vfs"
)

func init() {
	subcommands.Register("checksum", cmd_checksum)
}

func cmd_checksum(ctx *appcontext.AppContext, repo *repository.Repository, args []string) (int, error) {
	var enableFastChecksum bool

	flags := flag.NewFlagSet("checksum", flag.ExitOnError)
	flags.BoolVar(&enableFastChecksum, "fast", false, "enable fast checksum (return recorded checksum)")

	flags.Parse(args)

	if flags.NArg() == 0 {
		ctx.GetLogger().Error("%s: at least one parameter is required", flags.Name())
		return 1, fmt.Errorf("at least one parameter is required")
	}

	snapshots, err := utils.GetSnapshots(repo, flags.Args())
	if err != nil {
		ctx.GetLogger().Error("%s: could not obtain snapshots list: %s", flags.Name(), err)
		return 1, err
	}

	errors := 0
	for offset, snap := range snapshots {
		defer snap.Close()

		fs, err := snap.Filesystem()
		if err != nil {
			continue
		}

		_, pathname := utils.ParseSnapshotID(flags.Args()[offset])
		if pathname == "" {
			ctx.GetLogger().Error("%s: missing filename for snapshot %x", flags.Name(), snap.Header.GetIndexShortID())
			errors++
			continue
		}

		displayChecksums(ctx, fs, repo, snap, pathname, enableFastChecksum)

	}

	return 0, nil
}

func displayChecksums(ctx *appcontext.AppContext, fs *vfs.Filesystem, repo *repository.Repository, snap *snapshot.Snapshot, pathname string, fastcheck bool) error {
	fsinfo, err := fs.GetEntry(pathname)
	if err != nil {
		return err
	}

	if fsinfo.Stat().Mode().IsDir() {
		iter, err := fsinfo.Getdents(fs)
		if err != nil {
			return err
		}
		for child := range iter {
			if err := displayChecksums(ctx, fs, repo, snap, path.Join(pathname, child.Stat().Name()), fastcheck); err != nil {
				return err
			}
		}
		return nil
	}
	if !fsinfo.Stat().Mode().IsRegular() {
		return nil
	}

	object, err := snap.LookupObject(fsinfo.Object.Checksum)
	if err != nil {
		return err
	}

	checksum := object.Checksum
	if !fastcheck {
		rd, err := snap.NewReader(pathname)
		if err != nil {
			return err
		}
		defer rd.Close()

		hasher := repo.Hasher()
		if _, err := io.Copy(hasher, rd); err != nil {
			return err
		}
	}
	fmt.Fprintf(ctx.Stdout, "SHA256 (%s) = %x\n", pathname, checksum)
	return nil
}
