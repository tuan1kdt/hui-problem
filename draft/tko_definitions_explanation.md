Ký hiệu $\prec$ (đọc là *precede* - đứng trước/ưu tiên trước) là một toán tử quan hệ được định nghĩa tại **Định nghĩa 17 (Precede and succeed)** trong bài báo, dùng để xác định một **"trật tự sắp xếp toàn phần"** (total order) giữa các items.

Trong bài toán HUI (hay cụ thể là trong TKO/HUI-Miner), để thuật toán hoạt động và cắt tỉa hiệu quả, tất cả các items *bắt buộc* phải được xếp hàng theo một quy tắc phân cấp thống nhất.

Ký hiệu $I_i \prec I_j$ có nghĩa là **Item $I_i$ được sắp xếp đứng trước Item $I_j$** nếu thỏa mãn **1 trong 2** điều kiện sau:
1. **$TWU(I_i) < TWU(I_j)$**: Độ đánh giá hữu ích trọng số giao dịch (TWU) của $I_i$ nhỏ hơn của $I_j$.
2. Nếu hai item có hòa $TWU$ ($TWU(I_i) = TWU(I_j)$) thì so sánh theo **thứ tự từ điển (lexicographical order)**. Cái nào đứng trước trong danh sách bảng chữ cái thì được xếp trước. (Ví dụ: Nếu item "A" và "B" cùng có TWU là 10, thì "A" $\prec$ "B" vì chữ 'A' đứng trước chữ 'B' trong bảng chữ cái).

**Điều này áp dụng vào thực tế thuật toán như thế nào?**
Ở Bước 1 của thuật toán TKO, sau khi tính xong toàn bộ $TWU$ của Database, thuật toán sẽ áp dụng định nghĩa $\prec$ này để lên thói quen **"sắp xếp lại phần tử trong mọi giao dịch"**.

Ví dụ bạn có một giao dịch thô: $T_1 = \{D, A, C \}$
Khi xét TWU, nếu $C \prec A \prec D$, thuật toán sẽ xáo trộn và xếp lại giao dịch này thành: $T_1' = \{C, A, D \}$.

Từ đó về sau (ở Bước 2, Bước 3), mọi khái niệm *"mở rộng"* (Định nghĩa 18) hay *"xuất hiện sau"* (Định nghĩa 19) để tính Remaining Utility đều dựa trên mốc trật tự tĩnh đã được neo bởi dấu $\prec$ ngay từ đầu!


Dựa vào nội dung từ tài liệu PDF `16. Efficient Algorithms for Mining Top-K High Utility Itemsets` mà bạn đề cập, hai định nghĩa số 18 và 19 là các khái niệm cốt lõi phục vụ thuật toán TKO. Đặc biệt, chúng đặt nền móng cho việc duyệt cây không gian và tính toán lượng **Remaining Utility (rutils)** của cấu trúc Utility-List.

Giải thích chi tiết:

### 1. Định nghĩa 18 (Concatenation of an itemset - Phép mở rộng/nối của một tập mục)
> **Phát biểu gốc:** Let $X = \{x_1, x_2, ..., x_u\}$ and $Y = \{y_1, y_2, ..., y_v\}$ be itemsets, $Y$ is a concatenation of $X$ iff $X \subset Y$ and each item $y_j \notin X$ succeeds all items in $X$.

**Giải thích:**
- Một tập itemset $Y$ được gọi là **phần mở rộng (concatenation)** của tập $X$ nếu nó thỏa mãn 2 điều kiện:
    1. Tập $X$ là tập con thực sự (nằm trọn) bên trong $Y$.
    2. Mọi phần tử có trong $Y$ nhưng không có mặt trong $X$ ($y_j \in Y \setminus X$) đều phải nằm ở phía sau tất cả các phần tử của $X$, dựa theo quy tắc thứ tự sắp xếp (thường là sắp xếp tăng dần theo TWU).
- **Ý nghĩa trong thuật toán TKO:** Khi thuật toán đang xét một node $X$ (ví dụ: `X = {A,B}`), nó chỉ ghép (concatenate) $X$ với các item đứng bên phải nó (ví dụ item `C`, `D` để sinh ra `{A,B,C}` hoặc `{A,B,D}`) chứ không ghép lại với các item đứng ở đằng trước để sinh ra nhánh dư thừa. Điều này giúp tránh trùng lặp và giới hạn không gian cây tìm kiếm (search tree space) trở nên tuyến tính nhất (đây là cách sinh cây của họ thuật toán *HUI-Miner* và *FP-Growth*).

---

### 2. Định nghĩa 19 (Appear after - Các phần tử xuất hiện phía sau)
> **Phát biểu gốc:** Given a finite set of items $I$ and a total order $\prec$ on all items. (...) An item $I_j$ appears after an itemset $X = \{x_1, x_2, ..., x_L\}$ in a transaction $T_r$ iff $I_j \in T_r$ and $x_1 \prec x_2 \prec ... \prec x_L \prec I_j$. The set of all the items appearing after $X$ in $T_r$ is denoted as $T_r / X$.

**Giải thích:**
- Định nghĩa này chỉ ra trong một giao dịch cụ thể $T_r$, một vật phẩm (item) $I_j$ được tính là **xuất hiện sau** tập $X$ nếu như:
    1. $I_j$ có mặt trong giao dịch $T_r$.
    2. Xét theo quy tắc sắp xếp $\prec$, vật phẩm $I_j$ phải mang cấp bậc cao hơn/đứng sau vật phẩm tận cùng của tập $X$ (tức là $x_L \prec I_j$).
- Tập hợp bao gồm tất cả những $I_j$ như thế trong giao dịch $T_r$ được ký hiệu là **$T_r / X$**.
- **Ý nghĩa trong thuật toán TKO:** Nó được dùng để thiết lập giá trị **Remaining Utility (rutils)** của `Utility List`. Khi thuật toán tính toán độ hữu ích tiềm năng (để quyết định xem có cắt bỏ nhánh đi không), nó lấy tổng tiền của tất cả các phần tử nằm trong tập hợp "$T_r / X$" này. Những item nào mà bị đứng trước thì không được cộng dồn vào tiềm năng ở các bước sau vì đường nào nó cũng không thể ghép thành phần mở rộng/concatenation được nữa (nhờ vào cơ chế đã ghim ở Định nghĩa 18).

Tóm lại, Định nghĩa 18 định hướng **cách sinh ứng viên**, còn Định nghĩa 19 cung cấp phép chiếu để giới hạn các item nằm ở phần rutils. 

---

### 3. Định nghĩa 20 (Remaining utility of an itemset in a transaction - Độ hữu ích còn lại trong một giao dịch)
> **Phát biểu gốc:** The remaining utility of an itemset $X$ in a transaction $T_r$ is defined as $RU(X, T_r) = \sum_{I_i \in T_r / X} EU(I_i, T_r)$.

**Giải thích:**
- Giá trị này được tính bằng cách lấy tổng độ hữu ích (Exact Utility - $EU$) của tất cả các phần tử $I_i$ thuộc tập "$T_r / X$" (tức là những phần tử xuất hiện TRONG giao dịch $T_r$ và thỏa mãn điều kiện "xuất hiện/đứng SAU" mặt chữ của tập $X$ theo trật tự $\prec$ đã định ở Định nghĩa 19).
- **Ý nghĩa:** Đây là lượng "tiềm năng" tối đa mà tập $X$ có thể gặt hái thêm bằng cách tiếp tục kết hợp (concatenate) với các phần tử ở phía sau trong mô hình cây tìm kiếm.

---

### 4. Định nghĩa 21 (Remaining utility of an itemset in a database - Độ hữu ích còn lại trong toàn bộ CSDL)
> **Phát biểu gốc:** The remaining utility of an itemset $X$ in a database $D$ is defined as $RU(X) = \sum_{T_r \in D} RU(X, T_r)$.

**Giải thích:**
- Độ hữu ích còn lại tổng thể của $X$ (Remaining Utility tổng) chính là tổng cộng của tất cả các độ hữu ích còn lại $RU(X, T_r)$ đo được riêng biệt ở từng giao dịch $T_r$ mà $X$ xuất hiện.
- **Ý nghĩa:** Định nghĩa này rất quan trọng bởi vì nó xây dựng nên một Upper Bound (cận trên) vững chắc. Một nhánh DFS của $X$ được giữ lại hay bị chặt bỏ phụ thuộc hoàn toàn vào bất đẳng thức: $EU(X) + RU(X) \geq min\_utility$. Nếu vế trái nhỏ hơn ngưỡng $min\_utility$, nhánh của $X$ bị Cắt tỉa (Pruning) do phần "thực tế" cộng với "tiềm năng tương lai" cũng không thể với tới độ phổ biến yêu cầu.

---

### 5. Định nghĩa 22 (Utility-list structure - Cấu trúc đồ thị danh sách Utility-List)
> **Phát biểu gốc:** The utility-list of an itemset $X$, denoted as $ul(X)$ is a list containing $|g(X)|$ tuples of the form $\langle tid, iutil, rutil \rangle$. Each tuple is called an element and maintains the information of $X$ in a transaction $T_r$. An element with respect to the transaction $T_r$ contains the information $\langle r, EU(X, T_r), RU(X, T_r) \rangle$.

**Giải thích:**
- `$ul(X)$` (Utility list của tập $X$) là một chuỗi tuyến tính, mỗi dòng record (được gọi là một tuple/element) đại diện cho thông tin thu thập được của tập $X$ tại một giao dịch $T_r$ xác định.
- Mỗi phần tử (element) gói gọn 3 giá trị then chốt:
  1. `tid` (hay $r$): Mã chỉ mục của giao dịch $T_r$ (ví dụ: giao dịch ID = 1, 2...). Định danh này có chức năng giúp đối chiếu giao tuyến khi gộp 2 List với nhau (hai mảng giao dịch khớp `tid` mới được phép nối).
  2. `iutil` (chính là $EU(X, T_r)$): Độ hữu ích chính xác hiện tại của tập $X$ chỉ tính riêng trong $T_r$ đó. 
  3. `rutil` (chính là $RU(X, T_r)$ được trình bày ở Định nghĩa 20): Mức độ tiềm năng còn sót lại phía sau đuôi của $X$.
- **Ý nghĩa:** Hàng loạt thuật toán được hưởng lợi (như HUI-Miner, TKO) và có danh xưng "One phase algorithm" là nhờ chính sự ra đời của cấu trúc Utility List kinh điển này! TKO không cần phải mất thời gian tính toán và quét rà lại CSDL để xác định Candidate có phải HUI không, mà chỉ cần dựa trên việc dùng thuật toán chập (giao nhau bằng Binary Search / Hai con trỏ trên List) giữa hai `Utility List` nhỏ để đẻ ra `Utility List` cấp cao hơn là biết ngay được số phận của Itemset đó. Điều này giúp tối ưu hóa hiệu năng RAM cũng như thời gian thi hành.