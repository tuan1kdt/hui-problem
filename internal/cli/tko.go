package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"hui-problem/tko"
)

func newTKOCmd() *cobra.Command {
	var (
		input  string
		output string
		k      int
	)

	cmd := &cobra.Command{
		Use:   "tko",
		Short: "Run TKO-Basic (Top-K high-utility itemsets via utility lists)",
		Long:  "Mines top-k high-utility itemsets using the SPMF TKO-Basic algorithm (utility lists). Input: SPMF utility database (items : transactionUtility : itemUtilities per line).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" {
				return fmt.Errorf("required flag \"input\" not set")
			}
			algo := tko.NewAlgoTKOBasic()
			if err := algo.RunAlgorithm(input, output, k); err != nil {
				return fmt.Errorf("tko: %w", err)
			}
			algo.PrintStats()
			return nil
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "path to utility database (SPMF format)")
	cmd.Flags().StringVarP(&output, "output", "o", "output.txt", "path to write top-k HUIs")
	cmd.Flags().IntVarP(&k, "topk", "k", 8, "number of top high-utility itemsets")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}
