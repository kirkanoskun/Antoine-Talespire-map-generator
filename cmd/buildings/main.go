// Command buildings manages the local library of reusable TaleSpire buildings.
//
//	buildings add -file inn.txt -name "Smiling Goat Inn" \
//	    -source https://talestavern.com/... -author someone -license "CC BY-NC" \
//	    -tags inn,tavern,village
//	buildings list
//	buildings list -q tavern
//	buildings show smiling-goat-inn
//	buildings remove smiling-goat-inn
//
// Then build a map around one:
//
//	composemap -ir scene.json -building smiling-goat-inn -at 21,5 -slice 26 ...
//
// The library lives in ./buildings by default (override with -dir or
// TALESPIRE_BUILDINGS). It is kept out of git: it holds other people's builds,
// each under its own licence.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/buildings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "add":
		err = addCmd(args)
	case "list", "ls":
		err = listCmd(args)
	case "show":
		err = showCmd(args)
	case "remove", "rm":
		err = removeCmd(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `manage the local library of reusable TaleSpire buildings

  buildings add    -file CODE.txt -name "Name" [-id slug] [-tags a,b]
                   [-source URL] [-author WHO] [-license TERMS]
                   [-ground RAWZ] [-replace]
  buildings list   [-q query]
  buildings show   ID
  buildings remove ID

Common flags: -dir DIR (default ./buildings, or $TALESPIRE_BUILDINGS)
`)
}

func libDir(fs *flag.FlagSet) *string {
	def := os.Getenv("TALESPIRE_BUILDINGS")
	if def == "" {
		def = "buildings"
	}
	return fs.String("dir", def, "library directory")
}

func addCmd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	dir := libDir(fs)
	file := fs.String("file", "", "file holding the base64 slab code (default: stdin)")
	name := fs.String("name", "", "human-readable name (required)")
	id := fs.String("id", "", "library id (default: slug of the name)")
	tags := fs.String("tags", "", "comma-separated tags, e.g. inn,tavern,village")
	desc := fs.String("description", "", "one-line description")
	source := fs.String("source", "", "where it came from (URL)")
	author := fs.String("author", "", "who made it")
	license := fs.String("license", "", "licence terms, e.g. \"CC BY-NC\"")
	ground := fs.String("ground", "", "rawZ of the building's ground level (default: auto-detect the busiest level; see `slabdecode -levels`)")
	replace := fs.Bool("replace", false, "overwrite an existing id")
	_ = fs.Parse(args)

	if *name == "" && *id == "" {
		return fmt.Errorf("-name (or -id) is required")
	}

	var raw []byte
	var err error
	if *file != "" {
		raw, err = os.ReadFile(*file)
	} else {
		raw, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return err
	}

	opts := buildings.AddOptions{
		ID:          *id,
		Name:        *name,
		Code:        string(raw),
		Tags:        splitTags(*tags),
		Description: *desc,
		Source:      *source,
		Author:      *author,
		License:     *license,
		Replace:     *replace,
	}
	if *ground != "" {
		v, err := strconv.ParseUint(strings.TrimSpace(*ground), 10, 32)
		if err != nil {
			return fmt.Errorf("-ground: %w", err)
		}
		g := uint32(v)
		opts.GroundZ = &g
	}

	lib, err := buildings.Open(*dir)
	if err != nil {
		return err
	}
	e, err := lib.Add(opts)
	if err != nil {
		return err
	}

	fmt.Printf("added %q to %s\n", e.ID, lib.Dir)
	printEntry(e)
	if e.License == "" || e.Source == "" {
		fmt.Printf("\nnote: no %s recorded. Community builds are other people's work —\n"+
			"      re-run with -replace and the missing details when you have them.\n",
			strings.Join(missing(e), " and no "))
	}
	return nil
}

func missing(e *buildings.Entry) []string {
	var m []string
	if e.Source == "" {
		m = append(m, "source")
	}
	if e.License == "" {
		m = append(m, "licence")
	}
	return m
}

func listCmd(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	dir := libDir(fs)
	q := fs.String("q", "", "filter on id, name, tags or description")
	_ = fs.Parse(args)

	lib, err := buildings.Open(*dir)
	if err != nil {
		return err
	}
	found := lib.Find(*q)
	if len(found) == 0 {
		if len(lib.Entries) == 0 {
			fmt.Printf("no buildings yet in %s — add one with `buildings add -file CODE.txt -name \"...\"`\n", lib.Dir)
		} else {
			fmt.Printf("nothing matches %q (%d buildings in %s)\n", *q, len(lib.Entries), lib.Dir)
		}
		return nil
	}
	fmt.Printf("%-24s %-28s %-11s %-8s %-9s %s\n", "ID", "NAME", "FOOTPRINT", "HEIGHT", "BURIED", "TAGS")
	for _, e := range found {
		fmt.Printf("%-24s %-28s %-11s %-8s %-9s %s\n",
			e.ID, truncate(e.Name, 28),
			fmt.Sprintf("%dx%d", e.WidthTiles, e.LengthTiles),
			fmt.Sprintf("%.1f", e.HeightSteps),
			fmt.Sprintf("%.1f", e.BuriedSteps),
			strings.Join(e.Tags, ","))
	}
	return nil
}

func showCmd(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	dir := libDir(fs)
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("show needs exactly one id")
	}
	lib, err := buildings.Open(*dir)
	if err != nil {
		return err
	}
	e, err := lib.Get(fs.Arg(0))
	if err != nil {
		return err
	}
	printEntry(e)
	return nil
}

func removeCmd(args []string) error {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	dir := libDir(fs)
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("remove needs exactly one id")
	}
	lib, err := buildings.Open(*dir)
	if err != nil {
		return err
	}
	if err := lib.Remove(fs.Arg(0)); err != nil {
		return err
	}
	fmt.Printf("removed %q\n", fs.Arg(0))
	return nil
}

func printEntry(e *buildings.Entry) {
	fmt.Printf("  name        %s\n", e.Name)
	if e.Description != "" {
		fmt.Printf("  description %s\n", e.Description)
	}
	if len(e.Tags) > 0 {
		fmt.Printf("  tags        %s\n", strings.Join(e.Tags, ", "))
	}
	fmt.Printf("  footprint   %d x %d tiles\n", e.WidthTiles, e.LengthTiles)
	fmt.Printf("  height      %.1f steps\n", e.HeightSteps)
	fmt.Printf("  ground      rawZ %d (step %.1f)", e.GroundZ, float64(e.GroundZ)/50)
	if e.BuriedSteps > 0 {
		fmt.Printf(" — %.1f steps of cellar/footings below it", e.BuriedSteps)
	}
	fmt.Println()
	fmt.Printf("  size        %d assets, %d placements, %d chars of code\n", e.Assets, e.Placements, e.CodeChars)
	if e.Author != "" {
		fmt.Printf("  author      %s\n", e.Author)
	}
	if e.Source != "" {
		fmt.Printf("  source      %s\n", e.Source)
	}
	if e.License != "" {
		fmt.Printf("  licence     %s\n", e.License)
	}
}

func splitTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
