package cli

import (
	"fmt"
	"runtime"

	"hui-problem/internal/algorithms/ptku"

	"github.com/spf13/cobra"
)

func newPTKUCmd() *cobra.Command {
	var (
		input   string
		output  string
		k       int
		workers int
	)

	cmd := &cobra.Command{
		Use:   "ptku",
		Short: "Run PTKU (Parallel Top-K High Utility Itemsets)",
		Long:  "Mines top-k high-utility itemsets using a parallel TKU-style algorithm (SPMF format: items : transactionUtility : itemUtilities).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("required flag \"input\" not set")
			}
			algo := ptku.NewPTKU()
			algo.Workers = workers
			if err := algo.RunAlgorithm(input, output, k); err != nil {
				return fmt.Errorf("ptku: %w", err)
			}
			algo.PrintStats()
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "path to utility database (SPMF format)")
	cmd.Flags().StringVarP(&output, "output", "o", "output.txt", "path to write top-k HUIs")
	cmd.Flags().IntVarP(&k, "topk", "k", 3, "number of top high-utility itemsets")
	cmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "number of parallel workers")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}
