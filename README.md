# hui-problem

This repository is a **Go** toolkit for **high-utility itemset mining**: finding itemsets whose total utility in a transaction database is among the highest, without fixing a minimum utility threshold in advance.

The first algorithm provided is **TKU** (Top-K High Utility itemsets), implemented in the [`tku`](tku/) package and exposed through the [`hui-problem`](cmd/hui-problem/) CLI.

---

## The problem

In many applications (e.g. retail analytics), each transaction lists **items** and each item has a **utility** (profit, importance, or weight). The **utility of an itemset** is the sum of the utilities of its items in every transaction where **all** items of the set appear (the classic *transaction-weighted utility* model used in SPMF).

**High-utility itemset mining** asks: which itemsets have utility at least **minUtil**? The difficulty is that the search space is huge and utility is not anti-monotone like support, so pruning must use upper bounds (e.g. transaction-weighted utility, remaining utility).

A related question is **top-k** mining: *“Give me the k itemsets with the largest utility”* without choosing **minUtil** up front. That is harder because the cutoff is unknown until you explore the data—you need a **dynamic minimum utility** that rises as better candidates are found.

---

## What is TKU?

**TKU** (*Top-K high-utility itemset mining*) is an algorithm by **Tseng, Wu, Fournier-Viger, and Yu** (IEEE TKDE, 2016): *“Efficient Algorithms for Mining Top-K High Utility Itemsets.”*

It targets exactly the **top-k** setting: output (approximately) the **k** highest-utility itemsets, using strategies to:

- Estimate and **raise** a **border** minimum utility as candidates improve.
- **Prune** branches that cannot reach the current top-k.
- Work in phases so that heavy verification happens only on promising candidates.

This codebase follows the **SPMF** reference behavior (utility transaction database format and candidate pipeline). The Java reference used in development lives under [`reference/`](reference/).

---

## How TKU solves it (high level)

Conceptually, TKU combines **compact data representation**, **utility-aware tree growth**, and **verification**:

1. **Database scan and pre-evaluation**  
   Compute per-item statistics (e.g. transaction-weighted utility, min/max item utilities), build structures that summarize **pairwise** utility information, and derive an **initial** minimum utility from the **k** strongest **2-item** signals. That gives a first threshold without knowing global top-k itemsets yet.

2. **Phase 1 – UP-Tree / UP-Growth style mining**  
   Build a **utility pattern tree** over transactions filtered and ordered by current utilities, then **grow** conditional trees (similar in spirit to FP-Growth but with utility). Candidate itemsets with estimated utilities are written out; the **border** minimum utility is **updated** whenever enough evidence shows the k-th best estimate has increased.

3. **Sort candidates**  
   Order generated candidates (by estimated utility) for the next phase.

4. **Phase 2 – exact verification**  
   Load the original database, scan transactions for each surviving candidate, compute **exact** utility, and emit final **top-k** high-utility itemsets that meet the final threshold, in **SPMF-style** output (`itemset #UTIL: value`).

So TKU **does not** require you to guess **minUtil**; it **learns** a running cutoff and **focuses** exact counting on candidates that remain competitive for the top **k** places.

---

## Build and CLI

```bash
go build -o hui-problem ./cmd/hui-problem
```

Run TKU on an SPMF utility database (`items : transactionUtility : itemUtilities` per line):

```bash
./hui-problem tku -i path/to/db.txt -o out.txt --topk 8
```

- `-i` / `--input` — utility database (required)  
- `-o` / `--output` — output file (default `output.txt`)  
- `-k` / `--topk` — number of top itemsets to mine (default `3`)

```bash
go test ./tku
```

Sample data: [`testdata/DB_Utility.txt`](testdata/DB_Utility.txt).

---

## References

- V. S. Tseng, C.-W. Wu, P. Fournier-Viger, and P. S. Yu, **“Efficient Algorithms for Mining Top-K High Utility Itemsets,”** *IEEE Transactions on Knowledge and Data Engineering*, vol. 28, no. 1, pp. 54–67, 2016.
- SPMF: [http://www.philippe-fournier-viger.com/spmf/](http://www.philippe-fournier-viger.com/spmf/) (original TKU integration and file formats).
