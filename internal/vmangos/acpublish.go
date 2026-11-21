package vmangos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Publishing a map made in Meld to an AzerothCore server from the 3.3.5a client, as Publish does for
// VMaNGOS and PublishCMaNGOS for CMaNGOS classic. What the world server needs of a new map was read in
// azerothcore-wotlk (GPL, 2026-11; none of its code is copied):
//   - the map's row in its dbc/Map.dbc, which it reads at start: Meld's patch holds it, and
//     UpdateAzerothCoreMaps copies the patch's tables into the server's dbc folder (acInstallTables);
//   - its map and collision files, which UpdateAzerothCoreMaps extracts for that map alone;
//   - for a dungeon or raid, an instance_template row (map, parent, script, allowMount), without which
//     the server makes no instance of it;
//   - a MapDifficulty.dbc row of the map's normal difficulty, without which the server refuses a player
//     at a dungeon's entrance (the client gives its continents one too): buildMap adds it to the patch
//     when the client's table lacks one (addMapDifficultyRow), and it reaches the server with Map.dbc.
//
// A battleground needs a battleground_template row and a BattlemasterList.dbc entry, which Meld does not
// write, so one is refused before anything is written.

// ErrACBattleground is the error of a battleground published to AzerothCore.
var ErrACBattleground = errors.New("a battleground made in Meld is not published to AzerothCore: it needs a battleground_template row and a BattlemasterList.dbc entry, which Meld does not write. Set its type to a world map, dungeon or raid in Map Properties")

// ACInstanceSQL is the instance_template row of a dungeon or raid made in Meld for an AzerothCore world
// database, or "" for any other map. A row the author has already changed (its script, say) is kept.
func ACInstanceSQL(m MapSpec) string {
	if m.Info == nil || m.Info.Type != 1 && m.Info.Type != 2 {
		return ""
	}
	return fmt.Sprintf("INSERT INTO `instance_template` (`map`, `parent`, `script`, `allowMount`) VALUES (%d, 0, '', 1) "+
		"ON DUPLICATE KEY UPDATE `map` = `map`;", m.MapID)
}

// ACInstanceRow is map m's instance_template row as the Changes window lists it, and whether it has one.
func ACInstanceRow(m MapSpec) (RowChange, bool) {
	sql := ACInstanceSQL(m)
	if sql == "" {
		return RowChange{}, false
	}
	return RowChange{Table: "instance_template", Key: fmt.Sprintf("map %d", m.MapID), Action: "add", Core: string(AzerothCore), SQL: sql,
		Reason: "the AzerothCore world server makes no instance of a dungeon or raid without its row; one already there is kept"}, true
}

// PublishAzerothCore publishes a map made in Meld to an AzerothCore server: it checks the server's
// extractors and the map's type first, writes the map into the 3.3.5a client's patch (only the files
// that changed), adds a dungeon or raid's instance_template row to the world database, and rebuilds the
// server's map data and tables with UpdateAzerothCoreMaps, which waits for a running world server to
// stop. An unchanged map over map data newer than the patch leaves the server's files alone.
func PublishAzerothCore(s AzerothCoreServer, clientRoot string, spec MapSpec, skipMoveMaps bool, run Runner, log func(string)) error {
	if run == nil {
		run = OSRunner{}
	}
	clientData := filepath.Join(clientRoot, "Data")
	if info, err := os.Stat(clientData); err != nil || !info.IsDir() {
		return fmt.Errorf("%s has no Data folder: set the WoW client folder first", clientRoot)
	}
	if spec.Info != nil && spec.Info.Type == 3 {
		return ErrACBattleground
	}
	if err := acTools(s, clientRoot); err != nil {
		return err
	}
	var db worldDB
	instance := ACInstanceSQL(spec)
	if instance != "" {
		var err error
		if db, err = acDatabase(s); err != nil {
			return err
		}
	}

	log("Writing the map into the client's patch…")
	current := mapsCurrent(filepath.Join(s.Data, "maps"), spec.MapID, PatchPath(clientData))
	res, err := ExportMap(clientData, clientData, "", spec)
	if err != nil {
		return err
	}
	if res.Patch != "" {
		log(fmt.Sprintf("Wrote %d of %d tiles to %s (the rest were unchanged)", res.Changed, res.Tiles, filepath.Base(res.Patch)))
	}
	if instance != "" {
		log("Adding the dungeon's instance_template row to the AzerothCore world database…")
		if _, err := runSQL(db, instance); err != nil {
			return fmt.Errorf("world database: %w", err)
		}
	}
	if res.Patch == "" && current {
		log("Nothing changed since the last publish, and the AzerothCore server's map data is newer than the patch: the server was left alone.")
		return nil
	}
	var tiles []string
	if current && !res.Layout {
		tiles = res.ChangedTiles
	}
	return UpdateAzerothCoreMaps(s, clientRoot, spec.MapID, tiles, skipMoveMaps, run, log)
}
