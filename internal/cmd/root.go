// Package cmd implements the subrouterctl command tree.
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Apothic-AI/subrouterctl/internal/api"
	"github.com/Apothic-AI/subrouterctl/internal/cache"
	"github.com/Apothic-AI/subrouterctl/internal/config"
	"github.com/Apothic-AI/subrouterctl/internal/render"
)

var (
	flagBaseURL      string
	flagToken        string
	flagAPIKey       string
	flagUserID       int
	flagOutput       string
	flagNoColor      bool
	flagTimeout      int
	flagQuotaPerUnit float64
	flagShowSecrets  bool
	flagYes          bool
	flagCacheTTL     time.Duration
	flagCacheDir     string
	flagCacheClear   bool
)

// Version is stamped at build time.
var Version = "1.0.0"

// Execute runs the root command.
func Execute() error {
	return NewRootCommand().Execute()
}

// NewRootCommand builds the command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "subrouterctl",
		Short: "Manage a SubRouter account from the command line",
		Long: `subrouterctl is the command line companion for SubRouter, the multi-provider
AI API gateway. It drives the full account management API: API keys of every
kind, the model and provider directories, usage logs, async task records,
provider subscriptions and shared pool plans.

Every command works against the account management API using the system access
token. No browser is needed after the one time device authorization.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --cache-clear is a standalone action.
			if flagCacheClear {
				if err := newCacheStore().Clear(); err != nil {
					return err
				}
				fmt.Println("cache cleared")
				return nil
			}
			return cmd.Help()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&flagBaseURL, "base-url", "", "SubRouter site base URL (env SUBROUTER_BASE_URL)")
	pf.StringVar(&flagToken, "token", "", "system access token (env SUBROUTER_ACCESS_TOKEN)")
	pf.StringVar(&flagAPIKey, "api-key", "", "sk- relay key (env SUBROUTER_API_KEY)")
	pf.IntVar(&flagUserID, "user-id", 0, "numeric account id for the New-Api-User header (env SUBROUTER_USER_ID)")
	pf.StringVarP(&flagOutput, "output", "o", "table", "output format: table, json, yaml")
	pf.BoolVar(&flagNoColor, "no-color", false, "disable coloured output")
	pf.IntVar(&flagTimeout, "timeout", 60, "request timeout in seconds")
	pf.Float64Var(&flagQuotaPerUnit, "quota-per-unit", 0, "quota units per 1 USD (default 500000)")
	pf.BoolVar(&flagShowSecrets, "show-secrets", false, "print secret values in full instead of masking them")
	pf.BoolVarP(&flagYes, "yes", "y", false, "assume yes for confirmation prompts")
	pf.DurationVar(&flagCacheTTL, "cache-ttl", 0, "serve public directory responses from the on-disk cache for this long (0 disables the cache)")
	pf.StringVar(&flagCacheDir, "cache-dir", defaultCacheDir(), "directory for the on-disk cache")
	pf.BoolVar(&flagCacheClear, "cache-clear", false, "remove every cached entry and exit")

	root.AddCommand(
		newAuthCommand(),
		newAccountCommand(),
		newUsageCommand(),
		newTokenCommand(),
		newModelCommand(),
		newProviderCommand(),
		newSharedCommand(),
		newSubscriptionCommand(),
		newTaskCommand(),
		newMerchantCommand(),
		newStationCommand(),
	)
	return root
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// cfg resolves the configuration for this invocation.
func cfg() (*config.Config, error) {
	return config.Resolve(flagBaseURL, flagToken, flagAPIKey, flagUserID, flagQuotaPerUnit, flagTimeout, flagShowSecrets)
}

// defaultCacheDir is where the on-disk cache lives.
func defaultCacheDir() string {
	if p := os.Getenv("SUBROUTER_CACHE_DIR"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cache/subrouterctl"
	}
	return filepath.Join(home, ".cache", "subrouterctl")
}

// newCacheStore builds the on-disk cache store for this invocation.
func newCacheStore() *cache.Store {
	return cache.New(flagCacheDir, flagCacheTTL)
}

// client builds an API client and verifies the management credential pair.
func client() (*api.Client, *config.Config, error) {
	c, err := cfg()
	if err != nil {
		return nil, nil, err
	}
	if err := c.RequireManagementAuth(); err != nil {
		return nil, nil, err
	}
	cl := api.New(c.BaseURL, c.AccessToken, c.UserID, c.APIKey, time.Duration(c.Timeout)*time.Second)
	cl.Cache = newCacheStore()
	return cl, c, nil
}

// relayClient builds an API client for the sk- relay surface.
func relayClient() (*api.Client, *config.Config, error) {
	c, err := cfg()
	if err != nil {
		return nil, nil, err
	}
	if err := c.RequireRelayAuth(); err != nil {
		return nil, nil, err
	}
	return api.New(c.BaseURL, c.AccessToken, c.UserID, c.APIKey, time.Duration(c.Timeout)*time.Second), c, nil
}

// writer builds the output writer.
func writer() *render.Writer {
	opts := render.DefaultOptions()
	switch strings.ToLower(flagOutput) {
	case "json":
		opts.Format = render.FormatJSON
	case "yaml", "yml":
		opts.Format = render.FormatYAML
	default:
		opts.Format = render.FormatTable
	}
	if flagNoColor {
		opts.Color = false
	}
	if flagQuotaPerUnit > 0 {
		opts.QuotaPerUnit = flagQuotaPerUnit
	}
	return render.New(opts)
}

// ctx returns a request context bounded by the timeout flag.
func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(flagTimeout)*time.Second)
}

// confirm asks the operator to approve a destructive action.
func confirm(prompt string) bool {
	if flagYes {
		return true
	}
	fmt.Printf("%s [y/N]: ", prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// listFlags are the search, sort and filter flags shared by list commands.
type listFlags struct {
	search   string
	sort     string
	order    string
	limit    int
	page     int
	pageSize int
}

// add registers the shared list flags on a command.
func (l *listFlags) add(cmd *cobra.Command, defaultSort string) {
	f := cmd.Flags()
	f.StringVarP(&l.search, "search", "q", "", "free text search")
	f.StringVar(&l.sort, "sort", "", "sort field ("+defaultSort+")")
	f.StringVar(&l.order, "order", "asc", "sort order: asc or desc")
	f.IntVar(&l.limit, "limit", 50, "maximum rows to print (0 for no limit)")
	f.IntVar(&l.page, "page", 1, "page number")
	f.IntVar(&l.pageSize, "page-size", 50, "rows per page")
}

// desc reports whether the sort order is descending.
func (l *listFlags) desc() bool {
	return strings.EqualFold(l.order, "desc") || strings.EqualFold(l.order, "descending")
}

// applyLimit trims a slice to the requested limit.
func (l *listFlags) applyLimit[T any](items []T) []T {
	if l.limit > 0 && len(items) > l.limit {
		return items[:l.limit]
	}
	return items
}

// parseTimeFlag parses a date, timestamp or relative age into unix seconds.
//
// Accepted forms: 2026-01-31, 2026-01-31T15:04:05Z, 1738281600, 24h, 7d.
func parseTimeFlag(s string, endOfDay bool) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.EqualFold(s, "now") {
		return time.Now().Unix(), nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	if strings.HasSuffix(s, "d") || strings.HasSuffix(s, "h") || strings.HasSuffix(s, "m") {
		unit := s[len(s)-1]
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("cannot parse time %q", s)
		}
		now := time.Now()
		switch unit {
		case 'd':
			return now.AddDate(0, 0, -n).Unix(), nil
		case 'h':
			return now.Add(-time.Duration(n) * time.Hour).Unix(), nil
		case 'm':
			return now.Add(-time.Duration(n) * time.Minute).Unix(), nil
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.Unix(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix(), nil
	}
	return 0, fmt.Errorf("cannot parse time %q: use 2026-01-31, 24h or a unix timestamp", s)
}

// timeFlags are the shared date range flags.
type timeFlags struct {
	from string
	to   string
}

// add registers the date range flags on a command.
func (t *timeFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&t.from, "from", "", "start of the range: 2026-01-31, 24h or a unix timestamp")
	cmd.Flags().StringVar(&t.to, "to", "", "end of the range: 2026-01-31, 1h or a unix timestamp")
}

// range renders the flag values as unix seconds.
func (t *timeFlags) range_() (int64, int64, error) {
	start, err := parseTimeFlag(t.from, false)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseTimeFlag(t.to, true)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// splitList splits a comma separated flag value.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// concurrentPages fetches pages in parallel with a bounded worker pool.
// fetch returns the rows of one page and the total row count.
func concurrentPages[T any](ctx context.Context, pages, workers int, fetch func(context.Context, int) ([]T, int, error)) ([]T, error) {
	if pages <= 0 {
		return nil, nil
	}
	if workers < 1 {
		workers = 1
	}
	rows := make([][]T, pages)
	errs := make(chan error, pages)
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for p := 1; p <= pages; p++ {
		wg.Add(1)
		go func(page int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			got, _, err := fetch(ctx, page)
			if err != nil {
				errs <- err
				return
			}
			rows[page-1] = got
		}(p)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	var all []T
	for _, r := range rows {
		all = append(all, r...)
	}
	return all, nil
}

// apiLogQuery builds a log query from the shared date and list flags.
func apiLogQuery(start, end int64, lf listFlags) api.LogQuery {
	return api.LogQuery{
		PageSize:       lf.pageSize,
		StartTimestamp: start,
		EndTimestamp:   end,
	}
}

// parseModelRoutes parses repeated model=provider1,provider2 flags.
func parseModelRoutes(entries []string) (map[string][]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := map[string][]string{}
	for _, e := range entries {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected model=provider1,provider2, got %q", e)
		}
		model := strings.TrimSpace(parts[0])
		providers := splitList(parts[1])
		if model == "" || len(providers) == 0 {
			return nil, fmt.Errorf("expected model=provider1,provider2, got %q", e)
		}
		out[model] = providers
	}
	return out, nil
}
