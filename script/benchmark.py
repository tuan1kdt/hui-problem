#!/usr/bin/env python3
"""Benchmark runner for HUI-problem algorithms.

Runs TKU, TKO, PTKU, and THUI on configurable datasets and k values,
parses each algorithm's stdout stats, and prints aggregated comparison tables.
"""

import argparse
import csv
import os
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

ALGORITHMS = ["tku", "tko", "ptku", "thui"]

DEFAULT_DATASETS = [
    "testdata/mushroom_utility_SPMF.txt",
    "testdata/retail_utility_spmf.txt",
]

DEFAULT_K_VALUES = [1, 10, 100, 1000]


# ---------------------------------------------------------------------------
# Parsing helpers
# ---------------------------------------------------------------------------

def parse_stats(algo: str, stdout: str) -> dict:
    """Extract runtime (seconds), memory (MB), and itemset count from stdout."""
    stats = {"runtime_s": None, "memory_mb": None, "itemset_count": None}

    if algo == "tku":
        m = re.search(r"Total execution time\s*:\s*([\d.]+)\s*seconds", stdout)
        if m:
            stats["runtime_s"] = float(m.group(1))
        m = re.search(r"Max memory usage \(approx\):\s*([\d.]+)\s*MB", stdout)
        if m:
            stats["memory_mb"] = float(m.group(1))
        m = re.search(r"Number of top-k high utility patterns\s*:\s*(\d+)", stdout)
        if m:
            stats["itemset_count"] = int(m.group(1))

    elif algo == "tko":
        m = re.search(r"Total time\s*~\s*([\d.eE+-]+)\s*s", stdout)
        if m:
            stats["runtime_s"] = float(m.group(1))
        m = re.search(r"Memory\s*~\s*([\d.eE+-]+)\s*MB", stdout)
        if m:
            stats["memory_mb"] = float(m.group(1))
        m = re.search(r"High-utility itemsets count\s*:\s*(\d+)", stdout)
        if m:
            stats["itemset_count"] = int(m.group(1))

    elif algo == "ptku":
        m = re.search(r"Total execution time\s*:\s*([\d.eE+-]+)\s*s", stdout)
        if m:
            stats["runtime_s"] = float(m.group(1))
        m = re.search(r"Max memory usage \(approx\)\s*:\s*([\d.eE+-]+)\s*MB", stdout)
        if m:
            stats["memory_mb"] = float(m.group(1))
        m = re.search(r"Number of top-k high utility patterns\s*:\s*(\d+)", stdout)
        if m:
            stats["itemset_count"] = int(m.group(1))

    elif algo == "thui":
        m = re.search(r"Total time\s*~\s*(\d+)\s*ms", stdout)
        if m:
            stats["runtime_s"] = int(m.group(1)) / 1000.0
        m = re.search(r"Memory\s*~\s*([\d.]+)\s*MB", stdout)
        if m:
            stats["memory_mb"] = float(m.group(1))
        m = re.search(r"High-utility itemsets count\s*:\s*(\d+)", stdout)
        if m:
            stats["itemset_count"] = int(m.group(1))

    return stats


def parse_output_file(filepath: str) -> list:
    """Parse algorithm output file into [(frozenset_of_items, utility), ...]."""
    results = []
    if not os.path.isfile(filepath):
        return results
    with open(filepath) as f:
        for line in f:
            line = line.strip()
            if not line or "#UTIL:" not in line:
                continue
            parts = line.split("#UTIL:")
            items = frozenset(int(x) for x in parts[0].split() if x)
            utility = int(parts[1].strip())
            results.append((items, utility))
    return results


# ---------------------------------------------------------------------------
# Runner
# ---------------------------------------------------------------------------

def run_single(binary: str, algo: str, dataset: str, k: int,
               output_dir: str, timeout: int) -> dict:
    """Run one algorithm and return parsed results."""
    ds_stem = Path(dataset).stem
    out_path = os.path.join(output_dir, f"{algo}-{ds_stem}-k{k}.txt")
    cmd = [binary, algo, "-i", dataset, "-o", out_path, "-k", str(k)]

    result = {
        "algo": algo,
        "dataset": ds_stem,
        "k": k,
        "runtime_s": None,
        "memory_mb": None,
        "itemset_count": None,
        "itemsets": [],
        "error": None,
    }

    try:
        proc = subprocess.run(
            cmd, capture_output=True, text=True, timeout=timeout,
        )
        if proc.returncode != 0:
            result["error"] = proc.stderr.strip() or f"exit code {proc.returncode}"
            print(f"  [FAIL] {algo} k={k}: {result['error']}", file=sys.stderr)
        else:
            stats = parse_stats(algo, proc.stdout)
            result.update(stats)
            result["itemsets"] = parse_output_file(out_path)

    except subprocess.TimeoutExpired:
        result["error"] = f"timeout after {timeout}s"
        print(f"  [TIMEOUT] {algo} k={k} on {ds_stem}", file=sys.stderr)

    return result


# ---------------------------------------------------------------------------
# Output formatting
# ---------------------------------------------------------------------------

def fmt_val(v, precision=2):
    """Format a numeric value or return '-' if None."""
    if v is None:
        return "-"
    if isinstance(v, int):
        return str(v)
    return f"{v:.{precision}f}"


def print_tables(all_results: list, algos: list):
    """Print aggregated comparison tables grouped by dataset."""
    # Group: dataset -> k -> algo -> result
    grouped = defaultdict(lambda: defaultdict(dict))
    for r in all_results:
        grouped[r["dataset"]][r["k"]][r["algo"]] = r

    col_w = 10

    for ds in sorted(grouped):
        print(f"\n{'=' * 60}")
        print(f"  Dataset: {ds}")
        print(f"{'=' * 60}")

        k_values = sorted(grouped[ds])

        # --- Runtime table ---
        print(f"\n  Runtime (seconds):")
        header = f"  {'k':>{col_w}}" + "".join(f"{a:>{col_w}}" for a in algos)
        print(header)
        print("  " + "-" * (col_w * (1 + len(algos))))
        for k in k_values:
            row = f"  {k:>{col_w}}"
            for a in algos:
                r = grouped[ds][k].get(a)
                val = fmt_val(r["runtime_s"] if r else None)
                row += f"{val:>{col_w}}"
            print(row)

        # --- Memory table ---
        print(f"\n  Peak Memory (MB):")
        header = f"  {'k':>{col_w}}" + "".join(f"{a:>{col_w}}" for a in algos)
        print(header)
        print("  " + "-" * (col_w * (1 + len(algos))))
        for k in k_values:
            row = f"  {k:>{col_w}}"
            for a in algos:
                r = grouped[ds][k].get(a)
                val = fmt_val(r["memory_mb"] if r else None)
                row += f"{val:>{col_w}}"
            print(row)

        # --- Itemset count table ---
        print(f"\n  Itemset Count:")
        header = f"  {'k':>{col_w}}" + "".join(f"{a:>{col_w}}" for a in algos)
        print(header)
        print("  " + "-" * (col_w * (1 + len(algos))))
        for k in k_values:
            row = f"  {k:>{col_w}}"
            for a in algos:
                r = grouped[ds][k].get(a)
                val = fmt_val(r["itemset_count"] if r else None)
                row += f"{val:>{col_w}}"
            print(row)

    print()


def write_csv(all_results: list, csv_path: str):
    """Write flat CSV with one row per (dataset, algo, k)."""
    fields = ["dataset", "algo", "k", "runtime_s", "memory_mb",
              "itemset_count", "error"]
    with open(csv_path, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields, extrasaction="ignore")
        w.writeheader()
        for r in all_results:
            w.writerow({k: r.get(k) for k in fields})
    print(f"CSV written to {csv_path}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def parse_k_values(raw: list[str]) -> list[int]:
    """Accept space-separated or comma-separated k values."""
    values = []
    for item in raw:
        for part in item.split(","):
            part = part.strip()
            if part:
                values.append(int(part))
    return sorted(set(values))


def main():
    parser = argparse.ArgumentParser(
        description="Benchmark HUI-problem algorithms and aggregate results.",
    )
    parser.add_argument("--binary", default="./hui-problem",
                        help="path to compiled binary (default: ./hui-problem)")
    parser.add_argument("--datasets", nargs="+", default=DEFAULT_DATASETS,
                        help="dataset file paths")
    parser.add_argument("--k-values", nargs="+", default=["1,10,100,1000"],
                        help="top-k values (comma or space separated)")
    parser.add_argument("--algos", nargs="+", default=ALGORITHMS,
                        help="algorithms to run")
    parser.add_argument("--output-dir", default="outputs",
                        help="directory for algorithm output files")
    parser.add_argument("--csv", default=None,
                        help="optional CSV output path")
    parser.add_argument("--timeout", type=int, default=600,
                        help="timeout per run in seconds (default: 600)")
    args = parser.parse_args()

    # Handle comma-separated algos
    algos = []
    for a in args.algos:
        algos.extend(a.split(","))
    algos = [a.strip() for a in algos if a.strip()]

    k_values = parse_k_values(args.k_values)

    # Validate binary
    if not os.access(args.binary, os.X_OK):
        print(f"Error: binary not found or not executable: {args.binary}",
              file=sys.stderr)
        sys.exit(1)

    os.makedirs(args.output_dir, exist_ok=True)

    all_results = []
    total_runs = len(args.datasets) * len(k_values) * len(algos)
    run_idx = 0

    for dataset in args.datasets:
        ds_stem = Path(dataset).stem
        if not os.path.isfile(dataset):
            print(f"Warning: dataset not found: {dataset}", file=sys.stderr)
            continue

        print(f"\n--- {ds_stem} ---")

        for k in k_values:
            for algo in algos:
                run_idx += 1
                print(f"  [{run_idx}/{total_runs}] {algo} k={k} ... ",
                      end="", flush=True)
                r = run_single(args.binary, algo, dataset, k,
                               args.output_dir, args.timeout)
                all_results.append(r)
                if r["error"]:
                    print(f"ERROR ({r['error']})")
                else:
                    print(f"{fmt_val(r['runtime_s'])}s, "
                          f"{fmt_val(r['memory_mb'])}MB, "
                          f"{r['itemset_count']} itemsets")

    if not all_results:
        print("No results collected.", file=sys.stderr)
        sys.exit(1)

    print_tables(all_results, algos)

    if args.csv:
        write_csv(all_results, args.csv)


if __name__ == "__main__":
    main()
