// Command pleb-api serves the Pleb venue API, runs database migrations and imports venues.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yocca/pleb-api/internal/api"
	"github.com/yocca/pleb-api/internal/db"
	"github.com/yocca/pleb-api/internal/importer"
)

const usage = `usage: pleb-api <command> [flags]

commands:
  serve                  run the HTTP API (PORT, default 8080)
  migrate up|down|reset|status
                         apply or roll back database migrations
  import venues [flags]  import venues for an area from OSM and NY liquor license data

All commands read the database URL from DATABASE_URL.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "serve":
		return serve(ctx)
	case "migrate":
		if len(args) != 2 {
			return errors.New("usage: pleb-api migrate up|down|reset|status")
		}
		pool, err := db.Connect(ctx, databaseURL())
		if err != nil {
			return err
		}
		defer pool.Close()
		return db.Migrate(ctx, pool, args[1])
	case "import":
		if len(args) < 2 || args[1] != "venues" {
			return errors.New("usage: pleb-api import venues [flags]")
		}
		return importVenues(ctx, args[2:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func databaseURL() string {
	if u := os.Getenv("DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://pleb:pleb@localhost:5432/pleb?sslmode=disable"
}

func serve(ctx context.Context) error {
	pool, err := db.Connect(ctx, databaseURL())
	if err != nil {
		return err
	}
	defer pool.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.New(pool).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func importVenues(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import venues", flag.ContinueOnError)
	osmPath := fs.String("osm", "", "Overpass JSON export of the area (see docs/import.md)")
	slaPath := fs.String("sla", "", "NY State Liquor Authority license CSV export")
	slaURL := fs.String("sla-url", "", "with --fetch: URL of the liquor license CSV export to download")
	bboxArg := fs.String("bbox", "", "area as south,west,north,east (required)")
	zips := fs.String("zips", "", "comma-separated ZIP codes for licenses when the CSV has no coordinates")
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	fetch := fs.Bool("fetch", false, "download the OSM export from Overpass (and --sla-url) instead of reading files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	bbox, err := importer.ParseBBox(*bboxArg)
	if err != nil {
		return err
	}

	osmFile, err := openSource(ctx, *osmPath, *fetch, overpassURL(bbox), "--osm")
	if err != nil {
		return err
	}
	defer osmFile.Close()
	features, err := importer.ReadOSM(osmFile)
	if err != nil {
		return err
	}

	var licenses []importer.License
	if *slaPath != "" || (*fetch && *slaURL != "") {
		slaFile, err := openSource(ctx, *slaPath, *fetch && *slaURL != "", *slaURL, "--sla")
		if err != nil {
			return err
		}
		defer slaFile.Close()
		if licenses, err = importer.ReadLicenses(slaFile); err != nil {
			return err
		}
	}

	pool, err := db.Connect(ctx, databaseURL())
	if err != nil {
		return err
	}
	defer pool.Close()

	opts := importer.Options{BBox: bbox, DryRun: *dryRun}
	if *zips != "" {
		for _, z := range strings.Split(*zips, ",") {
			opts.ZIPs = append(opts.ZIPs, strings.TrimSpace(z))
		}
	}
	rep, err := importer.Run(ctx, pool, features, licenses, opts)
	if err != nil {
		return err
	}
	rep.Print(os.Stdout, *dryRun)
	return nil
}

func overpassURL(b importer.BBox) string {
	return "https://overpass-api.de/api/interpreter?data=" + url.QueryEscape(b.OverpassQuery())
}

// openSource opens a local file, or downloads src when fetch is set.
func openSource(ctx context.Context, path string, fetch bool, src, flagName string) (io.ReadCloser, error) {
	if !fetch {
		if path == "" {
			return nil, fmt.Errorf("%s is required (or use --fetch)", flagName)
		}
		return os.Open(path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pleb-api-importer/0.1 (+https://github.com/yocca/pleb-api)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", src, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: %s", src, resp.Status)
	}
	return resp.Body, nil
}
