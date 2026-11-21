package vmangos

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// MapDifficulty.dbc, a 3.3.5a client's table of each instanced map's difficulties (it has none before
// 3.0). The 12340 client's layout: ID, MapID, Difficulty, the localised message shown at a locked
// entrance (16 strings and their mask), RaidDuration (seconds a save lasts), MaxPlayers and
// Difficultystring (the difficulty's name key). AzerothCore refuses a player entering a dungeon or raid
// with no row of the map's difficulty, so a new map gets one (addMapDifficultyRow); the client gives its
// continents a row too (no players, no save), which a new world map copies.
const (
	mapDiffFields     = 23
	mapDiffMap        = 1
	mapDiffDifficulty = 2
	mapDiffMessage    = 3 // through 19
	mapDiffDuration   = 20
	mapDiffPlayers    = 21
	mapDiffName       = 22
)

// mapDifficultyFile is the table's name in the client's archives.
const mapDifficultyFile = `DBFilesClient\MapDifficulty.dbc`

// hasMapDifficulty reports whether dbc has a row for mapID.
func hasMapDifficulty(dbc []byte, mapID uint32) (bool, error) {
	records, recSize, _, err := mapDiffHeader(dbc)
	if err != nil {
		return false, err
	}
	for r := 0; r < records; r++ {
		if binary.LittleEndian.Uint32(dbc[20+r*recSize+mapDiffMap*4:]) == mapID {
			return true, nil
		}
	}
	return false, nil
}

// mapDiffHeader reads the table's record count, record size and string block size, refusing a table not
// in the 3.3.5a client's layout.
func mapDiffHeader(dbc []byte) (records, recSize, stringSize int, err error) {
	if len(dbc) < 20 || string(dbc[:4]) != "WDBC" {
		return 0, 0, 0, errors.New("MapDifficulty.dbc: not a DBC")
	}
	records = int(binary.LittleEndian.Uint32(dbc[4:]))
	fields := int(binary.LittleEndian.Uint32(dbc[8:]))
	recSize = int(binary.LittleEndian.Uint32(dbc[12:]))
	stringSize = int(binary.LittleEndian.Uint32(dbc[16:]))
	if fields != mapDiffFields || recSize != fields*4 {
		return 0, 0, 0, fmt.Errorf("MapDifficulty.dbc has %d fields of %d bytes, not the 3.3.5a client's %d", fields, recSize, mapDiffFields)
	}
	if 20+records*recSize+stringSize > len(dbc) {
		return 0, 0, 0, errors.New("MapDifficulty.dbc: truncated")
	}
	return records, recSize, stringSize, nil
}

// addMapDifficultyRow returns MapDifficulty.dbc with a row of the normal difficulty (0) for map mapID
// of mapType (MapInfo.Type: 0 a world map, 1 a dungeon, 2 a raid): the next free ID, no locked-entrance
// message, and for a dungeon or raid a save of resetDays days (0 binds no one), maxPlayers (5 for a
// dungeon, 10 for a raid when 0) and the name key of the table's first normal row of the same kind, as
// the client shows it in the raid frame; a world map's row has none of these, as the continents' have.
// The table must not have a row for the map already.
func addMapDifficultyRow(dbc []byte, mapID, mapType, resetDays, maxPlayers uint32) ([]byte, error) {
	records, recSize, stringSize, err := mapDiffHeader(dbc)
	if err != nil {
		return nil, err
	}
	raid := mapType == 2
	switch {
	case mapType == 0:
		resetDays, maxPlayers = 0, 0
	case maxPlayers == 0 && raid:
		maxPlayers = 10
	case maxPlayers == 0:
		maxPlayers = 5
	}
	field := func(r, col int) uint32 { return binary.LittleEndian.Uint32(dbc[20+r*recSize+col*4:]) }
	var nextID, name uint32
	nameFound := false
	for r := 0; r < records; r++ {
		if field(r, mapDiffMap) == mapID {
			return nil, fmt.Errorf("MapDifficulty.dbc already has map %d", mapID)
		}
		nextID = max(nextID, field(r, 0)+1)
		// A raid's normal row is for 10 players or more; a dungeon's for 5.
		if mapType != 0 && !nameFound && field(r, mapDiffDifficulty) == 0 && (field(r, mapDiffPlayers) > 5) == raid {
			name, nameFound = field(r, mapDiffName), true
		}
	}
	row := make([]byte, recSize)
	put := func(col int, v uint32) { binary.LittleEndian.PutUint32(row[col*4:], v) }
	put(0, nextID)
	put(mapDiffMap, mapID)
	put(mapDiffDifficulty, 0)
	put(mapDiffDuration, resetDays*86400)
	put(mapDiffPlayers, maxPlayers)
	put(mapDiffName, name)

	stringsAt := 20 + records*recSize
	out := make([]byte, 0, len(dbc)+recSize)
	out = append(out, dbc[:4]...)
	out = binary.LittleEndian.AppendUint32(out, uint32(records+1))
	out = append(out, dbc[8:20]...)
	out = append(out, dbc[20:stringsAt]...)
	out = append(out, row...)
	return append(out, dbc[stringsAt:stringsAt+stringSize]...), nil
}
