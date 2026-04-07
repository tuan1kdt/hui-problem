# Đánh giá bản refactor TKU Go so với Java gốc (SPMF)

## Tổng kết nhanh

| Hạng mục | Đánh giá |
| :--- | :--- |
| **Flow tổng thể (Pipeline)** | ✅ Đúng |
| **Phase 0: DatabaseInfo** | ✅ Đúng |
| **Phase 1a: Pre-Evaluation (PE)** | ✅ Đúng |
| **Phase 1b: Build UP-Tree (NU)** | ✅ Đúng |
| **Phase 1c: Sum Descendents (MD)** | ⚠️ Có khác biệt nhỏ (không ảnh hưởng kết quả) |
| **Phase 1d: UPGrowth + MC** | ✅ Đúng |
| **Phase 1e: Output P1 candidates** | ✅ Đúng |
| **Sorting Candidates (SE)** | ✅ Đúng |
| **Phase 2: Verification** | ⚠️ Có 1 lỗi logic tiềm ẩn |
| **Cấu trúc phụ trợ** | ✅ Đúng |

---

## Chi tiết đánh giá từng bước

### 1. Flow tổng thể (Pipeline) ✅

**Java (`AlgoTKU.runAlgorithm`):**
```
Phase 0 → PreEvaluation → BuildUPTree → SumDescendents → UPGrowth → Output P1 → Sort → Phase 2
```

**Go (`TKU.RunAlgorithm`):**
```
Phase 0 → preEvaluation → buildUPTree → SumDescendents → UPGrowth → Output P1 → Sort → Phase 2
```

**Kết luận:** Flow hoàn toàn khớp 1:1 với Java gốc.

---

### 2. Phase 0: DatabaseInfo ✅

| Java (`CalculateDatabaseInfo`) | Go (`DatabaseInfo`) |
| :--- | :--- |
| Quét file tính MaxID và DBSize | Quét file tính MaxID và DBSize |
| `getMaxID()`, `getDBSize()` | `GetMaxID()`, `GetDBSize()` |

**Kết luận:** Logic tương đương.

---

### 3. PreEvaluation (Chiến lược PE) ✅

Cả hai thực hiện chính xác cùng flow:
1. Đọc DB, tính TWU cho từng item.
2. Tính MinBNF (MIU) và MaxBNF (MAU) cho từng item.
3. Xây dựng Triangular Matrix: Với mỗi giao dịch, lấy item đầu tiên (`temp2[0]`) bắt cặp với mọi item còn lại (`temp2[s]`, s > 0), cộng `u0 + util` vào ma trận.
4. Gọi `getInitialUtility` để tìm giá trị thứ k lớn nhất trong ma trận.

**Java (line 348-357):**
```java
if (s > 0) {
    a.incrementCount(Integer.parseInt(temp2[0]),
            Integer.parseInt(temp2[s]),
            Integer.parseInt(temp3[0]) + Integer.parseInt(temp3[s]));
}
```

**Go (line 244-246):**
```go
if s > 0 {
    tm.IncrementCount(firstItem, itemID, u0+util)
}
```

**Kết luận:** Logic hoàn toàn tương đương.

**`getInitialUtility`:** Java sử dụng PriorityQueue (min-heap), Go sử dụng `container/heap` (min-heap). Cùng thuật toán giữ k phần tử lớn nhất, trả về giá trị nhỏ nhất trong heap (tức phần tử hạng k). ✅

---

### 4. BuildUPTree (Chiến lược NU) ✅

Cả hai triển khai `instrans3` với cùng hành vi:
- Lọc item có TWU < globalMinUtil.
- Sort transaction theo TWU giảm dần (bubble sort).
- Chèn vào cây, tích lũy TWU prefix tại mỗi node.
- **Chiến lược NU:** Khi tạo node mới hoặc cập nhật node, gọi `UpdateNodeCountHeap` để cập nhật ngưỡng min-heap → nâng `globalMinUtil`.

**Kiểm tra chi tiết `instrans3`:**

| Hành vi | Java | Go |
| :--- | :--- | :--- |
| TWU prefix tích lũy | `TWU += parseInt(bran[i])` | `twu += Atoi(bran[i])` |
| Node mới: push vào heap | `UpdateNodeCountHeap(NodeCountHeap, nNode.twu)` | `updateNodeCountHeap(nodeCountHeap, nNode.TWU)` |
| Node cũ: remove old + push new | `NodeCountHeap.remove(comp.twu)` rồi `UpdateNodeCountHeap(NCH, comp.twu + TWU)` | `nodeCountHeap.Remove(comp.TWU)` rồi `updateNodeCountHeap(NCH, comp.TWU+twu)` |
| Insert + tie-break khi TWU bằng nhau | `L1[target] == L1[comp.item] && target < comp.item` | `l1[target] == l1[comp.Item] && target < comp.Item` |

**Kết luận:** Logic hoàn toàn tương đương.

---

### 5. SumDescendents (Chiến lược MD) ⚠️ Khác biệt nhỏ

**Java (line 141-161):**
```java
RedBlackTree<Integer> DSNodeCountHeap = new RedBlackTree<Integer>(true);
for (int i = 0; i < tree.root.childlink.size(); i++) {
    int[] Sum_DS = new int[itemCount];
    int DSItem = tree.root.childlink.get(i).item;
    tree.SumDescendent(tree.root.childlink.get(i), Sum_DS);
    for (int j = 0; j < Sum_DS.length; j++) {
        if ((Sum_DS[j] != 0) && (j != DSItem)) {
            int DS_Value = (arrayMIU[j] + arrayMIU[DSItem]) * Sum_DS[j];
            UpdateNodeCountHeap(DSNodeCountHeap, DS_Value);
        }
    }
}
// Sau khi xong, Java ngay lập tức RESET DSNodeCountHeap:
DSNodeCountHeap = new RedBlackTree<Integer>(true);  // ← line 161
```

**Go (line 77-88):**
```go
dsHeap := NewIntRedBlackTree()
for i := 0; i < len(tree.Root.Children); i++ {
    sumDS := make([]int, a.itemCount)
    dsItem := tree.Root.Children[i].Item
    tree.SumDescendent(tree.Root.Children[i], sumDS)
    for j := 0; j < len(sumDS); j++ {
        if sumDS[j] != 0 && j != dsItem {
            dsVal := (a.arrayMIU[j] + a.arrayMIU[dsItem]) * sumDS[j]
            a.updateNodeCountHeap(dsHeap, dsVal)
        }
    }
}
// Go KHÔNG reset dsHeap, nhưng dsHeap cũng không được sử dụng lại sau đó
```

**Phân tích:**
- **Java** tạo `DSNodeCountHeap`, dùng nó trong vòng lặp SumDescendents (liên tục gọi `UpdateNodeCountHeap` sẽ ép `globalMinUtil` lên), rồi **reset** nó bằng `new RedBlackTree`. Heap này sau khi reset sẽ không bao giờ được dùng lại, nhưng khi nó hoạt động **bên trong vòng lặp**, nó **có tác dụng** ép `globalMinUtil` lên.
- **Go** cũng làm đúng tương tự: `dsHeap` hoạt động bên trong vòng lặp để ép `globalMinUtil` thông qua `updateNodeCountHeap`. Sau vòng lặp, `dsHeap` không được sử dụng nữa (tương đương việc Java reset nó).

**Kết luận:** Logic tương đương. Việc Java reset heap chỉ để giải phóng bộ nhớ, không ảnh hưởng kết quả thuật toán vì heap đã hoàn thành nhiệm vụ ép ngưỡng.

---

### 6. UPGrowth + UPGrowth_MinBNF (Chiến lược MC) ✅

Đây là phần phức tạp nhất. Cả hai phiên bản đều triển khai:

1. **Duyệt bottom-up trên Header Table**
2. **Xây Conditional Pattern Base (CPB):** Traverse horizontal link → lên parent → thu gom path
3. **Loại bỏ item cục bộ yếu:** `LocalF1[j] < globalMinUtil → -1`
4. **Output candidate + MAU filter:** Tính `SumMau * localCount[j]`, nếu `>= globalMinUtil` → ghi file
5. **MC strategy:** Tính `MIU = SumMiu * localCount[j]`, nếu `> globalMinUtil` → push vào `ISNodeCountHeap`
6. **Xây local tree:** Loại item yếu khỏi CPB, trừ `CPBW`, sort lại, chèn vào tree con bằng `insPatternBase`
7. **Đệ quy:** Gọi `UPGrowth_MinBNF` trên tree con

| Bước | Java | Go |
| :--- | :--- | :--- |
| Traverse horizontal link | `chlink.hlink` | `chlink.HLink` |
| Build path lên root | `cptr.parentlink` | `cptr.Parent` |
| Remove phần tử đầu path | `path.remove(0)` | `path = path[1:]` |
| Accumulate LocalF1/LocalCount | `+= chlink.twu / chlink.count` | `+= chlink.TWU / chlink.Count` |
| MAU filter trước output | Có | Có |
| MC: push MIU > globalMinUtil | `UpdateNodeCountHeap(ISNodeCountHeap, MIU)` | `updateNodeCountHeap(isNodeCountHeap, miu)` |
| InsPatternBase cho local tree | Có (DGN strategy) | Có (DGN strategy) |

**Kết luận:** Logic tương đương.

---

### 7. Sort Candidates (Chiến lược SE) ✅

**Java (`runSortHUIAlgorithm`):** Đọc candidates → Nạp vào RedBlackTree<StringPair> → Pop maximum liên tiếp → Ghi ra file giảm dần.

**Go (`runSortHUIAlgorithm`):** Logic tương tự → RedBlackTree[StringPair] → PopMaximum → Ghi file.

**Kết luận:** Tương đương.

---

### 8. Phase 2: Verification ⚠️ Có 1 lỗi logic tiềm ẩn

**Java (`AlgoPhase2OfTKU.readCandidateItemsets`, line 191-242):**
```java
if (Integer.parseInt(CI[1]) >= minUtility) {  // Kiểm tra estimated utility
    for (int i = 0; i < num_trans; i++) {     // Quét DB
        // ...tính EUtility thực tế...
    }
    if (EUtility >= minUtility) {
        Lbfw.write(CI[0] + ":" + EUtility);
        updateHeap(Heap, CI[0], EUtility);    // CẬP NHẬT minUtility
    }
}
```

**Java updateHeap (line 292-309):**
```java
void updateHeap(RedBlackTree<StringPair> NCH, String HUI, int Utility) {
    if (NCH.size() < theCurrentK) {
        NCH.add(new StringPair(HUI, Utility));
    } else if (NCH.size() >= theCurrentK) {
        if (Utility > minUtility) {           // ← So sánh với minUtility
            NCH.add(new StringPair(HUI, Utility));
            NCH.popMinimum();
        }
    }
    if ((NCH.minimum().y > minUtility) && (NCH.size() >= theCurrentK)) {
        minUtility = NCH.minimum().y;         // ← CẬP NHẬT minUtility
    }
}
```

**Go (`Phase2.readCandidateItemsets`, line 149-182):**
```go
if estUtil < p.minUtility {
    continue                                  // ← Skip
}
// ...tính eUtility thực tế...
if eUtility >= p.minUtility {
    lbfw.WriteString(ci[0] + ":" + strconv.Itoa(eUtility))
    p.updateHeap(h, ci[0], eUtility)          // CẬP NHẬT p.minUtility
}
```

**Go updateHeap (line 231-246):**
```go
func (p *Phase2) updateHeap(nch *RedBlackTree[StringPair], hui string, utility int) {
    if nch.Size() < p.theCurrentK {
        nch.Add(StringPair{X: hui, Y: utility})
    } else if nch.Size() >= p.theCurrentK {
        if utility > p.minUtility {
            nch.Add(StringPair{X: hui, Y: utility})
            nch.PopMinimum()
        }
    }
    if nch.Size() >= p.theCurrentK {
        minP := nch.Minimum()
        if minP.Y > p.minUtility {
            p.minUtility = minP.Y
        }
    }
}
```

**🐛 Nhận xét quan trọng:**

Java bản gốc có một **sai sót tinh vi** ở chiến lược SE: Bước `runSortHUIAlgorithm` đã **sắp xếp candidates sẽ giảm dần utility** trước khi ghi vào `sortedTopKcandidateFile`. Tuy nhiên, Phase 2 khi đọc file sorted này và tính exact utility, nó **không** dừng sớm (early termination) khi gặp candidate có `estUtil < minUtility`.  Thay vào đó nó chỉ `continue` bỏ qua candidate đó.

Cả Java và Go đều xử lý giống nhau ở đây. **Go đã refactor đúng logic gốc.**

Tuy nhiên, có một **khác biệt nhỏ nhưng đáng chú ý** ở Phase 2:

> **Java** gọi `Match_Count` riêng rồi kiểm tra `Match_Count == candidate.length` để cộng dồn utility. **Go** dùng biến `allMatch` boolean, nếu bất kỳ item nào không match thì `break` ngay và set `pUtility = 0`.

Hai cách viết **tương đương logic**, nhưng Go clean hơn.

**Một vấn đề nhỏ ở Go Phase 2:** `bfw.Flush()` được gọi tại dòng 55 nhưng Java gốc gọi `Lbfw.flush()` tại dòng 246 (sau vòng while). Go gọi flush **trước khi đóng file**, Java gọi flush **trong hàm readCandidateItemsets**. Tuy nhiên, Go sau đó gọi `bfw.Flush()` trước `tmp.Close()`, nên kết quả vẫn đúng vì buffer đã được flush đủ data. ✅

---

### 9. Cấu trúc phụ trợ ✅

| Component | Java | Go | Match? |
| :--- | :--- | :--- | :--- |
| **TriangularMatrix** | `TKUTriangularMatrix` | `TriangularMatrix` | ✅ Giống nhau (allocation, incrementCount, getSupportForItems) |
| **StringPair** | `StringPair` (compareTo: `this.y - o.y`) | `StringPair` + `CompareStringPair` | ✅ Giống nhau |
| **RedBlackTree** | Sử dụng SPMF RBT (allowDuplicates=true) | Go generics RBT (allowDup=true) | ✅ API tương đương  |
| **TreeNode** | `treenode` {item, count, twu, hlink, parentlink, childlink} | `TreeNode` {Item, Count, TWU, HLink, Parent, Children} | ✅ |

---

## Tổng kết

### ✅ Những điểm Go refactor đúng:
1. **Flow pipeline** hoàn toàn khớp Java gốc: Phase 0 → PE → BuildTree (NU) → MD → UPGrowth (MC) → Sort (SE) → Phase 2
2. **5 chiến lược tăng tốc** (PE, NU, MD, MC, SE) đều được cài đặt đúng
3. **Cấu trúc dữ liệu** (TreeNode, TriangularMatrix, RedBlackTree) tương đương
4. **DGN strategy** trong `insPatternBase` (local tree) hoạt động đúng
5. **Error handling** tốt hơn Java (Go trả lỗi thay vì throw exception)
6. **Memory tracking** hợp lý (dùng `runtime.ReadMemStats`)

### ⚠️ Những điểm cần lưu ý (không ảnh hưởng kết quả):
1. **dsHeap sau SumDescendents không được reset** — nhưng không ảnh hưởng vì không dùng lại
2. **Phase 2 không có early termination** — đúng với Java gốc (Java gốc cũng không có)
3. Go sử dụng `float64` cho `globalMinUtil` trong khi Java dùng `double` — tương đương về mặt precision

### Kết luận cuối cùng:
> **Bản Go refactor đã triển khai đúng flow thuật toán gốc TKU.** Tất cả các bước từ PreEvaluation, xây cây UP-Tree, SumDescendents, UPGrowth/UPGrowth_MinBNF, đến Phase 2 verification đều khớp logic 1:1 với Java reference. Không phát hiện lỗi sai logic nào ảnh hưởng đến tính đúng đắn của kết quả thuật toán.
