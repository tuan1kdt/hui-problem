package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"hui-problem/tku"
)

func newTKUCmd() *cobra.Command {
	var (
		input  string
		output string
		k      int
	)

	cmd := &cobra.Command{
		Use:   "tku",
		Short: "Run TKU (Top-K High Utility Itemsets)",
		Long:  "Mines top-k high-utility itemsets from a utility database in SPMF format (items : transactionUtility : itemUtilities).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("required flag \"input\" not set")
			}
			algo := tku.NewTKU()
			if err := algo.RunAlgorithm(input, output, k); err != nil {
				return fmt.Errorf("tku: %w", err)
			}
			algo.PrintStats()
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "path to utility database (SPMF format)")
	cmd.Flags().StringVarP(&output, "output", "o", "output.txt", "path to write top-k HUIs")
	cmd.Flags().IntVarP(&k, "topk", "k", 3, "number of top high-utility itemsets")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}
