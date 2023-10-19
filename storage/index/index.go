/*
 * Copyright (c) 2023 Gilles Chehade <gilles@poolp.org>
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

package index

import (
	"sync"
	"time"

	"github.com/PlakarLabs/plakar/logger"
	"github.com/PlakarLabs/plakar/profiler"
	"github.com/vmihailenco/msgpack/v5"
)

type Index struct {
	muChecksums      sync.Mutex
	checksumID       uint32
	Checksums        map[[32]byte]uint32
	checksumsInverse map[uint32][32]byte

	muChunks sync.Mutex
	Chunks   map[uint32]uint32

	muObjects sync.Mutex
	Objects   map[uint32]uint32
}

func NewIndex() *Index {
	return &Index{
		Checksums: make(map[[32]byte]uint32),
		Chunks:    make(map[uint32]uint32),
		Objects:   make(map[uint32]uint32),
	}
}

func NewFromBytes(serialized []byte) (*Index, error) {
	t0 := time.Now()
	defer func() {
		profiler.RecordEvent("index.NewFromBytes", time.Since(t0))
		logger.Trace("index", "NewFromBytes(...): %s", time.Since(t0))
	}()

	var index Index
	if err := msgpack.Unmarshal(serialized, &index); err != nil {
		return nil, err
	}

	index.checksumsInverse = make(map[uint32][32]byte)
	for checksum, checksumID := range index.Checksums {
		index.checksumsInverse[checksumID] = checksum
	}

	return &index, nil
}

func (index *Index) Serialize() ([]byte, error) {
	t0 := time.Now()
	defer func() {
		profiler.RecordEvent("index.Serialize", time.Since(t0))
		logger.Trace("index", "Serialize(): %s", time.Since(t0))
	}()

	serialized, err := msgpack.Marshal(index)
	if err != nil {
		return nil, err
	}
	return serialized, nil
}

func (index *Index) addChecksum(checksum [32]byte) uint32 {
	index.muChecksums.Lock()
	defer index.muChecksums.Unlock()

	if checksumID, exists := index.Checksums[checksum]; !exists {
		index.Checksums[checksum] = index.checksumID
		index.checksumsInverse[index.checksumID] = checksum
		checksumID = index.checksumID
		index.checksumID++
		return checksumID
	} else {
		return checksumID
	}
}

func (index *Index) LookupChecksum(checksumID uint32) [32]byte {
	index.muChecksums.Lock()
	defer index.muChecksums.Unlock()

	checksum, exists := index.checksumsInverse[checksumID]
	if !exists {
		panic("checksum not found")
	}
	return checksum
}

func (index *Index) Merge(deltaIndex *Index) {
	index.muChecksums.Lock()
	defer index.muChecksums.Unlock()

	for deltaChecksum := range deltaIndex.Checksums {
		if _, exists := index.Checksums[deltaChecksum]; !exists {
			index.addChecksum(deltaChecksum)
		}
	}

	for deltaChunkChecksumID, deltaPackfileChecksumID := range deltaIndex.Chunks {
		index.SetPackfileForChunk(deltaIndex.LookupChecksum(deltaChunkChecksumID),
			deltaIndex.LookupChecksum(deltaPackfileChecksumID))
	}

	for deltaObjectChecksumID, deltaPackfileChecksumID := range deltaIndex.Objects {
		index.SetPackfileForObject(deltaIndex.LookupChecksum(deltaObjectChecksumID),
			deltaIndex.LookupChecksum(deltaPackfileChecksumID))
	}
}

func (index *Index) SetPackfileForChunk(packfileChecksum [32]byte, chunkChecksum [32]byte) {
	index.muChunks.Lock()
	defer index.muChunks.Unlock()

	chunkID := index.addChecksum(chunkChecksum)
	if _, exists := index.Chunks[chunkID]; !exists {
		packfileID := index.addChecksum(packfileChecksum)
		index.Chunks[chunkID] = packfileID
	}
}

func (index *Index) GetPackfileForChunk(chunkChecksum [32]byte) ([32]byte, bool) {
	index.muChunks.Lock()
	defer index.muChunks.Unlock()

	chunkID := index.addChecksum(chunkChecksum)
	if packfileID, exists := index.Chunks[chunkID]; !exists {
		return [32]byte{}, false
	} else {
		index.muChecksums.Lock()
		packfileChecksum, exists := index.checksumsInverse[packfileID]
		index.muChecksums.Unlock()
		if !exists {
			panic("packfile checksum not found")
		}
		return packfileChecksum, true
	}
}

func (index *Index) SetPackfileForObject(packfileChecksum [32]byte, objectChecksum [32]byte) {
	index.muObjects.Lock()
	defer index.muObjects.Unlock()

	objectID := index.addChecksum(objectChecksum)
	if _, exists := index.Objects[objectID]; !exists {
		packfileID := index.addChecksum(packfileChecksum)
		index.Objects[objectID] = packfileID
	}
}

func (index *Index) GetPackfileForObject(objectChecksum [32]byte) ([32]byte, bool) {
	index.muObjects.Lock()
	defer index.muObjects.Unlock()

	objectID := index.addChecksum(objectChecksum)
	if packfileID, exists := index.Objects[objectID]; !exists {
		return [32]byte{}, false
	} else {
		index.muChecksums.Lock()
		packfileChecksum, exists := index.checksumsInverse[packfileID]
		index.muChecksums.Unlock()
		if !exists {
			panic("packfile checksum not found")
		}
		return packfileChecksum, true
	}
}
