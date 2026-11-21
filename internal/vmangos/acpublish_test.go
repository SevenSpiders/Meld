package vmangos

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Meld/internal/adt"
	"Meld/internal/mpq"
)

// acPublishClient is a 3.3.5a client (told by its lichking.MPQ) holding what a new map is built from:
// Map.dbc, Azeroth's WDT and a MapDifficulty.dbc of one dungeon.
func acPublishClient(t *testing.T) string {
	t.Helper()
	root := filepath.Join(shortTempDir(t), "client")
	data := filepath.Join(root, "Data")
	if err := os.MkdirAll(filepath.Join(data, "enUS"), 0o755); err != nil {
		t.Fatal(err)
	}
	mphd := append([]byte("DHPM"), 32, 0, 0, 0)
	files := []mpq.File{
		{Name: `DBFilesClient\Map.dbc`, Data: testMapDBC(t)},
		{Name: `World\Maps\Azeroth\Azeroth.wdt`, Data: append(mphd, make([]byte, 32)...)},
		{Name: mapDifficultyFile, Data: mapDiffDBC([4]uint32{1, 33, 0, 5})},
	}
	if err := os.WriteFile(filepath.Join(data, "lichking.MPQ"), mpq.Write(files), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "enUS", "locale-enUS.MPQ"), mpq.Write(nil), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// acPublishRunner fakes AzerothCore's tools: the map extractor writes a tile file for each map of the
// Map.dbc its input lists, and records which maps those were.
func acPublishRunner(t *testing.T, s AzerothCoreServer, maps *[]uint32) *fakeRunner {
	f := &fakeRunner{image: "worldserver.exe", writes: map[string][]string{
		"vmap4_extractor": {"Buildings/dir_bin"},
		"vmap4_assembler": {"vmaps/1902.vmtree"},
	}}
	f.onRun = func(name string, args []string) {
		if !strings.HasSuffix(name, "map_extractor") {
			return
		}
		stage := filepath.Join(s.Data, stagingDir)
		input := args[1]
		if !filepath.IsAbs(input) {
			input = filepath.Join(stage, input)
		}
		set, err := mpq.OpenDataDir(filepath.Join(input, "Data"))
		if err != nil {
			t.Errorf("the extractor's client: %v", err)
			return
		}
		defer set.Close()
		dbc, err := set.ReadFile(`DBFilesClient\Map.dbc`)
		if err != nil {
			t.Errorf("the extractor's Map.dbc: %v", err)
			return
		}
		*maps = MapRowIDs(dbc)
		os.MkdirAll(filepath.Join(stage, "maps"), 0o755)
		for _, id := range *maps {
			if id == chainID {
				os.WriteFile(filepath.Join(stage, "maps", "19021010.map"), []byte("new"), 0o644)
			}
		}
	}
	return f
}

func TestPublishAzerothCore(t *testing.T) {
	cache, oldRoot := t.TempDir(), pendingRoot
	pendingRoot = func() string { return cache }
	defer func() { pendingRoot = oldRoot }()
	var queries []string
	old := runSQL
	defer func() { runSQL = old }()
	runSQL = func(_ worldDB, sql string) ([]byte, error) {
		queries = append(queries, sql)
		return nil, nil
	}

	s, _ := acFixture(t)
	f, err := os.OpenFile(s.Conf, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("WorldDatabaseInfo = \"127.0.0.1;3306;acore;acore;acore_world\"\n")
	f.Close()
	client := acPublishClient(t)
	dataDir := filepath.Join(client, "Data")
	tile := TileSpec{X: 10, Y: 10, Outer: flatTile(0)}
	spec := MapSpec{Dir: chainMap, MapID: chainID, Tiles: []TileSpec{tile},
		Info: &MapInfo{Type: 1, Name: "Probe Halls", MaxPlayers: 5, ResetDelay: 1, GhostMap: -1}}
	var maps []uint32

	// A battleground is refused before anything is written.
	bg := spec
	bg.Info = &MapInfo{Type: 3, MaxPlayers: 10}
	run := acPublishRunner(t, s, &maps)
	if err := PublishAzerothCore(s, client, bg, true, run, func(string) {}); !errors.Is(err, ErrACBattleground) || len(run.calls) != 0 {
		t.Fatalf("a battleground: %v, calls %q", err, run.calls)
	}
	if fileExists(PatchPath(dataDir)) {
		t.Fatal("the battleground was written into the patch")
	}

	var log []string
	if err := PublishAzerothCore(s, client, spec, true, run, func(l string) { log = append(log, l) }); err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(log, "\n"))
	}
	if !strings.HasSuffix(PatchPath(dataDir), "patch-4.MPQ") || patchFile4(t, dataDir, adt.TilePath(chainMap, 10, 10, "")) == nil {
		t.Fatal("the patch-4.MPQ lacks the map's tile")
	}
	diff := patchFile4(t, dataDir, mapDifficultyFile)
	if has, err := hasMapDifficulty(diff, chainID); err != nil || !has {
		t.Errorf("the patch's MapDifficulty.dbc has no row for the dungeon: %v", err)
	}
	if len(queries) != 2 || queries[0] != "SELECT 1" || queries[1] != ACInstanceSQL(spec) || !strings.Contains(queries[1], "1902") {
		t.Errorf("queries %q", queries)
	}
	if len(maps) != 1 || maps[0] != chainID {
		t.Errorf("the extractor read maps %v, want only %d", maps, chainID)
	}
	if len(run.calls) != 3 {
		t.Errorf("calls %q; want the three extractors, no mmaps_generator", run.calls)
	}
	if got := readText(t, filepath.Join(s.Data, "maps", "19021010.map")); got != "new" {
		t.Errorf("the server's map file = %q", got)
	}
	// The server reads the new map's row, and its difficulty, from its own dbc folder.
	for _, name := range []string{"Map.dbc", "MapDifficulty.dbc"} {
		b, err := os.ReadFile(filepath.Join(s.Data, "dbc", name))
		if err != nil || string(b) == "old" {
			t.Errorf("the server's %s was not replaced by the patch's: %v", name, err)
		}
	}
	if got := readText(t, filepath.Join(s.Data, "meld-backup", "dbc", "Map.dbc")); got != "old" {
		t.Errorf("the server's own Map.dbc was not kept: %q", got)
	}

	// Unchanged, over map data newer than the patch: no tool runs.
	later := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(s.Data, "maps", "19021010.map"), later, later)
	run.calls, log = nil, nil
	if err := PublishAzerothCore(s, client, spec, true, run, func(l string) { log = append(log, l) }); err != nil || len(run.calls) != 0 {
		t.Fatalf("an unchanged publish: %v, calls %q", err, run.calls)
	}
	if !strings.Contains(strings.Join(log, "\n"), "Nothing changed") {
		t.Errorf("log %q", log)
	}

	// A world map has no instance_template row and no difficulty.
	queries = nil
	world := spec
	world.Info = &MapInfo{Type: 0}
	if ACInstanceSQL(world) != "" {
		t.Error("a world map got an instance_template row")
	}
	if _, ok := ACInstanceRow(world); ok {
		t.Error("a world map lists an instance_template row")
	}
}

// A server whose map extractor never opens Meld's patch is refused before the patch is written.
func TestPublishAzerothCoreRefusesAStockExtractor(t *testing.T) {
	s, _ := acFixture(t)
	if err := os.WriteFile(filepath.Join(s.Bin, "map_extractor"), []byte("\x7fELF...patch-3.MPQ\x00"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := acPublishClient(t)
	var maps []uint32
	run := acPublishRunner(t, s, &maps)
	spec := MapSpec{Dir: chainMap, MapID: chainID, Tiles: []TileSpec{{X: 10, Y: 10, Outer: flatTile(0)}}}
	if err := PublishAzerothCore(s, client, spec, true, run, func(string) {}); !errors.Is(err, ErrStockACExtractor) || len(run.calls) != 0 {
		t.Fatalf("%v, calls %q", err, run.calls)
	}
	if fileExists(PatchPath(filepath.Join(client, "Data"))) {
		t.Error("the patch was written")
	}
}

// patchFile4 is a file of the client's own patch (patch-4.MPQ in a 3.3.5a client), or nil.
func patchFile4(t *testing.T, dataDir, name string) []byte {
	t.Helper()
	a, err := mpq.Open(PatchPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := a.ReadFile(name)
	if err != nil {
		return nil
	}
	return b
}
