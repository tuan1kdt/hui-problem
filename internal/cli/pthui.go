package cli

import (
	"fmt"
	//"runtime"
	"hui-problem/internal/algorithms/pthui"

	"github.com/spf13/cobra"
)

func newPTHUICmd() *cobra.Command {
	var (
		input     string
		output    string
		k         int
		eucsPrune bool
	)

	cmd := &cobra.Command{
		Use:   "pthui",
		Short: "Run PTHUI (Parallel Top-K High Utility Itemsets Mining)",
		Long:  "Mines top-k high-utility itemsets using the Parallel THUI algorithm with optional EUCS pruning. Input: SPMF utility database (items : transactionUtility : itemUtilities per line).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("required flag \"input\" not set")
			}
			algo := pthui.NewAlgoPTHUI()
			//algo.Workers = workers
			if err := algo.RunAlgorithm(input, output, eucsPrune, k); err != nil {
				return fmt.Errorf("pthui: %w", err)
			}
			algo.PrintStats()
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "path to utility database (SPMF format)")
	cmd.Flags().StringVarP(&output, "output", "o", "output.txt", "path to write top-k HUIs")
	cmd.Flags().IntVarP(&k, "topk", "k", 10, "number of top high-utility itemsets")
	cmd.Flags().BoolVarP(&eucsPrune, "eucs-prune", "e", true, "enable EUCS pruning optimization")
	//cmd.Flags().IntVarP(&workers, "workers", "w", runtime.NumCPU(), "number of parallel workers")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}
