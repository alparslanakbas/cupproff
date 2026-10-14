// Command cupproof runs the site and its database migrations.
//
//	cupproof serve     serve the site
//	cupproof migrate   apply database migrations
//	cupproof scrape [-only a,b]  read active roasters once, store the prices, refresh summaries
//	cupproof refresh   recompute the offer summaries from the price history
//
// Configuration comes from the environment (DATABASE_URL, SITE_URL, ADDR);
// a local .env file is read when present, never required.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/ingest"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/scrape"
	"github.com/alparslanakbas/cupproof/internal/shopify"
	"github.com/alparslanakbas/cupproof/internal/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loadDotEnv(".env")
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cupproof serve | migrate | scrape | refresh")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(ctx, log)
	case "migrate":
		err = db.Migrate(ctx, mustEnv("DATABASE_URL"))
	case "scrape":
		err = runScrape(ctx, log, os.Args[2:])
	case "refresh":
		err = runRefresh(ctx, log)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Error("failed", "err", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, log *slog.Logger) error {
	pool, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	s := &web.Server{
		Q:       db.New(pool),
		SiteURL: strings.TrimSuffix(envOr("SITE_URL", "http://localhost:8080"), "/"),
		Log:     log,
	}
	srv := &http.Server{
		Addr:              envOr("ADDR", "127.0.0.1:8080"),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("serving", "addr", srv.Addr, "site", s.SiteURL)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func runRefresh(ctx context.Context, log *slog.Logger) error {
	pool, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	return refreshSummaries(ctx, log, db.New(pool))
}

// The scraper identifies itself; the token matches the robots.txt checks.
const userAgent = "Mozilla/5.0 (compatible; CupProofBot/0.1)"

func runScrape(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("scrape", flag.ExitOnError)
	only := fs.String("only", "", "comma-separated roaster slugs to read (default: all active)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	runner := &scrape.Runner{
		Q:      db.New(pool),
		Ingest: &ingest.Service{Pool: pool},
		Client: &shopify.Client{
			HTTP:      &http.Client{Transport: shopify.NewTransport(), Timeout: 60 * time.Second},
			UserAgent: userAgent,
			PageDelay: 3 * time.Second,
			// A 5xx gets one retry after this pause (14 Oct 2026: three 503s
			// in a minute, each cost a store its day).
			ServerRetryDelay: 15 * time.Second,
			// Yunnan Sourcing hit the first cap of 4 pages (1,000 products) on
			// 13 Oct 2026; 20 pages is 5,000 products, one minute at 3 s a page.
			MaxPages: 20,
		},
		Log:  log,
		Only: splitList(*only),
	}
	failed, err := runner.Run(ctx, market.All)
	if err != nil {
		return err
	}
	// Even when some roasters failed: the others' new prices must reach the
	// pages, which read only the summaries.
	if err := refreshSummaries(ctx, log, db.New(pool)); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d roasters failed", failed)
	}
	return nil
}

func refreshSummaries(ctx context.Context, log *slog.Logger, q *db.Queries) error {
	start := time.Now()
	n, err := q.RefreshOfferSummaries(ctx)
	if err != nil {
		return fmt.Errorf("refresh summaries: %w", err)
	}
	log.Info("summaries refreshed", "offers", n, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "%s is not set\n", key)
		os.Exit(2)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv sets KEY=VALUE lines from path for keys not already set.
// Values are never logged.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
