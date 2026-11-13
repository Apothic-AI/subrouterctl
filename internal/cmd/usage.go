package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Apothic-AI/subrouterctl/internal/api"
	"github.com/Apothic-AI/subrouterctl/internal/render"
)

func newUsageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "usage",
		Aliases: []string{"logs"},
		Short:   "Inspect usage, quota and request logs",
	}
	cmd.AddCommand(
		newUsageLogsCommand(),
		newUsageStatsCommand(),
		newUsageDataCommand(),
		newUsageExportCommand(),
	)
	return cmd
}

func newUsageLogsCommand() *cobra.Command {
	var (
		lf        listFlags
		tf        timeFlags
		tokenName string
		modelName string
		group     string
		logType   int
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "List request logs",
		Long: `List request logs. Filters are sent to the server; --search and --sort are
applied locally, which makes --sort fetch every matching page first.

The server accepts a window of at most 31 days per request.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := client()
			if err != nil {
				return err
			}
			w := writer()
			ctx, cancel := ctx()
			defer cancel()

			start, end, err := tf.range_()
			if err != nil {
				return err
			}
			q := api.LogQuery{
				PageSize:       lf.pageSize,
				Type:           logType,
				TokenName:      tokenName,
				ModelName:      modelName,
				Group:          group,
				StartTimestamp: start,
				EndTimestamp:   end,
			}

			items, err := fetchLogPages(ctx, cl, q, lf)
			if err != nil {
				return err
			}
			items = api.FilterLogs(items, lf.search)
			api.SortLogs(items, lf.sort, lf.desc())
			if w.IsStructured() {
				return w.Structured(items)
			}
			rows := make([][]string, 0, len(items))
			for _, l := range items {
				rows = append(rows, []string{
					w.Time(l.CreatedAt),
					orDash(l.TokenName),
					orDash(l.ModelName),
					orDash(l.Group),
					w.Money(l.Quota),
					strconv.Itoa(l.PromptTokens + l.CompletionTokens),
					fmt.Sprintf("%.1fs", l.UseTime),
					orDash(l.RequestID),
				})
			}
			w.Table([]render.Column{
				{Title: "TIME"}, {Title: "KEY"}, {Title: "MODEL"}, {Title: "GROUP"},
				{Title: "COST", Align: render.AlignRight}, {Title: "TOKENS", Align: render.AlignRight},
				{Title: "TIME(s)", Align: render.AlignRight}, {Title: "REQUEST"},
			}, lf.applyLimit(rows))
			return nil
		},
	}
	lf.add(cmd, "time")
	tf.add(cmd)
	f := cmd.Flags()
	f.StringVar(&tokenName, "key-name", "", "filter by API key name")
	f.StringVar(&modelName, "model", "", "filter by model name")
	f.StringVar(&group, "group", "", "filter by routing group")
	f.IntVar(&logType, "type", 0, "filter by log type (0 = all)")
	return cmd
}

// fetchLogPages walks the cursor paging of the log endpoint.
func fetchLogPages(ctx context.Context, cl *api.Client, q api.LogQuery, lf listFlags) ([]api.LogEntry, error) {
	var all []api.LogEntry
	cursor := "first"
	page := 1
	for {
		q.Cursor = cursor
		pageRes, err := cl.GetLogs(ctx, q)
		if err != nil {
			return nil, err
		}
		all = append(all, pageRes.Items...)
		if page >= lf.page && lf.page > 0 {
			break
		}
		if !pageRes.HasMore || pageRes.NextCursor == "" {
			break
		}
		cursor = pageRes.NextCursor
		page++
		if lf.sort == "" && lf.limit > 0 && len(all) >= lf.limit {
			break
		}
	}
	return all, nil
}

func newUsageStatsCommand() *cobra.Command {
	var (
		tf        timeFlags
		tokenName string
		modelName string
		group     string
		logType   int
	)
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Summarize usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := client()
			if err != nil {
				return err
			}
			w := writer()
			ctx, cancel := ctx()
			defer cancel()

			start, end, err := tf.range_()
			if err != nil {
				return err
			}
			q := api.LogQuery{
				Type:           logType,
				TokenName:      tokenName,
				ModelName:      modelName,
				Group:          group,
				StartTimestamp: start,
				EndTimestamp:   end,
			}
			stat, err := cl.GetLogStat(ctx, q)
			if err != nil {
				return err
			}
			if w.IsStructured() {
				return w.Structured(stat)
			}
			w.Detail("Usage", []render.Field{{Label: "Spend", Value: w.Money(stat.Quota)},
				{Label: "Tokens", Value: strconv.FormatInt(int64(stat.Token), 10)},
				{Label: "Requests per minute", Value: strconv.Itoa(stat.RPM)},
				{"Tokens per minute", strconv.Itoa(stat.TPM)},
			})
			return nil
		},
	}
	tf.add(cmd)
	f := cmd.Flags()
	f.StringVar(&tokenName, "key-name", "", "filter by API key name")
	f.StringVar(&modelName, "model", "", "filter by model name")
	f.StringVar(&group, "group", "", "filter by routing group")
	f.IntVar(&logType, "type", 0, "filter by log type (0 = all)")
	return cmd
}

func newUsageDataCommand() *cobra.Command {
	var (
		lf listFlags
		tf timeFlags
	)
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Show per-day quota usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := client()
			if err != nil {
				return err
			}
			w := writer()
			ctx, cancel := ctx()
			defer cancel()

			start, end, err := tf.range_()
			if err != nil {
				return err
			}
			points, err := cl.GetData(ctx, start, end)
			if err != nil {
				return err
			}
			if w.IsStructured() {
				return w.Structured(points)
			}
			rows := make([][]string, 0, len(points))
			for _, p := range points {
				rows = append(rows, []string{
					w.Time(p.CreatedAt),
					orDash(p.ModelName),
					w.Money(p.Quota),
					strconv.Itoa(p.Count),
					strconv.FormatInt(p.TokenUsed, 10),
				})
			}
			w.Table([]render.Column{
				{Title: "DAY"}, {Title: "MODEL"}, {Title: "COST", Align: render.AlignRight},
				{Title: "REQUESTS", Align: render.AlignRight}, {Title: "TOKENS", Align: render.AlignRight},
			}, lf.applyLimit(rows))
			return nil
		},
	}
	lf.add(cmd, "day")
	tf.add(cmd)
	return cmd
}

func newUsageExportCommand() *cobra.Command {
	var (
		tf        timeFlags
		tokenName string
		modelName string
		group     string
		logType   int
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Print the CSV export URL for the current filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := client()
			if err != nil {
				return err
			}
			w := writer()
			start, end, err := tf.range_()
			if err != nil {
				return err
			}
			q := api.LogQuery{
				Type:           logType,
				TokenName:      tokenName,
				ModelName:      modelName,
				Group:          group,
				StartTimestamp: start,
				EndTimestamp:   end,
			}
			w.Plain("%s", cl.ExportLogsURL(q))
			return nil
		},
	}
	tf.add(cmd)
	f := cmd.Flags()
	f.StringVar(&tokenName, "key-name", "", "filter by API key name")
	f.StringVar(&modelName, "model", "", "filter by model name")
	f.StringVar(&group, "group", "", "filter by routing group")
	f.IntVar(&logType, "type", 0, "filter by log type (0 = all)")
	return cmd
}
