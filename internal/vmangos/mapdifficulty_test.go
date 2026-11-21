package vmangos

import (
	"encoding/binary"
	"strings"
	"testing"
)

// mapDiffDBC is a MapDifficulty.dbc in the 3.3.5a layout of rows {id, map, difficulty, players}, whose
// name keys are "DUNGEON" for 5 players and "RAID" otherwise.
func mapDiffDBC(rows ...[4]uint32) []byte {
	strs := []byte("\x00DUNGEON\x00RAID\x00")
	var recs []byte
	for _, r := range rows {
		rec := make([]byte, mapDiffFields*4)
		binary.LittleEndian.PutUint32(rec[0:], r[0])
		binary.LittleEndian.PutUint32(rec[mapDiffMap*4:], r[1])
		binary.LittleEndian.PutUint32(rec[mapDiffDifficulty*4:], r[2])
		binary.LittleEndian.PutUint32(rec[mapDiffMessage*4:], 1) // a message the new row must not copy
		binary.LittleEndian.PutUint32(rec[mapDiffDuration*4:], 604800)
		binary.LittleEndian.PutUint32(rec[mapDiffPlayers*4:], r[3])
		name := uint32(1)
		if r[3] > 5 {
			name = 9
		}
		binary.LittleEndian.PutUint32(rec[mapDiffName*4:], name)
		recs = append(recs, rec...)
	}
	out := []byte("WDBC")
	for _, v := range []uint32{uint32(len(rows)), mapDiffFields, mapDiffFields * 4, uint32(len(strs))} {
		out = binary.LittleEndian.AppendUint32(out, v)
	}
	return append(append(out, recs...), strs...)
}

// A new map gets one normal row under the next ID with no entrance message: a dungeon or raid with its
// players and save length and the name key of a row of its kind, a world map with none of these; a map
// the table has is refused.
func TestAddMapDifficultyRow(t *testing.T) {
	dbc := mapDiffDBC([4]uint32{1, 33, 0, 5}, [4]uint32{7, 33, 1, 5}, [4]uint32{3, 249, 0, 10}, [4]uint32{4, 249, 1, 25})
	for _, c := range []struct {
		kind, days, players          uint32
		wantDays, wantPlayers, wantN uint32
	}{{1, 0, 0, 0, 5, 1}, {2, 7, 0, 7, 10, 9}, {2, 3, 40, 3, 40, 9}, {0, 3, 40, 0, 0, 0}} {
		out, err := addMapDifficultyRow(dbc, 1902, c.kind, c.days, c.players)
		if err != nil {
			t.Fatal(err)
		}
		if has, err := hasMapDifficulty(out, 1902); err != nil || !has {
			t.Fatalf("the new table has no row for the map: %v", err)
		}
		records, recSize, _, _ := mapDiffHeader(out)
		if records != 5 || len(out) != len(dbc)+recSize {
			t.Fatalf("%d records, %d bytes", records, len(out))
		}
		row := out[20+4*recSize : 20+5*recSize]
		f := func(col int) uint32 { return binary.LittleEndian.Uint32(row[col*4:]) }
		if f(0) != 8 || f(mapDiffMap) != 1902 || f(mapDiffDifficulty) != 0 || f(mapDiffMessage) != 0 ||
			f(mapDiffDuration) != c.wantDays*86400 || f(mapDiffPlayers) != c.wantPlayers || f(mapDiffName) != c.wantN {
			t.Errorf("type %d: row %v", c.kind, row)
		}
		if string(out[len(out)-14:]) != "\x00DUNGEON\x00RAID\x00" {
			t.Error("the string block moved")
		}
	}
	if _, err := addMapDifficultyRow(dbc, 249, 2, 0, 0); err == nil {
		t.Error("a map the table has got a second row")
	}
	short := mapDiffDBC()
	binary.LittleEndian.PutUint32(short[8:], 22)
	if _, err := hasMapDifficulty(short, 1); err == nil || !strings.Contains(err.Error(), "23") {
		t.Errorf("another layout: %v", err)
	}
}
