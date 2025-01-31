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
	"log"
	"os/user"
	"path"
	"time"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/cmd/plakar/subcommands"
	"github.com/PlakarKorp/plakar/cmd/plakar/utils"
	"github.com/PlakarKorp/plakar/objects"
	"github.com/PlakarKorp/plakar/repository"
	"github.com/PlakarKorp/plakar/snapshot/vfs"
	"github.com/dustin/go-humanize"
)

func init() {
	subcommands.Register("ls", cmd_ls)
}

func cmd_ls(ctx *appcontext.AppContext, repo *repository.Repository, args []string) (int, error) {
	var opt_recursive bool
	var opt_tag string
	var opt_uuid bool

	flags := flag.NewFlagSet("ls", flag.ExitOnError)
	flags.BoolVar(&opt_uuid, "uuid", false, "display uuid instead of short ID")
	flags.StringVar(&opt_tag, "tag", "", "filter by tag")
	flags.BoolVar(&opt_recursive, "recursive", false, "recursive listing")
	flags.Parse(args)

	if flags.NArg() == 0 {
		list_snapshots(ctx, repo, opt_uuid, opt_tag)
		return 0, nil
	}

	if err := list_snapshot(ctx, repo, flags.Arg(0), opt_recursive); err != nil {
		log.Println("error:", err)
		return 1, err
	}
	return 0, nil
}

func list_snapshots(ctx *appcontext.AppContext, repo *repository.Repository, useUuid bool, tag string) {
	metadatas, err := utils.GetHeaders(repo, nil)
	if err != nil {
		log.Fatalf("%s: could not fetch snapshots list", flag.CommandLine.Name())
	}

	for _, metadata := range metadatas {
		if tag != "" {
			found := false
			for _, t := range metadata.Tags {
				if tag == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if !useUuid {
			fmt.Fprintf(ctx.Stdout, "%s %10s%10s%10s %s\n",
				metadata.Timestamp.UTC().Format(time.RFC3339),
				hex.EncodeToString(metadata.GetIndexShortID()),
				humanize.Bytes(metadata.Summary.Directory.Size+metadata.Summary.Below.Size),
				metadata.Duration.Round(time.Second),
				metadata.Importer.Directory)
		} else {
			indexID := metadata.GetIndexID()
			fmt.Fprintf(ctx.Stdout, "%s %3s%10s%10s %s\n",
				metadata.Timestamp.UTC().Format(time.RFC3339),
				hex.EncodeToString(indexID[:]),
				humanize.Bytes(metadata.Summary.Directory.Size+metadata.Summary.Below.Size),
				metadata.Duration.Round(time.Second),
				metadata.Importer.Directory)
		}
	}
}

func list_snapshot(ctx *appcontext.AppContext, repo *repository.Repository, snapshotPath string, recursive bool) error {
	prefix, pathname := utils.ParseSnapshotID(snapshotPath)
	pathname = path.Clean(pathname)

	snap, err := utils.OpenSnapshotByPrefix(repo, prefix)
	if err != nil {
		log.Fatalf("%s: could not fetch snapshot: %s", flag.CommandLine.Name(), err)
	}
	defer snap.Close()

	pvfs, err := snap.Filesystem()
	if err != nil {
		log.Fatal(err)
	}

	return pvfs.WalkDir(pathname, func(path string, d *vfs.Entry, err error) error {
		if err != nil {
			log.Println("error at", path, ":", err)
			return err
		}
		if path == pathname {
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

		fmt.Fprintf(ctx.Stdout, "%s %s % 8s % 8s % 8s %s\n",
			sb.ModTime().UTC().Format(time.RFC3339),
			sb.Mode(),
			username,
			groupname,
			humanize.Bytes(uint64(sb.Size())),
			entryname)

		if !recursive && pathname != path && sb.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
}
