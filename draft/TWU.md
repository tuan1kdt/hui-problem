Để hiểu thấu đáo về **TWU (Transaction-Weighted Utilization)**, chúng ta cần đi từng bước từ các công thức cơ sở cấu thành nên nó. Giá trị của TWU nằm ở chỗ nó giải quyết được bài toán khó nhất trong việc khai phá High Utility Itemset: **Tính chất chống đơn điệu (Anti-monotone property)**. 

Dưới đây mình sẽ giải thích thật chi tiết kèm theo ví dụ cụ thể bám sát bài báo.

---

### 1. Bước đệm: Khái niệm TU (Transaction Utility)
Trước khi tính TWU, ta phải tính **TU (Tiện ích của toàn bộ một giao dịch)**.
- **Định nghĩa:** $TU(T_r)$ là tổng lợi nhuận (utility) của **tất cả** các sản phẩm được mua trong một biên lai / giao dịch $T_r$.
- **Công thức:** 
  $$TU(T_r) = \sum_{I_j \in T_r} EU(I_j, T_r)$$
  *(Nghĩa là: lấy lợi nhuận đơn vị nhân với số lượng mua của từng món, rồi cộng dồn lại cho cả biên lai).*

### 2. Định nghĩa TWU (Transaction-Weighted Utilization)
- **Định nghĩa:** TWU của một tập các mặt hàng $X$ là **tổng giá trị TU của tất cả biên lai nào có chứa tập $X$**. 
- **Công thức:**
  $$TWU(X) = \sum_{X \subseteq T_r \wedge T_r \in D} TU(T_r)$$
- **Nghĩa là:** Để tính $TWU(X)$, bạn tìm xem $X$ xuất hiện trong các giao dịch nào. Sau đó, bạn lấy giá trị $TU$ của các giao dịch đó bốc lên cộng lại với nhau. (Bạn "mượn" tổng giá trị của cả biên lai gán luôn cho $X$).

---

### 3. Ví dụ tính toán cụ thể

Hãy xét một bảng giá (**Profit Table**) và một cơ sở dữ liệu gồm 5 giao dịch (**Database**):

**A. Bảng Giá (Unit Profit):**
| Mặt hàng | A | B | C | D | E |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Giá ($)** | 5 | 2 | 1 | 2 | 3 |

**B. Dữ liệu Giao Dịch (Database):**
*(Gồm chi tiết biên lai và giá trị TU của từng biên lai đã được tính sẵn)*

| Biên lai (TID) | Chi tiết: (Mã Hàng, Số lượng) | Cách tính $TU(T_r)$ | **$TU$** |
| :--- | :--- | :--- | :--- |
| **T1** | (A, 1), (C, 1), (D, 1) | $(1 \times 5) + (1 \times 1) + (1 \times 2)$ | **8** |
| **T2** | (A, 2), (C, 6), (E, 2) | $(2 \times 5) + (6 \times 1) + (2 \times 3)$ | **22** |
| **T3** | (A, 1), (B, 2), (D, 6) | $(1 \times 5) + (2 \times 2) + (6 \times 2)$ | **21** |
| **T4** | (B, 4), (C, 3) | $(4 \times 2) + (3 \times 1)$ | **11** |
| **T5** | (C, 2), (E, 1) | $(2 \times 1) + (1 \times 3)$ | **5** |

#### 👉 Bài tập 1: Tính $TWU$ cho mặt hàng `A`
1. Tìm các biên lai chứa mục **A**. Ta thấy A xuất hiện trong: **T1, T2, T3**.
2. Lấy $TU$ của các biên lai này cộng lại:
   $$TWU(\{A\}) = TU(T1) + TU(T2) + TU(T3)$$
   $$TWU(\{A\}) = 8 + 22 + 21 = \mathbf{51}$$
*Giải thích ý nghĩa logic: Bất cứ khi nào A được mua, tổng số tiền cao nhất mà cửa hàng thu được từ các hóa đơn đó là 51$.*

#### 👉 Bài tập 2: Tính $TWU$ cho tập hợp 2 món `{A, D}`
1. Tìm các biên lai mà khách mua **CÙNG LÚC** cả **A** và **D**. Ta thấy chỉ có: **T1** và **T3**.
2. Lấy $TU$ của chúng cộng lại:
   $$TWU(\{A, D\}) = TU(T1) + TU(T3)$$
   $$TWU(\{A, D\}) = 8 + 21 = \mathbf{29}$$

---

### 4. Vì sao TWU lại cực kỳ thần thánh đối với thuật toán?

Bài toán HUI thông thường vướng một lỗi toán học trí mạng: **Lợi tức thực tế $EU(X)$ không hề giảm dần khi tập $X$ to lên**. 
*Ví dụ:* Một người mua 1 món kim cương (lãi 10k$) và 1 bó rau (lãi 1$). Lợi nhuận của tập `{Rau}` là 1$, tập `{Kim Cương}` là 10k$, nhưng tập `{Rau, Kim Cương}` lại lên 10,001$. Bạn không thể dựa vào một món rẽ mạt `{Rau}` để kết luận loại bỏ luôn các nhánh superset chứa nó. 

Nhưng **TWU** lại giải quyết được! Đặc tính có tên là **TWDC (Transaction-Weighted Downward Closure):**
1. Thực tế: $EU(X) \leq TWU(X)$. Lợi nhuận thực tế của tập X **luôn nhỏ hơn hoặc bằng** TWU của nó. (Vì TWU là lấy của cả biên lai chập lại).
2. Khi tập $X$ to lên thành tập mở rộng $Y$ ($X \subset Y$). Thì số lượng biên lai chứa cả tập $Y$ **chắc chắn ít hơn hoặc bằng** số biên lai chứa $X$. Dẫn tới $TWU(Y) \leq TWU(X)$.

**Tóm lại:** Lợi ích thực tế của mọi tổ hợp sinh ra từ nhánh $X$ ($EU(Y)$) đều bị chặn trần bởi $TWU(X)$: 
$$EU(Y) \leq TWU(Y) \leq TWU(X)$$

**Ứng dụng vào code:** Nếu bạn tính ra $TWU(\{Rau\}) = 50\$$, mà thuật toán đang đòi ngưỡng chặn `min_util_Border = 100$`. Ngay lập tức thuật toán sẽ **hủy bỏ** việc đẻ ra các ứng cử viên như `{Rau, Kim cương}, {Rau, Tivi}...` vì theo công thức trên, kịch trần (TWU) lợi nhuận của tất cả chúng cũng không bao giờ bò nổi qua mốc 50$. Nhờ đây hệ thống tiết kiệm được vô số RAM và CPU!
