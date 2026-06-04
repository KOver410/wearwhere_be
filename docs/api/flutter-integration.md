# WearWhere API — Flutter Integration Guide

Tài liệu hướng dẫn nối Flutter app vào WearWhere backend (Go + gin). Mọi endpoint trả JSON, JWT-based auth.

- Server: `cmd/api` (Go)
- Base URL dev: `http://10.0.2.2:8080/api/v1` (Android emulator) hoặc `http://localhost:8080/api/v1` (iOS sim / web)
- Module path Go: `github.com/wearwhere/wearwhere_be`
- Auth: `Authorization: Bearer <access_token>`
- Content-Type: `application/json` (trừ upload ảnh dùng `multipart/form-data`)

---

## 1. Quick start

### Dependencies (`pubspec.yaml`)

```yaml
dependencies:
  dio: ^5.4.0                  # HTTP client với interceptor
  flutter_secure_storage: ^9.0.0   # Lưu refresh_token an toàn
  webview_flutter: ^4.4.0      # Mở PayOS checkout
  url_launcher: ^6.2.0         # Fallback open externally
```

### Cấu hình base URL theo platform

```dart
// lib/core/api_config.dart
import 'dart:io';

class ApiConfig {
  static String get baseUrl {
    if (const bool.fromEnvironment('dart.vm.product')) {
      return 'https://api.wearwhere.vn/api/v1';  // production
    }
    // Dev: Android emulator dùng 10.0.2.2 để route ra host
    if (Platform.isAndroid) return 'http://10.0.2.2:8080/api/v1';
    return 'http://localhost:8080/api/v1';
  }

  static const accessTokenExpirySafety = Duration(seconds: 30);
}
```

### Cấu trúc thư mục đề xuất

```
lib/
  core/
    api_config.dart
    api_client.dart           # Dio + interceptor refresh
    token_storage.dart        # secure storage wrapper
    api_exception.dart
  features/
    auth/
      data/auth_repository.dart
      data/auth_dto.dart      # parse AuthResponse
    cart/
      data/cart_repository.dart
    orders/
      data/order_repository.dart
      ui/checkout_webview.dart
    ...
```

---

## 2. Auth flow

### Hai loại token

| Token | Lưu ở đâu | Dùng để | TTL mặc định |
|-------|-----------|---------|--------------|
| `access_token` | RAM (provider/state) hoặc secure storage | Mọi request authenticated | 15 phút (`JWT_ACCESS_TTL`) |
| `refresh_token` | **Bắt buộc** secure storage | Đổi lấy access mới qua `/auth/refresh` | 30 ngày (`JWT_REFRESH_TTL`) |

### Token storage

```dart
// lib/core/token_storage.dart
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class TokenStorage {
  static const _kAccess  = 'access_token';
  static const _kRefresh = 'refresh_token';
  static const _kExpiry  = 'access_expires_at';   // ISO8601

  final _storage = const FlutterSecureStorage();

  Future<void> save({required String access, required String refresh, required DateTime expiresAt}) async {
    await _storage.write(key: _kAccess,  value: access);
    await _storage.write(key: _kRefresh, value: refresh);
    await _storage.write(key: _kExpiry,  value: expiresAt.toIso8601String());
  }

  Future<String?> readAccess()   => _storage.read(key: _kAccess);
  Future<String?> readRefresh()  => _storage.read(key: _kRefresh);
  Future<DateTime?> readExpiry() async {
    final s = await _storage.read(key: _kExpiry);
    return s == null ? null : DateTime.tryParse(s);
  }
  Future<void> clear() => _storage.deleteAll();
}
```

### Dio client + auto refresh interceptor

```dart
// lib/core/api_client.dart
import 'package:dio/dio.dart';
import 'api_config.dart';
import 'token_storage.dart';

class ApiClient {
  late final Dio dio;
  final TokenStorage tokens;

  ApiClient(this.tokens) {
    dio = Dio(BaseOptions(
      baseUrl: ApiConfig.baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
      responseType: ResponseType.json,
    ));

    dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) async {
        if (options.extra['skip_auth'] != true) {
          final access = await tokens.readAccess();
          if (access != null) options.headers['Authorization'] = 'Bearer $access';
        }
        return handler.next(options);
      },
      onError: (err, handler) async {
        // Tự refresh khi gặp 401, retry đúng 1 lần
        if (err.response?.statusCode == 401 && err.requestOptions.extra['retried'] != true) {
          final refreshed = await _tryRefresh();
          if (refreshed) {
            final req = err.requestOptions;
            req.extra['retried'] = true;
            req.headers['Authorization'] = 'Bearer ${await tokens.readAccess()}';
            try {
              final clone = await dio.fetch(req);
              return handler.resolve(clone);
            } catch (e) {
              return handler.next(err);
            }
          }
        }
        return handler.next(err);
      },
    ));
  }

  Future<bool> _tryRefresh() async {
    final rt = await tokens.readRefresh();
    if (rt == null) return false;
    try {
      final resp = await dio.post(
        '/auth/refresh',
        data: {'refresh_token': rt},
        options: Options(extra: {'skip_auth': true}),
      );
      final t = resp.data['tokens'] as Map<String, dynamic>;
      await tokens.save(
        access:  t['access_token']  as String,
        refresh: t['refresh_token'] as String,
        expiresAt: DateTime.parse(t['expires_at'] as String),
      );
      return true;
    } catch (_) {
      await tokens.clear();
      return false;
    }
  }
}
```

### Đăng ký + đăng nhập

```dart
// lib/features/auth/data/auth_repository.dart
class AuthRepository {
  final Dio _dio;
  final TokenStorage _tokens;
  AuthRepository(this._dio, this._tokens);

  Future<UserProfile> register({
    String? email, String? phone,
    required String password, required String name,
  }) async {
    final resp = await _dio.post('/auth/register',
      data: {
        if (email != null) 'email': email,
        if (phone != null) 'phone': phone,
        'password': password,
        'name': name,
      },
      options: Options(extra: {'skip_auth': true}),
    );
    return _persistAuth(resp.data);
  }

  Future<UserProfile> login({String? email, String? phone, required String password}) async {
    final resp = await _dio.post('/auth/login',
      data: {
        if (email != null) 'email': email,
        if (phone != null) 'phone': phone,
        'password': password,
      },
      options: Options(extra: {'skip_auth': true}),
    );
    return _persistAuth(resp.data);
  }

  Future<UserProfile> loginWithGoogle(String googleIdToken) async {
    final resp = await _dio.post('/auth/oauth/google',
      data: {'id_token': googleIdToken},
      options: Options(extra: {'skip_auth': true}),
    );
    return _persistAuth(resp.data);
  }

  Future<UserProfile> loginWithApple(String appleIdentityToken) async {
    final resp = await _dio.post('/auth/oauth/apple',
      data: {'id_token': appleIdentityToken},
      options: Options(extra: {'skip_auth': true}),
    );
    return _persistAuth(resp.data);
  }

  Future<void> logout() async {
    final rt = await _tokens.readRefresh();
    if (rt != null) {
      try { await _dio.post('/auth/logout', data: {'refresh_token': rt}); } catch (_) {}
    }
    await _tokens.clear();
  }

  Future<UserProfile> _persistAuth(Map<String, dynamic> data) async {
    final t = data['tokens'] as Map<String, dynamic>;
    await _tokens.save(
      access:  t['access_token']  as String,
      refresh: t['refresh_token'] as String,
      expiresAt: DateTime.parse(t['expires_at'] as String),
    );
    return UserProfile.fromJson(data['user'] as Map<String, dynamic>);
  }
}
```

---

## 3. Định dạng response & error

### Success

JSON object hoặc array trực tiếp — không có envelope `{data: ...}` (trừ list paginated).

### Error: 2 format (do lịch sử Sprint 1 vs Sprint 2/3)

**Format A — nested** (Sprint 1: auth/profile/brand/catalog/product):
```json
{ "error": { "code": "VALIDATION_FAILED", "message": "Email is required", "details": { "field": "email" } } }
```

**Format B — flat** (Sprint 2/3: cart, wishlist, addresses, orders, payments):
```json
{ "error": "cart_empty" }
{ "error": "insufficient_stock", "variant_id": "uuid", "requested": 3, "available": 1 }
{ "error": "cancel_not_allowed", "subcode": "paid_not_supported" }
```

### Helper parse cả 2 dạng

```dart
// lib/core/api_exception.dart
class ApiException implements Exception {
  final int statusCode;
  final String code;       // luôn có; format A: error.code, format B: error
  final String? message;
  final Map<String, dynamic>? details;

  ApiException(this.statusCode, this.code, {this.message, this.details});

  factory ApiException.fromDio(DioException e) {
    final status = e.response?.statusCode ?? 0;
    final body = e.response?.data;
    if (body is Map) {
      final err = body['error'];
      if (err is Map) {
        return ApiException(status,
          err['code'] as String? ?? 'UNKNOWN',
          message: err['message'] as String?,
          details: (err['details'] as Map?)?.cast<String, dynamic>());
      }
      if (err is String) {
        return ApiException(status, err,
          message: body['message'] as String?,
          details: {...body}..remove('error'));
      }
    }
    return ApiException(status, 'NETWORK_ERROR', message: e.message);
  }

  @override String toString() => 'ApiException($statusCode/$code): ${message ?? ''}';
}
```

### HTTP status codes

| Code | Khi nào | Cách xử lý FE |
|------|---------|---------------|
| 200 / 201 | Thành công | Parse body |
| 204 | Xóa thành công | Không có body |
| 400 | Body sai / thiếu | Hiển thị `error.message` |
| 401 | Hết hạn access token / không auth | Interceptor tự refresh; nếu refresh fail → logout |
| 403 | Sai role (vd customer hit /brand/me) | Báo "không có quyền" |
| 404 | Resource không tồn tại / không owned | "Không tìm thấy" |
| 409 | Conflict (stock, slug, cancel rule…) | Hiển thị message + xử lý theo `subcode` |
| 422 | Validation semantic (variant unavailable…) | Hiển thị message |
| 429 | Rate-limited | Backoff + retry-after |
| 502 | PayOS gateway lỗi | "Thanh toán tạm thời lỗi, thử lại" |
| 5xx | Server lỗi | Retry với exponential backoff |

---

## 4. Endpoint reference

### 4.1 Auth — `/api/v1/auth/*` (public)

| Method | Path | Body | Response | Auth |
|--------|------|------|----------|------|
| POST | `/auth/register` | `{email?, phone?, password, name}` | `AuthResponse` | — |
| POST | `/auth/login` | `{email?, phone?, password}` | `AuthResponse` | — |
| POST | `/auth/refresh` | `{refresh_token}` | `AuthResponse` | — |
| POST | `/auth/brand/login` | giống login | `AuthResponse` (role=brand) | — |
| POST | `/auth/admin/login` | giống login | `AuthResponse` (role=admin) | — |
| POST | `/auth/password/forgot` | `{email?, phone?}` | `204` (gửi OTP) | — |
| POST | `/auth/password/reset` | `{email?, phone?, otp, new_password}` | `204` | — |
| POST | `/auth/otp/send` | `{email?, phone?, purpose}` | `204` | — |
| POST | `/auth/otp/verify` | `{email?, phone?, otp, purpose}` | `204` hoặc `AuthResponse` (nếu purpose=verify_email/phone tự login) | — |
| POST | `/auth/oauth/google` | `{id_token}` | `AuthResponse` | — |
| POST | `/auth/oauth/apple` | `{id_token}` | `AuthResponse` | — |
| POST | `/auth/logout` | `{refresh_token}` | `204` | Bearer |

**`purpose` cho OTP:** `verify_email` | `verify_phone` | `reset_password`. Phải kèm đúng `email` hoặc `phone` tương ứng.

**`AuthResponse`:**
```json
{
  "user": {
    "id": "uuid", "email": "a@b.com", "phone": null, "name": "An",
    "role": "customer", "status": "active",
    "avatar_url": null, "bio": null,
    "email_verified": true, "phone_verified": false,
    "created_at": "2026-05-27T10:00:00Z"
  },
  "tokens": {
    "access_token":  "eyJhbGc...",
    "refresh_token": "rt_...",
    "token_type":    "Bearer",
    "expires_at":    "2026-05-27T10:15:00Z"
  }
}
```

**Password rule:** `strong_password` validator = min 8 ký tự, có chữ HOA + thường + số. Sai → 400 `VALIDATION_FAILED`.

**Phone format:** E.164 (`+84901234567`). KHÔNG dùng `0901234567`.

### 4.2 Profile — `/api/v1/me`

| Method | Path | Body | Notes |
|--------|------|------|-------|
| GET | `/me` | — | Trả `UserResponse` |
| PATCH | `/me` | `{name?, avatar_url?, bio?}` | Partial update |
| DELETE | `/me` | `{password}` | Soft-delete account |
| POST | `/me/password` | `{current_password, new_password}` | Đổi mật khẩu |

### 4.3 Catalog — `/api/v1/*` (public, không cần auth)

| Method | Path | Query | Notes |
|--------|------|-------|-------|
| GET | `/products` | `q`, `brand`, `category`, `style_tags` (comma), `price_min`, `price_max`, `sort` (`newest`/`price_asc`/`price_desc`/`popular`), `page`, `page_size` (≤50) | Paginated, full-text search qua `q` |
| GET | `/products/by-id/:id` | — | Detail theo UUID |
| GET | `/brands/:brand_slug/products/:product_slug` | — | Detail theo slug (canonical URL); auto-tăng `view_count` |
| GET | `/categories` | — | Cây danh mục flat |
| GET | `/style-tags` | — | List style tag |
| GET | `/brands` | `q`, `page`, `page_size` | List brand verified |
| GET | `/brands/:brand_slug` | — | Brand detail + addresses |

**Paginated envelope:**
```json
{
  "data": [ /* ... */ ],
  "page": 1, "page_size": 20, "total": 137, "total_pages": 7
}
```

### 4.4 Customer addresses — `/api/v1/me/addresses` (auth, role=customer)

| Method | Path | Body | Notes |
|--------|------|------|-------|
| GET | `/me/addresses` | — | List addresses |
| POST | `/me/addresses` | `{recipient_name, recipient_phone, address_line, ward, district, city, is_default?}` | `is_default=true` tự bỏ flag của địa chỉ cũ |
| GET | `/me/addresses/:id` | — | Detail (IDOR-safe) |
| PATCH | `/me/addresses/:id` | partial | |
| DELETE | `/me/addresses/:id` | — | Soft-delete; nếu xóa default thì promote địa chỉ cũ nhất |

### 4.5 Wishlist — `/api/v1/me/wishlist` (auth)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/me/wishlist` | List wishlist items (denormalized: product info) |
| GET | `/me/wishlist/contains?product_ids=uuid1,uuid2` | Batch check: trả `{ "uuid1": true, "uuid2": false }` |
| POST | `/me/wishlist/:product_id` | Idempotent add (re-add OK) |
| DELETE | `/me/wishlist/:product_id` | Idempotent remove |

### 4.6 Cart — `/api/v1/me/cart` (auth)

| Method | Path | Body | Notes |
|--------|------|------|-------|
| GET | `/me/cart` | — | List + summary (subtotal, current_price khác price_snapshot → đánh dấu) |
| POST | `/me/cart/items` | `{variant_id, qty}` | UPSERT; qty cộng dồn clamped ≤ 10 và ≤ stock |
| PATCH | `/me/cart/items/:item_id` | `{qty}` | qty ∈ [1,10] |
| DELETE | `/me/cart/items/:item_id` | — | Xóa 1 item |
| DELETE | `/me/cart` | — | Xóa toàn bộ |

**Cart errors (flat format):**
- `qty_exceeds_max` (>10)
- `out_of_stock`
- `variant_unavailable` (đã soft-delete hoặc inactive)
- `cart_item_not_found`

**`GET /me/cart` response:**
```json
{
  "items": [{
    "id": "uuid", "qty": 2,
    "price_snapshot": 350000, "current_price": 350000, "currency": "VND",
    "added_at": "...",
    "variant": { "id":"uuid", "sku":"BLK-L", "size":"L", "color":"Black", "color_hex":"#000", "stock_qty":12 },
    "product": { "id":"uuid", "slug":"oversized-tee", "name":"Oversized Tee", "primary_image_url":"https://..." },
    "brand":   { "id":"uuid", "slug":"rep-vn", "name":"REP VN" },
    "unavailable": false, "unavailable_reason": null
  }],
  "summary": { "item_count": 2, "subtotal": 700000, "currency": "VND" }
}
```

### 4.7 Checkout & Orders — `/api/v1/me/*` (auth)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/me/checkout/preview?address_id=uuid` | Dry-run, KHÔNG tạo order, KHÔNG hold stock |
| POST | `/me/orders` | Body `{address_id, payment_method: "cod"|"payos", notes?}` |
| GET | `/me/orders?status=&page=&page_size=&from=&to=` | List có filter |
| GET | `/me/orders/:order_no` | Detail (vd `WW-20260527-AB12CD`) |
| POST | `/me/orders/:order_no/cancel` | Body `{reason?}` |

**Min order value:** 50,000 VND (trên subtotal). Sai → 400 `min_order_value` + `min_value_vnd`.

**`POST /me/orders` response 201:**
```json
{
  "order": {
    "id": "uuid", "order_no": "WW-20260527-AB12CD",
    "status": "pending_payment",          // hoặc "processing" cho COD
    "payment_method": "payos", "payment_status": "pending",
    "subtotal_vnd": 700000, "shipping_total_vnd": 30000, "grand_total_vnd": 730000,
    "shipping_address": {...},
    "sub_orders": [
      { "id":"uuid", "brand":{...}, "subtotal_vnd":700000, "shipping_fee_vnd":30000,
        "total_vnd":730000, "status":"pending", "items":[...] }
    ],
    "created_at":"..."
  },
  "payment": {
    "id": "uuid", "method":"payos", "status":"pending",
    "amount_vnd": 730000,
    "checkout_url": "https://pay.payos.vn/web/abc...",   // null cho COD
    "qr_code": "data:image/png;base64,...",              // null cho COD
    "expired_at": "..."                                  // null cho COD
  }
}
```

**Cancel error subcodes** (409):
```json
{ "error": "cancel_not_allowed", "subcode": "paid_not_supported" }   // Sprint 3: cancel đơn paid → Sprint 4
{ "error": "cancel_not_allowed", "subcode": "already_shipped" }      // sub_order != pending
{ "error": "cancel_not_allowed", "subcode": "already_cancelled" }
{ "error": "cancel_not_allowed", "subcode": "already_completed" }
```

### 4.8 Brand portal — `/api/v1/brand/me/*` (auth + role=brand)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/brand/me` | Thông tin brand đang đăng nhập |
| PATCH | `/brand/me` | Update profile |
| GET/POST/PATCH/DELETE | `/brand/me/addresses[/:id]` | Brand store addresses |
| GET/POST | `/brand/me/products` | List + create |
| GET/PATCH/DELETE | `/brand/me/products/:id` | Detail + update + soft-delete |
| POST/PATCH/DELETE | `/brand/me/products/:id/variants[/:variant_id]` | Variant CRUD |
| POST | `/brand/me/products/:id/images` | **multipart/form-data**, field `images` (≤6 files, mỗi file ≤5MB JPG/PNG/WebP) |
| PATCH/DELETE | `/brand/me/products/:id/images/:image_id` | Update alt_text / set primary / soft-delete |

---

## 5. Payment flow với PayOS (WebView)

```
Customer chọn payment_method=payos
  ↓
POST /me/orders  → response.payment.checkout_url
  ↓
FE mở WebView với URL đó
  ↓
User thanh toán trên PayOS (banking / QR / e-wallet)
  ↓
PayOS gọi webhook → BE update payment + order (paid)
  ↓
PayOS redirect user về PAYOS_RETURN_URL  (kèm ?orderNo=...)
  ↓
WebView detect URL match → close + navigate Order Detail screen
  ↓
FE poll GET /me/orders/:order_no → status=processing, payment_status=paid
```

### Flutter implementation

```dart
// lib/features/orders/ui/checkout_webview.dart
import 'package:flutter/material.dart';
import 'package:webview_flutter/webview_flutter.dart';

class CheckoutWebView extends StatefulWidget {
  final String checkoutUrl;
  final String orderNo;
  final String returnUrlPrefix;   // ví dụ "https://app.wearwhere.vn/checkout/success"
  final String cancelUrlPrefix;   // "https://app.wearwhere.vn/checkout/cancel"
  const CheckoutWebView({
    super.key, required this.checkoutUrl, required this.orderNo,
    required this.returnUrlPrefix, required this.cancelUrlPrefix,
  });

  @override
  State<CheckoutWebView> createState() => _CheckoutWebViewState();
}

class _CheckoutWebViewState extends State<CheckoutWebView> {
  late final WebViewController controller;

  @override
  void initState() {
    super.initState();
    controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(NavigationDelegate(
        onNavigationRequest: (req) {
          if (req.url.startsWith(widget.returnUrlPrefix)) {
            Navigator.pop(context, _CheckoutResult.success);
            return NavigationDecision.prevent;
          }
          if (req.url.startsWith(widget.cancelUrlPrefix)) {
            Navigator.pop(context, _CheckoutResult.cancelled);
            return NavigationDecision.prevent;
          }
          return NavigationDecision.navigate;
        },
      ))
      ..loadRequest(Uri.parse(widget.checkoutUrl));
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text('Thanh toán đơn ${widget.orderNo}')),
    body: WebViewWidget(controller: controller),
  );
}

enum _CheckoutResult { success, cancelled }
```

### Gọi từ màn order summary

```dart
final placed = await orderRepo.placeOrder(addressId: addrId, method: 'payos');
if (placed.payment.checkoutUrl != null) {
  final result = await Navigator.push<_CheckoutResult>(
    context,
    MaterialPageRoute(builder: (_) => CheckoutWebView(
      checkoutUrl: placed.payment.checkoutUrl!,
      orderNo: placed.order.orderNo,
      returnUrlPrefix: 'http://localhost:3000/checkout/success',
      cancelUrlPrefix: 'http://localhost:3000/checkout/cancel',
    )),
  );
  // result==success → poll detail; result==cancelled → trở về cart
  // LƯU Ý: webhook có thể đến TRƯỚC hoặc SAU khi user redirect.
  // Nên poll GET /me/orders/:order_no mỗi 2s × 5 lần để đảm bảo paid_at xuất hiện.
}
```

**Lưu ý quan trọng:**
- Đừng tin redirect URL để đánh dấu thanh toán thành công. Chỉ tin `payment_status=paid` ở backend (do webhook).
- Có thể user đóng app sau khi thanh toán → app mở lại phải fetch lại `/me/orders/:order_no` để biết trạng thái thật.
- Trên iOS/Android nên dùng `flutter_inappwebview` hoặc `webview_flutter` 4.x (đã hỗ trợ cookie sharing).

### Dev environment với PAYOS_MODE=mock

Backend serve `/dev/payos/mock-checkout?orderCode=N` (HTML đơn giản với 2 nút Success/Fail). WebView hiển thị → bấm Success → POST tới `/dev/payos/simulate` mô phỏng webhook → đơn chuyển sang paid.

---

## 6. Upload ảnh product (multipart)

```dart
Future<List<ProductImage>> uploadImages({
  required String productId, required List<XFile> files,
}) async {
  final form = FormData();
  for (final f in files) {
    form.files.add(MapEntry('images', await MultipartFile.fromFile(
      f.path, filename: f.name,
      // contentType: optional, dio đoán từ extension
    )));
  }
  final resp = await dio.post(
    '/brand/me/products/$productId/images',
    data: form,
    options: Options(contentType: 'multipart/form-data'),
  );
  return (resp.data['data'] as List)
    .map((j) => ProductImage.fromJson(j)).toList();
}
```

**Giới hạn server:**
- `STORAGE_MAX_FILE_SIZE` mặc định 5MB (`5*1024*1024` bytes).
- `STORAGE_ALLOWED_MIMES` mặc định `image/jpeg,image/png,image/webp`.
- Max 6 ảnh/lần upload.
- File URL trả về có dạng `http://localhost:8080/uploads/...` (dev local) hoặc `https://storage.googleapis.com/bucket/...` (GCS prod).

---

## 7. Dart model snippets

### UserProfile

```dart
class UserProfile {
  final String id, name, role, status, createdAt;
  final String? email, phone, avatarUrl, bio;
  final bool emailVerified, phoneVerified;
  UserProfile({/*...*/});

  factory UserProfile.fromJson(Map<String, dynamic> j) => UserProfile(
    id: j['id'] as String,
    email: j['email'] as String?,
    phone: j['phone'] as String?,
    name: j['name'] as String,
    role: j['role'] as String,
    status: j['status'] as String,
    avatarUrl: j['avatar_url'] as String?,
    bio: j['bio'] as String?,
    emailVerified: j['email_verified'] as bool,
    phoneVerified: j['phone_verified'] as bool,
    createdAt: j['created_at'] as String,
  );
}
```

### OrderResp / SubOrder / OrderItem

```dart
enum OrderStatus { pendingPayment, processing, cancelled, completed }
enum PaymentMethod { cod, payos }
enum PaymentStatus { pending, paid, failed, cancelled }
enum SubOrderStatus { pending, confirmed, preparing, shipped, delivered, cancelled }

T _enumFromString<T>(List<T> values, String s) =>
  values.firstWhere((v) => v.toString().split('.').last.toLowerCase() == s.replaceAll('_', '').toLowerCase());

class OrderResp {
  final String id, orderNo;
  final OrderStatus status;
  final PaymentMethod paymentMethod;
  final PaymentStatus paymentStatus;
  final int subtotalVnd, shippingTotalVnd, grandTotalVnd;
  final ShippingAddress shippingAddress;
  final String notes, cancelReason;
  final List<SubOrderResp> subOrders;
  final DateTime createdAt;
  final DateTime? paidAt, cancelledAt;
  // ...

  factory OrderResp.fromJson(Map<String, dynamic> j) => OrderResp(
    id: j['id'], orderNo: j['order_no'],
    status: _parseOrderStatus(j['status']),
    paymentMethod: j['payment_method'] == 'cod' ? PaymentMethod.cod : PaymentMethod.payos,
    paymentStatus: _parsePaymentStatus(j['payment_status']),
    subtotalVnd:      (j['subtotal_vnd']      as num).toInt(),
    shippingTotalVnd: (j['shipping_total_vnd']as num).toInt(),
    grandTotalVnd:    (j['grand_total_vnd']   as num).toInt(),
    shippingAddress: ShippingAddress.fromJson(j['shipping_address']),
    notes: j['notes'] ?? '', cancelReason: j['cancel_reason'] ?? '',
    subOrders: (j['sub_orders'] as List? ?? [])
      .map((s) => SubOrderResp.fromJson(s as Map<String, dynamic>)).toList(),
    createdAt: DateTime.parse(j['created_at']),
    paidAt: j['paid_at'] == null ? null : DateTime.parse(j['paid_at']),
    cancelledAt: j['cancelled_at'] == null ? null : DateTime.parse(j['cancelled_at']),
  );
}
```

`int64` VND: backend trả `number` JSON. Dart's `int` là 64-bit trên VM, 53-bit trên web. Với VND giá trị ~10^9 vẫn an toàn cho web — nhưng nếu cần >2^53, dùng `BigInt`.

---

## 8. Bảng env app

| Env | Backend var | Ý nghĩa cho FE |
|-----|-------------|---------------|
| Dev mock | `PAYOS_MODE=mock` | Checkout URL trỏ về `localhost:8080/dev/payos/mock-checkout` |
| Dev real | `PAYOS_MODE=production` (cần creds) | Checkout URL là `https://pay.payos.vn/web/...` thật |
| Prod | giống Dev real + domain ổn định | |

**Để Flutter test với PayOS thật từ máy dev:**
1. Bật server với `PAYOS_MODE=production` (đã có creds trong `.env`)
2. Dùng ngrok / cloudflared expose `localhost:8080` ra public URL
3. Trong PayOS dashboard cấu hình webhook URL = `https://<tunnel>/api/v1/payments/payos/webhook`
4. Flutter trỏ `baseUrl` tới tunnel hoặc giữ `10.0.2.2` (vì webhook là server→server, không liên quan FE)

---

## 9. Healthcheck

```dart
final ok = (await dio.get('/healthz')).statusCode == 200;
```

Trả `{"status":"ok"}` — không cần auth.

---

## 10. Checklist tích hợp

- [ ] Cài `dio`, `flutter_secure_storage`, `webview_flutter`.
- [ ] Implement `TokenStorage` + `ApiClient` với refresh interceptor.
- [ ] Register/login screens → `AuthRepository`.
- [ ] Catalog screens dùng public endpoints (không gửi Authorization).
- [ ] Cart/wishlist/address screens — auth required.
- [ ] Checkout flow: preview → place_order → (nếu PayOS) WebView → poll detail.
- [ ] Handle 401 silently (interceptor đã làm); 403 → "không có quyền"; 409 dựa vào subcode.
- [ ] Hiển thị warnings từ `/checkout/preview.warnings` (low stock / unavailable).
- [ ] Test full flow trên Android emulator (10.0.2.2) + iOS sim (localhost).
- [ ] Khi đẩy build prod, đảm bảo `baseUrl` đổi sang HTTPS và `PAYOS_MODE=production` ở server.

---

**Liên hệ backend:** đọc spec `docs/superpowers/specs/2026-05-24-sprint-3-orders-checkout-payos-design.md` để hiểu lifecycle order/payment. Nếu gặp lỗi không có trong tài liệu này, kiểm tra `internal/<module>/handler/handler.go` để xem code-string chính xác.
