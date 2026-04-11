package cli

import (
	"fmt"

	"hui-problem/internal/algorithms/thui"

	"github.com/spf13/cobra"
)

func newTHUICmd() *cobra.Command {
	var (
		input     string
		output    string
		k         int
		eucsPrune bool
	)

	cmd := &cobra.Command{
		Use:   "thui",
		Short: "Run THUI (Top-K High Utility Itemsets Mining)",
		Long:  "Mines top-k high-utility itemsets using the THUI algorithm with optional EUCS pruning. Input: SPMF utility database (items : transactionUtility : itemUtilities per line).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("required flag \"input\" not set")
			}
			algo := thui.NewAlgoTHUI()
			if err := algo.RunAlgorithm(input, output, eucsPrune, k); err != nil {
				return fmt.Errorf("thui: %w", err)
			}
			algo.PrintStats()
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "path to utility database (SPMF format)")
	cmd.Flags().StringVarP(&output, "output", "o", "output.txt", "path to write top-k HUIs")
	cmd.Flags().IntVarP(&k, "topk", "k", 10, "number of top high-utility itemsets")
	cmd.Flags().BoolVarP(&eucsPrune, "eucs-prune", "e", true, "enable EUCS pruning optimization")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}
