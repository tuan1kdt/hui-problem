package cli

import (
	"fmt"
	"runtime"

	"hui-problem/internal/algorithms/ptko"

	"github.com/spf13/cobra"
)

func newPTKOCmd() *cobra.Command {
	var (
		input   string
		output  string
		k       int
		workers int
	)

	cmd := &cobra.Command{
		Use:   "ptko",
		Short: "Run PTKO (Parallel Top-K High Utility Itemsets via utility lists)",
		Long:  "Mines top-k high-utility itemsets using a parallel TKO-style algorithm: speculative initial threshold + Fork/Join at top level with atomic minUtility propagation (SPMF format: items : transactionUtility : itemUtilities).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("required flag \"input\" not set")
			}
			algo := ptko.NewPTKO()
			algo.Workers = workers
			if err := algo.RunAlgorithm(input, output, k); err != nil {
				return fmt.Errorf("ptko: %w", err)
			}
			algo.PrintStats()
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "path to utility database (SPMF format)")
	cmd.Flags().StringVarP(&output, "output", "o", "output.txt", "path to write top-k HUIs")
	cmd.Flags().IntVarP(&k, "topk", "k", 8, "number of top high-utility itemsets")
	cmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "number of parallel workers (goroutines)")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}
