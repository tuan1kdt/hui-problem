package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the hui-problem CLI (root command and subcommands).
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NewRootCmd builds the root cobra command. Exposed for tests.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hui-problem",
		Short: "High-utility itemset mining utilities",
		Long:  "CLI for high-utility pattern mining algorithms (TKU, TKO, and more).",
	}
	root.AddCommand(newTKUCmd())
	root.AddCommand(newPTKUCmd())
	root.AddCommand(newTKOCmd())
	return root
}
