package server

import (
	"runtime"
	"testing"
	"time"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/protocol"
)

func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// A long session does not make the server keep more and more memory (reports/suite_rapida/HALLAZGOS.md, F: the
// local server of a 20 minute real-time soak was seen at 48 MB once, from 14.7). 48,000 ticks (40 minutes of play)
// of two players with their hashes, a resynchronization with a 60 KB state every 400 ticks, and a third player who
// founds another group and leaves it every 4,000: what stays alive after a collection is the same at the end as
// after the first sixth.
func TestLongSessionKeepsNoMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	cfg := testConfig()
	cfg.ZeroFillAfter = time.Hour
	cfg.SlowFillAfter = time.Hour
	_, addr := startServerCfg(t, cfg)
	a := welcome3(t, addr, "A")
	found3(t, a, 85, sharedGame(10, 0))
	b := welcome3(t, addr, "B")
	p := join3(t, b, a, 85, "zmp_spawn 9")
	head := p.SpawnTick
	a.input(t, head, 1)
	a.expectTick(t, head)
	b.expectTick(t, head)
	head++
	state := make([]byte, 60000)
	const epoch, epochs, batch = 4000, 12, 20
	var early uint64
	for e := 0; e < epochs; e++ {
		for n := 0; n < epoch/batch; n++ {
			for tick := head; tick < head+batch; tick++ {
				b.input(t, tick, 2)
			}
			for tick := head; tick < head+batch; tick++ {
				a.input(t, tick, 1)
			}
			for tick := head; tick < head+batch; tick++ {
				a.expectTick(t, tick)
				b.expectTick(t, tick)
			}
			head += batch
			hb := uint64(7)
			if n%20 == 19 {
				hb = 8 // B differs: resync
			}
			a.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: 7})
			b.send(t, protocol.Hash{T: "HASH", Tick: head - 1, Hash: hb})
			if hb != 7 {
				a.expect(t, "RESYNC")
				req := decode[protocol.StateBlobRequest](t, a.expect(t, "STATE_BLOB_REQUEST"))
				a.send(t, protocol.StateBlob{T: "STATE_BLOB", GroupID: req.GroupID, Tick: req.Tick, ForSlot: 1,
					Data: state, Hash: 7})
				b.expect(t, "RESYNC")
				b.expect(t, "STATE_BLOB")
			}
		}
		c := welcome3(t, addr, "C")
		found3(t, c, 45, nil)
		c.input(t, 0, 3)
		c.expectTick(t, 0)
		c.conn.Close()
		time.Sleep(20 * time.Millisecond)
		if e == 1 {
			early = liveHeap()
		}
	}
	late := liveHeap()
	t.Logf("live heap after %d ticks: %d KB; after %d ticks: %d KB", 2*epoch, early/1024, epochs*epoch, late/1024)
	if late > early+2<<20 {
		t.Fatalf("the server keeps memory as the session goes on: %d KB after %d ticks, %d KB after %d", early/1024,
			2*epoch, late/1024, epochs*epoch)
	}
}
