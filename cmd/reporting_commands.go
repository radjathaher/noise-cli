package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/radjathaher/noise-cli/internal/reporting"
	commandruntime "github.com/radjathaher/noise-cli/internal/runtime"
	"github.com/spf13/cobra"
)

type reportingAPI interface {
	Health(context.Context) (json.RawMessage, error)
	DailyReport(context.Context, reporting.DailyReportParams) (json.RawMessage, error)
	ListPosts(context.Context, reporting.PostsParams) (json.RawMessage, error)
	ListCampaigns(context.Context) (json.RawMessage, error)
	ListPlaybooks(context.Context) (json.RawMessage, error)
}

type reportingFactory func(context.Context) (reportingAPI, error)

func NewReportingCommand() *cobra.Command {
	return newReportingCommand(defaultReportingFactory())
}

func ReportingCommands() []*cobra.Command {
	return reportingCommands(defaultReportingFactory())
}

func ReportingCommandsWithClient(client *reporting.Client) []*cobra.Command {
	return reportingCommands(func(context.Context) (reportingAPI, error) { return client, nil })
}

// AddReportingCommands merges reporting children into existing command groups.
// This lets campaign and playbook share one top-level namespace with MCP commands.
func AddReportingCommands(root *cobra.Command) error {
	for _, incoming := range ReportingCommands() {
		existing := directChild(root, incoming.Name())
		if existing == nil {
			root.AddCommand(incoming)
			continue
		}
		if !incoming.HasSubCommands() {
			return fmt.Errorf("command %q already exists", incoming.Name())
		}
		for _, child := range incoming.Commands() {
			if directChild(existing, child.Name()) != nil {
				return fmt.Errorf("command %q already exists", existing.CommandPath()+" "+child.Name())
			}
			existing.AddCommand(child)
		}
	}
	return nil
}

func directChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func defaultReportingFactory() reportingFactory {
	return func(ctx context.Context) (reportingAPI, error) {
		options := commandruntime.FromContext(ctx)
		return reporting.NewClient(reporting.Config{
			BaseURL: options.BaseURL,
			Org:     os.Getenv("NOISE_ORG"), APIKey: os.Getenv("NOISE_API_KEY"),
			RequestTimeout: options.Timeout,
		})
	}
}

func NewReportingCommandWithClient(client *reporting.Client) *cobra.Command {
	return newReportingCommand(func(context.Context) (reportingAPI, error) { return client, nil })
}

func newReportingCommand(factory reportingFactory) *cobra.Command {
	command := &cobra.Command{
		Use:          "reporting",
		Short:        "Query the Noise Brand Reporting API",
		SilenceUsage: true,
	}
	command.AddCommand(reportingCommands(factory)...)
	return command
}

func reportingCommands(factory reportingFactory) []*cobra.Command {
	return []*cobra.Command{
		newHealthCommand(factory),
		newReportCommand(factory),
		newPostCommand(factory),
		newCampaignCommand(factory),
		newPlaybookCommand(factory),
	}
}

func newHealthCommand(factory reportingFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check Reporting API health",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runReporting(command, factory, func(ctx context.Context, client reportingAPI) (json.RawMessage, error) {
				return client.Health(ctx)
			})
		},
	}
}

func newReportCommand(factory reportingFactory) *cobra.Command {
	command := &cobra.Command{Use: "report", Short: "Query aggregate reports"}
	command.AddCommand(newDailyReportCommand(factory))
	return command
}

func newDailyReportCommand(factory reportingFactory) *cobra.Command {
	params := reporting.DailyReportParams{}
	command := &cobra.Command{
		Use:   "daily",
		Short: "Get daily metrics",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if command.Flags().Changed("campaign-id") {
				value, err := command.Flags().GetInt64("campaign-id")
				if err != nil {
					return err
				}
				params.CampaignID = &value
			}
			if command.Flags().Changed("playbook-id") {
				value, err := command.Flags().GetInt64("playbook-id")
				if err != nil {
					return err
				}
				params.PlaybookID = &value
			}
			return runReporting(command, factory, func(ctx context.Context, client reportingAPI) (json.RawMessage, error) {
				return client.DailyReport(ctx, params)
			})
		},
	}
	command.Flags().StringVar(&params.StartDate, "start-date", "", "inclusive UTC start date (YYYY-MM-DD)")
	command.Flags().StringVar(&params.EndDate, "end-date", "", "inclusive UTC end date (YYYY-MM-DD)")
	command.Flags().StringVar(&params.GroupBy, "group-by", "", "comma-separated dimensions: campaign,playbook")
	command.Flags().Int64("campaign-id", 0, "filter by campaign ID")
	command.Flags().Int64("playbook-id", 0, "filter by playbook ID (0 is Custom Posts)")
	command.Flags().StringVar(&params.Metrics, "metrics", "", "comma-separated metrics")
	_ = command.MarkFlagRequired("start-date")
	_ = command.MarkFlagRequired("end-date")
	return command
}

func newPostCommand(factory reportingFactory) *cobra.Command {
	command := &cobra.Command{Use: "post", Short: "Query posts"}
	command.AddCommand(newPostListCommand(factory))
	return command
}

func newPostListCommand(factory reportingFactory) *cobra.Command {
	params := reporting.PostsParams{}
	command := &cobra.Command{
		Use:   "list",
		Short: "List posts in a date range",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runReporting(command, factory, func(ctx context.Context, client reportingAPI) (json.RawMessage, error) {
				return client.ListPosts(ctx, params)
			})
		},
	}
	command.Flags().StringVar(&params.StartDate, "start-date", "", "inclusive UTC start date (YYYY-MM-DD)")
	command.Flags().StringVar(&params.EndDate, "end-date", "", "inclusive UTC end date (YYYY-MM-DD)")
	command.Flags().StringVar(&params.Mode, "mode", "created", "range mode: created or active")
	command.Flags().StringVar(&params.CampaignIDs, "campaign-id", "", "comma-separated campaign IDs")
	command.Flags().StringVar(&params.PlaybookIDs, "playbook-id", "", "comma-separated playbook IDs")
	command.Flags().IntVar(&params.Page, "page", 1, "page number")
	command.Flags().IntVar(&params.PageSize, "page-size", 25, "results per page (1-100)")
	command.Flags().StringVar(&params.Sort, "sort", "posted", "server-side sort field")
	command.Flags().StringVar(&params.Order, "order", "desc", "sort order: asc or desc")
	_ = command.MarkFlagRequired("start-date")
	_ = command.MarkFlagRequired("end-date")
	return command
}

func newCampaignCommand(factory reportingFactory) *cobra.Command {
	command := &cobra.Command{Use: "campaign", Short: "Query campaigns"}
	command.AddCommand(&cobra.Command{
		Use: "list", Short: "List campaigns", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runReporting(command, factory, func(ctx context.Context, client reportingAPI) (json.RawMessage, error) {
				return client.ListCampaigns(ctx)
			})
		},
	})
	return command
}

func newPlaybookCommand(factory reportingFactory) *cobra.Command {
	command := &cobra.Command{Use: "playbook", Short: "Query playbooks"}
	command.AddCommand(&cobra.Command{
		Use: "list", Short: "List playbooks", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runReporting(command, factory, func(ctx context.Context, client reportingAPI) (json.RawMessage, error) {
				return client.ListPlaybooks(ctx)
			})
		},
	})
	return command
}

func runReporting(command *cobra.Command, factory reportingFactory, request func(context.Context, reportingAPI) (json.RawMessage, error)) error {
	options := commandruntime.FromContext(command.Context())
	client, err := factory(command.Context())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(command.Context(), options.Timeout)
	defer cancel()
	payload, err := request(ctx, client)
	if err != nil {
		return err
	}
	return writeRawJSON(command.OutOrStdout(), payload, options.Pretty)
}

func writeRawJSON(writer io.Writer, payload json.RawMessage, pretty bool) error {
	if !json.Valid(payload) {
		return fmt.Errorf("cannot write invalid JSON response")
	}
	if pretty {
		buffer := &bytes.Buffer{}
		if err := json.Indent(buffer, payload, "", "  "); err != nil {
			return fmt.Errorf("format JSON response: %w", err)
		}
		payload = buffer.Bytes()
	}
	if _, err := writer.Write(payload); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	if _, err := io.WriteString(writer, "\n"); err != nil {
		return fmt.Errorf("write response newline: %w", err)
	}
	return nil
}
