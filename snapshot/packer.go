package snapshot

import (
	"bytes"
	"hash"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/PlakarKorp/plakar/objects"
	"github.com/PlakarKorp/plakar/packfile"
	"github.com/PlakarKorp/plakar/resources"
	"github.com/PlakarKorp/plakar/versioning"
)

type PackerMsg struct {
	Timestamp time.Time
	Type      resources.Type
	Version   versioning.Version
	MAC       objects.MAC
	Data      []byte
}

type Packer struct {
	Blobs    map[resources.Type]map[[32]byte][]byte
	Packfile *packfile.PackFile
}

func NewPacker(hasher hash.Hash) *Packer {
	blobs := make(map[resources.Type]map[[32]byte][]byte)
	for _, Type := range resources.Types() {
		blobs[Type] = make(map[[32]byte][]byte)
	}
	return &Packer{
		Packfile: packfile.New(hasher),
		Blobs:    blobs,
	}
}

func (packer *Packer) AddBlob(Type resources.Type, version versioning.Version, mac [32]byte, data []byte) {
	if _, ok := packer.Blobs[Type]; !ok {
		packer.Blobs[Type] = make(map[[32]byte][]byte)
	}
	packer.Blobs[Type][mac] = data
	packer.Packfile.AddBlob(Type, version, mac, data)
}

func (packer *Packer) Size() uint32 {
	return packer.Packfile.Size()
}

func (packer *Packer) Types() []resources.Type {
	ret := make([]resources.Type, 0, len(packer.Blobs))
	for k := range packer.Blobs {
		ret = append(ret, k)
	}
	return ret
}

func packerJob(snap *Snapshot) {
	wg := sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var packer *Packer

			for msg := range snap.packerChan {
				if packer == nil {
					packer = NewPacker(snap.Repository().GetMACHasher())
				}

				if msg, ok := msg.(*PackerMsg); !ok {
					panic("received data with unexpected type")
				} else {
					snap.Logger().Trace("packer", "%x: PackerMsg(%d, %s, %064x), dt=%s", snap.Header.GetIndexShortID(), msg.Type, msg.Version, msg.MAC, time.Since(msg.Timestamp))
					packer.AddBlob(msg.Type, msg.Version, msg.MAC, msg.Data)
				}

				if packer.Size() > uint32(snap.repository.Configuration().Packfile.MaxSize) {
					err := snap.PutPackfile(packer)
					if err != nil {
						panic(err)
					}
					packer = nil
				}
			}

			if packer != nil {
				err := snap.PutPackfile(packer)
				if err != nil {
					panic(err)
				}
				packer = nil
			}
		}()
	}
	wg.Wait()
	snap.packerChanDone <- true
	close(snap.packerChanDone)
}

func (snap *Snapshot) PutBlob(Type resources.Type, mac [32]byte, data []byte) error {
	snap.Logger().Trace("snapshot", "%x: PutBlob(%s, %064x) len=%d", snap.Header.GetIndexShortID(), Type, mac, len(data))

	encodedReader, err := snap.repository.Encode(bytes.NewReader(data))
	if err != nil {
		return err
	}

	encoded, err := io.ReadAll(encodedReader)
	if err != nil {
		return err
	}

	snap.packerChan <- &PackerMsg{Type: Type, Version: versioning.GetCurrentVersion(Type), Timestamp: time.Now(), MAC: mac, Data: encoded}
	return nil
}

func (snap *Snapshot) GetBlob(Type resources.Type, mac [32]byte) ([]byte, error) {
	snap.Logger().Trace("snapshot", "%x: GetBlob(%s, %x)", snap.Header.GetIndexShortID(), Type, mac)

	// XXX: Temporary workaround, once the state API changes to get from both sources (delta+aggregated state)
	// we can remove this hack.
	if snap.deltaState != nil {
		packfilemac, offset, length, exists := snap.deltaState.GetSubpartForBlob(Type, mac)
		if exists {
			rd, err := snap.repository.GetPackfileBlob(packfilemac, offset, length)
			if err != nil {
				return nil, err
			}

			return io.ReadAll(rd)
		}
	}

	// Not found in delta, let's lookup the localstate
	rd, err := snap.repository.GetBlob(Type, mac)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(rd)
}

func (snap *Snapshot) BlobExists(Type resources.Type, mac [32]byte) bool {
	snap.Logger().Trace("snapshot", "%x: CheckBlob(%s, %064x)", snap.Header.GetIndexShortID(), Type, mac)

	// XXX: Same here, remove this workaround when state API changes.
	if snap.deltaState != nil {
		return snap.deltaState.BlobExists(Type, mac) || snap.repository.BlobExists(Type, mac)
	} else {
		return snap.repository.BlobExists(Type, mac)
	}
}
