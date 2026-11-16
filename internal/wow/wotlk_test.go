package wow

import (
	"encoding/binary"
	"testing"

	"Meld/internal/adt"
	"Meld/internal/clientbuild"
	"Meld/internal/testclient"
)

// The 3.3.5a client opens as one (#1066): its build is told, its locale folder's archives are among
// the ones read (the tables are only there), and Meld's patch is the one that client leaves free.
// The rest pins what the readers of the later parts start from: a table of that client's width, a
// map whose WDT asks for 8-bit alpha maps beside one that does not, and a tile whose terrain and
// texture layers the 1.12 reader already reads.
//
//	WOW335_DIR=/path/to/a/3.3.5a/client go test ./internal/wow/ -run WotLK -v
func TestWotLKClientOpens(t *testing.T) {
	dir := testclient.WotLKDir(t)
	c := NewClient()
	if err := c.Open(dir); err != nil {
		t.Fatal(err)
	}
	st := c.Status()
	t.Logf("status: %+v", st)
	if st.Build != int(clientbuild.WotLK) || st.Version != "3.3.5a" || st.BuildWarning != "" {
		t.Fatalf("build %d, version %q, warning %q; want 12340, 3.3.5a and none", st.Build, st.Version, st.BuildWarning)
	}
	data, err := ResolveDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := clientbuild.PatchFor(data); got != "patch-4.MPQ" {
		t.Errorf("Meld's patch is %s, want patch-4.MPQ: the client ships its own patch-3.MPQ", got)
	}
	if clientbuild.Locale(data) == "" {
		t.Error("no locale folder found in the Data folder")
	}

	// Map.dbc is in the locale archives only. Its header: WDBC, records, fields, record size, strings.
	table, err := c.ReadFile(`DBFilesClient\Map.dbc`)
	if err != nil {
		t.Fatalf("Map.dbc, which the locale archives hold: %v", err)
	}
	if len(table) < 20 || string(table[:4]) != "WDBC" {
		t.Fatalf("Map.dbc is not a DBC table (%d bytes)", len(table))
	}
	records, fields := binary.LittleEndian.Uint32(table[4:]), binary.LittleEndian.Uint32(table[8:])
	if fields != 66 || records < 100 {
		t.Errorf("Map.dbc has %d records of %d fields; the 3.3.5a table has 66 fields and over 100 maps", records, fields)
	}

	// Northrend's WDT asks for 8-bit alpha maps; the old world's does not, in the same client, so the
	// alpha format follows the map and not the client's build (#1069).
	for name, want := range map[string]bool{`World\Maps\Northrend\Northrend.wdt`: true, `World\Maps\Azeroth\Azeroth.wdt`: false} {
		wdt, err := c.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := adt.ReadBigAlpha(wdt); got != want {
			t.Errorf("%s: 8-bit alpha maps %v, want %v", name, got, want)
		}
	}

	// A Northrend tile's terrain and texture layers read with the 1.12 reader, 8-bit alpha included.
	root, err := c.ReadFile(`World\Maps\Northrend\Northrend_30_20.adt`)
	if err != nil {
		t.Fatal(err)
	}
	terrain, err := adt.ParseTerrain(root)
	if err != nil {
		t.Fatalf("the tile's terrain: %v", err)
	}
	textures, err := adt.ParseTextures(root, true, terrain)
	if err != nil {
		t.Fatalf("the tile's textures: %v", err)
	}
	layers, alpha := 0, 0
	for _, tc := range textures.Chunks {
		layers += len(tc.Layers)
		for _, a := range tc.Alpha {
			if len(a) == 64*64 {
				alpha++
			}
		}
	}
	if len(terrain.Chunks) != 256 || len(textures.Chunks) != 256 || layers < 256 || alpha == 0 {
		t.Errorf("%d terrain chunks, %d texture chunks, %d layers, %d alpha maps of 64x64; want 256, 256, at least 256 and some", len(terrain.Chunks), len(textures.Chunks), layers, alpha)
	}
}
