// Command server is the TaleSpire Map Generator desktop app. Double-click the
// built binary (see scripts/) and it starts a local server, opens the web UI in
// your default browser, and runs until you click Quit. It is self-contained: the
// biome/prop catalogues and the UI are embedded, so it does not depend on the
// working directory.
//
// For development it also runs via `go run ./cmd/server`.
//
// The natural-language features need an Anthropic credential (ANTHROPIC_API_KEY,
// or a .env in the working directory — auto-loaded). Without one, the UI and the
// "apply edited IR" path still work offline.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/server"
)

func main() {
	loadEnv()

	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	biomes := flag.String("biomes", "", "path to biomes.json (default: embedded)")
	props := flag.String("props", "", "path to props.json (default: embedded)")
	model := flag.String("model", "", "Claude model id (default claude-opus-4-8)")
	maxRetries := flag.Int("max-retries", 2, "retries on invalid IR")
	scale := flag.Int("scale", 8, "preview pixels per tile")
	seed := flag.Int64("seed", 1, "generation seed")
	transition := flag.Int("transition", 3, "zone-border stitching half-width (0 = hard)")
	slice := flag.Int("slice", 0, "slice maps into slabs of at most N tiles per side (0 = single slab)")
	open := flag.Bool("open", true, "open the web UI in the default browser on start")
	flag.Parse()

	// Resolve config paths: embedded by default, overridable via flags.
	biomesPath, propsPath := *biomes, *props
	if biomesPath == "" || propsPath == "" {
		bp, pp, cleanup, err := talespire.WriteConfigs()
		if err != nil {
			log.Fatalf("loading embedded configs: %v", err)
		}
		defer cleanup()
		if biomesPath == "" {
			biomesPath = bp
		}
		if propsPath == "" {
			propsPath = pp
		}
	}

	gen, err := generator.New(biomesPath, propsPath)
	if err != nil {
		log.Fatalf("initialising generator: %v", err)
	}
	gen.SetTransitionHalfWidth(*transition)

	catalog, err := nl.LoadCatalog(biomesPath, generator.POIAliasNames())
	if err != nil {
		log.Fatalf("loading catalog: %v", err)
	}
	interp := nl.NewInterpreter(nl.NewAnthropicCompleter(nl.AnthropicConfig{Model: *model}), catalog)

	httpSrv := &http.Server{}
	srv := server.New(gen, server.Options{
		Interpreter:  interp,
		PreviewScale: *scale,
		MaxRetries:   *maxRetries,
		Seed:         *seed,
		SliceSize:    *slice,
		OnQuit: func() {
			log.Print("shutting down")
			_ = httpSrv.Shutdown(context.Background())
		},
	})
	httpSrv.Handler = srv.Handler()

	// Bind localhost; fall back to an OS-assigned port if the default is taken,
	// so a double-click launch never fails on "address already in use".
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("could not bind %s (%v); trying an available port", *addr, err)
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("could not bind a local port: %v", err)
		}
	}
	url := "http://" + ln.Addr().String()

	fmt.Printf("\n  TaleSpire Map Generator is running.\n  Open %s in your browser.\n  Use the Quit button in the app (or close this window) to stop.\n\n", url)
	if *open {
		if err := openBrowser(url); err != nil {
			log.Printf("could not open the browser automatically (%v); open %s yourself", err, url)
		}
	}

	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// loadEnv loads a .env from several locations so the key is found whether the
// app is launched from a terminal (working directory) or double-clicked as a
// GUI app (which inherits no shell environment and whose cwd is not the project).
// godotenv never overrides an already-set variable, so precedence is: real
// environment variable > cwd/.env > .env next to the executable > ~/.talespire/.env.
func loadEnv() {
	_ = godotenv.Load()
	if exe, err := os.Executable(); err == nil {
		_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		_ = godotenv.Load(filepath.Join(home, ".talespire", ".env"))
	}
}

// openBrowser opens url in the user's default browser, per OS.
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	return exec.Command(name, args...).Start()
}
