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

package clone

import (
	"flag"
	"fmt"
	"os"
	"sync"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/cmd/plakar/subcommands"
	"github.com/PlakarKorp/plakar/objects"
	"github.com/PlakarKorp/plakar/repository"
	"github.com/PlakarKorp/plakar/storage"
	"github.com/google/uuid"
)

func init() {
	subcommands.Register("clone", cmd_clone)
}

func cmd_clone(ctx *appcontext.AppContext, repo *repository.Repository, args []string) (int, error) {
	flags := flag.NewFlagSet("clone", flag.ExitOnError)
	flags.Parse(args)

	if flags.NArg() != 2 || flags.Arg(0) != "to" {
		ctx.GetLogger().Error("usage: %s to repository", flags.Name())
		return 1, fmt.Errorf("usage: %s to repository", flags.Name())
	}

	sourceStore := repo.Store()

	configuration := sourceStore.Configuration()
	configuration.RepositoryID = uuid.Must(uuid.NewRandom())

	cloneStore, err := storage.Create(flags.Arg(1), configuration)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not create repository: %s\n", flags.Arg(1), err)
		return 1, err
	}

	packfileChecksums, err := sourceStore.GetPackfiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not get packfiles list from repository: %s\n", sourceStore.Location(), err)
		return 1, err
	}

	wg := sync.WaitGroup{}
	for _, _packfileChecksum := range packfileChecksums {
		wg.Add(1)
		go func(packfileChecksum objects.Checksum) {
			defer wg.Done()

			rd, err := sourceStore.GetPackfile(packfileChecksum)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: could not get packfile from repository: %s\n", sourceStore.Location(), err)
				return
			}

			err = cloneStore.PutPackfile(packfileChecksum, rd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: could not put packfile to repository: %s\n", cloneStore.Location(), err)
				return
			}
		}(_packfileChecksum)
	}
	wg.Wait()

	indexesChecksums, err := sourceStore.GetStates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not get paclfiles list from repository: %s\n", sourceStore.Location(), err)
		return 1, err
	}

	wg = sync.WaitGroup{}
	for _, _indexChecksum := range indexesChecksums {
		wg.Add(1)
		go func(indexChecksum objects.Checksum) {
			defer wg.Done()

			data, err := sourceStore.GetState(indexChecksum)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: could not get index from repository: %s\n", sourceStore.Location(), err)
				return
			}

			err = cloneStore.PutState(indexChecksum, data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: could not put packfile to repository: %s\n", cloneStore.Location(), err)
				return
			}
		}(_indexChecksum)
	}
	wg.Wait()

	return 0, nil
}
