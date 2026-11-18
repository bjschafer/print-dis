// Command fetch writes the scripts pinned in package thirdparty into
// internal/web/static. Run it from the repository root: `make vendor-js`.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bjschafer/print-dis/internal/web/thirdparty"
)

func main() {
	dir := flag.String("dir", "internal/web/static", "directory to write the scripts into")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}

	for _, s := range thirdparty.Scripts {
		data, err := thirdparty.Fetch(ctx, client, thirdparty.Registry, s)
		if err != nil {
			fmt.Fprintln(os.Stderr, "vendor-js:", err)
			os.Exit(1)
		}
		path := filepath.Join(*dir, s.File)
		if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // a checked-in source file, world-readable like the rest
			fmt.Fprintln(os.Stderr, "vendor-js:", err)
			os.Exit(1)
		}
		fmt.Printf("%s@%s -> %s\n", s.Package, s.Version, path)
	}
}
