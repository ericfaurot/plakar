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
	"fmt"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/poolpOrg/plakar/logger"
	"github.com/poolpOrg/plakar/snapshot"
	"github.com/poolpOrg/plakar/storage"
)

func cmd_stats(ctx Plakar, args []string) int {
	var enableFastCheck bool

	flags := flag.NewFlagSet("check", flag.ExitOnError)
	flags.BoolVar(&enableFastCheck, "fast", false, "enable fast checking (no checksum verification)")
	flags.Parse(args)

	if flags.NArg() == 0 {
		return stats_plakar(ctx.Store())
	}
	return 1
}

func stats_plakar(store *storage.Store) int {
	indexes, err := store.GetIndexes()
	if err != nil {
		logger.Warn("%s", err)
		return 1
	}

	muChunks := sync.Mutex{}
	chunks := make(map[string]uint16)

	muObjects := sync.Mutex{}
	objects := make(map[string]uint16)

	errors := 0

	chunksSize := uint64(0)
	dedupedChunksSize := uint64(0)
	objectsSize := uint64(0)
	dedupedObjectsSize := uint64(0)
	for _, index := range indexes {
		snap, err := snapshot.Load(store, index)
		if err != nil {
			logger.Warn("%s", err)
			errors++
			continue
		}

		for chunkChecksum := range snap.Chunks {
			muChunks.Lock()
			if _, exists := chunks[chunkChecksum]; !exists {
				chunks[chunkChecksum] = 0
			}
			chunks[chunkChecksum] = chunks[chunkChecksum] + 1
			muChunks.Unlock()
		}

		for objectChecksum := range snap.Objects {
			muObjects.Lock()
			if _, exists := objects[objectChecksum]; !exists {
				objects[objectChecksum] = 0
			}
			objects[objectChecksum] = objects[objectChecksum] + 1
			muObjects.Unlock()
		}
	}

	chunksChecksums, err := store.GetChunks()
	if err != nil {
		logger.Warn("%s", err)
		errors++
		return 1
	}

	objectsChecksums, err := store.GetObjects()
	if err != nil {
		logger.Warn("%s", err)
		errors++
		return 1
	}

	for _, checksum := range chunksChecksums {
		if _, exists := chunks[checksum]; !exists {
			logger.Warn("orphan chunk: %s", checksum)
			errors++
		}
	}

	for _, checksum := range objectsChecksums {
		if _, exists := objects[checksum]; !exists {
			logger.Warn("orphan object: %s", checksum)
			errors++
		}
	}

	for chunkChecksum, count := range chunks {
		refCount, err := store.GetChunkRefCount(chunkChecksum)
		if err != nil {
			logger.Warn("%s", err)
			errors++
		} else if refCount != uint64(count) {
			logger.Warn("invalid references count: %s", chunkChecksum)
			errors++
		}
		size, err := store.GetChunkSize(chunkChecksum)
		if err != nil {
			logger.Warn("%s", err)
			errors++
		} else {
			chunksSize += size
			dedupedChunksSize += (size * uint64(chunks[chunkChecksum]))
		}
	}

	for objectChecksum, count := range objects {
		refCount, err := store.GetObjectRefCount(objectChecksum)
		if err != nil {
			logger.Warn("%s", err)
			errors++
		} else if refCount != uint64(count) {
			logger.Warn("invalid references count: %s", objectChecksum)
			errors++
		}
		size, err := store.GetObjectSize(objectChecksum)
		if err != nil {
			logger.Warn("%s", err)
			errors++
		} else {
			objectsSize += size
			dedupedObjectsSize += (size * uint64(chunks[objectChecksum]))
		}
	}

	fmt.Println("Snapshots:", len(indexes))
	fmt.Printf("Chunks: %d (stored size: %s, real: %s, saved: %.02f%%)\n", len(chunks), humanize.Bytes(chunksSize), humanize.Bytes(dedupedChunksSize), float64(dedupedChunksSize-chunksSize)/float64(dedupedChunksSize)*100)
	fmt.Printf("Objects: %d (stored size: %s, real: %s, saved: %.02f%%)\n", len(objects), humanize.Bytes(objectsSize), humanize.Bytes(dedupedObjectsSize), float64(dedupedObjectsSize-objectsSize)/float64(dedupedObjectsSize)*100)

	return 0
}
