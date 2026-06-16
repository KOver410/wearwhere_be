# WearWhere API — Flutter Integration Guide

Tài liệu hướng dẫn nối Flutter app vào WearWhere backend (Go + gin). Mọi endpoint trả JSON, JWT-based auth.

- Server: `cmd/api` (Go)
- Base URL dev: `http://10.0.2.2:8080/api/v1` (Android emulator) hoặc `http://localhost:8080/api/v1` (iOS sim / web)
- Base URL demo (VM trên GCP): `https://34-87-41-62.sslip.io/api/v1` — HTTPS hợp lệ (Let's Encrypt), dùng test trên thiết bị thật. **Lưu ý:** hostname gắn với IP của VM; nếu VM được tạo lại (`terraform destroy`/`apply`) thì IP đổi → đổi lại URL. Production thật nên dùng domain riêng.
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
  // Override khi chạy: flutter run --dart-define=API_BASE_URL=https://34-87-41-62.sslip.io/api/v1
  // Cho phép trỏ vào VM demo mà không phải sửa hằng số production.
  static const _override = String.fromEnvironment('API_BASE_URL');

  static String get baseUrl {
    if (_override.isNotEmpty) return _override;
    if (const bool.fromEnvironment('dart.vm.product')) {
      return 'https://api.wearwhere.vn/api/v1';  // production (domain thật khi có)
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

**Format A — nested** (Sprint 1: auth/profile/brand/catalog/product; **+ locations & brand fulfillment (Goship)**):
```json
{ "error": { "code": "VALIDATION_FAILED", "message": "Email is required", "details": { "field": "email" } } }
```
Mã lỗi mới (Format A): `SUB_ORDER_NOT_FOUND`, `NOT_OWNER`, `INVALID_TRANSITION`, `CARRIER_UNAVAILABLE`, `ADDRESS_INCOMPLETE`, `SHIPMENT_FAILED`, `GOSHIP_UNAVAILABLE` (location).

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
| POST | `/me/addresses` | xem dưới | Bắt buộc `label` + **mã địa điểm** (`city_code/district_code/ward_code`) |
| GET | `/me/addresses/:id` | — | Detail (IDOR-safe) |
| PATCH | `/me/addresses/:id` | partial | Mã địa điểm optional; nhưng nếu gửi 1 mã thì phải gửi đủ cả 3 |
| DELETE | `/me/addresses/:id` | — | Soft-delete; nếu xóa default thì promote địa chỉ cũ nhất |

> ⚠️ **Thay đổi (Goship):** địa chỉ giờ yêu cầu **mã địa điểm** để tính phí ship + tạo đơn vận. FE phải lấy mã từ endpoint **Locations** (§4.4.1) bằng dropdown phân cấp tỉnh→huyện→xã, rồi gửi kèm khi tạo/sửa địa chỉ.

**`POST /me/addresses` body:**
```json
{
  "label": "Nhà",
  "recipient_name": "Nguyễn Văn A",
  "recipient_phone": "+84901234567",
  "address_line": "123 Lê Lợi",
  "ward": "Phường Bến Nghé",
  "district": "Quận 1",
  "city": "Hồ Chí Minh",
  "city_code": "700000",
  "district_code": "700100",
  "ward_code": "70010001",
  "country": "VN",          // optional, default VN
  "postal_code": null,       // optional
  "note": null,              // optional
  "is_default": true         // optional; true tự bỏ flag default của địa chỉ cũ
}
```
- `recipient_phone`: E.164 (`+84...`).
- `city_code/district_code/ward_code`: lấy từ §4.4.1, kiểu **string**. Validate phân cấp ở server: district phải thuộc city, ward phải thuộc district → sai trả `ErrInvalidLocation` (Format A).
- **Lưu ý:** `AddressResponse` hiện **không** echo lại 3 mã. FE muốn pre-fill khi sửa nên giữ mã ở client hoặc map lại từ tên qua Locations.

**`PATCH /me/addresses/:id`:** mọi field optional (partial). Với mã địa điểm: gửi cả 3 (đổi địa điểm) hoặc không gửi cái nào (giữ nguyên); gửi thiếu (1–2 mã) → lỗi.

#### 4.4.1 Locations — `/api/v1/locations/*` (public, không cần auth) — **MỚI (Goship)**

Dữ liệu tỉnh/huyện/xã từ Goship (cache 24h). Dùng để đổ dropdown phân cấp và lấy mã gửi kèm khi tạo địa chỉ.

| Method | Path | Notes |
|--------|------|-------|
| GET | `/locations/cities` | Danh sách tỉnh/thành |
| GET | `/locations/cities/:city_code/districts` | Quận/huyện thuộc 1 tỉnh |
| GET | `/locations/districts/:district_code/wards` | Phường/xã thuộc 1 quận |

Response (cả 3 endpoint cùng dạng):
```json
{ "data": [ { "code": "700000", "name": "Hồ Chí Minh" }, { "code": "100000", "name": "Hà Nội" } ] }
```
Flow FE: chọn city → gọi `/cities/:code/districts` → chọn district → gọi `/districts/:code/wards` → chọn ward → gửi 3 `code` + 3 tên vào `POST /me/addresses`.

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
| GET | `/me/checkout/preview?address_id=uuid` | Dry-run; trả **carrier options** mỗi brand + `address_incomplete`. KHÔNG tạo order, KHÔNG hold stock |
| POST | `/me/orders` | Body có thêm **`shipping_selections`** (chọn carrier mỗi brand) |
| GET | `/me/orders?status=&page=&page_size=&from=&to=` | List có filter |
| GET | `/me/orders/:order_no` | Detail (vd `WW-20260527-AB12CD`) — có tracking |
| POST | `/me/orders/:order_no/cancel` | Body `{reason?}` |

> ⚠️ **Thay đổi (Goship):** luồng checkout giờ là **preview → khách chọn nhà vận chuyển cho từng brand → place**. Preview trả nhiều carrier (GHN/GHTK/VTP…) kèm phí + ETA; FE cho khách chọn rồi gửi `shipping_selections` khi đặt đơn. Server **re-quote** lúc đặt để chốt phí chính thức.

**Min order value:** 50,000 VND (trên subtotal). Sai → 400 `min_order_value` + `min_value_vnd`.

**`GET /me/checkout/preview?address_id=uuid` response:**
```json
{
  "cart_empty": false,
  "address": { "recipient":"...", "phone":"...", "line1":"...", "ward":"...", "district":"...", "city":"...",
               "city_code":"700000", "district_code":"700100", "ward_code":"70010001" },
  "address_incomplete": false,        // true nếu địa chỉ THIẾU mã → KHÔNG có options + sẽ chặn đặt đơn
  "sub_orders": [
    {
      "brand": { "id":"uuid", "slug":"rep-vn", "name":"REP VN" },
      "items": [ { "variant_id":"uuid","product_name":"...","variant_label":"Black / L","qty":2,
                   "unit_price_vnd":350000,"line_total_vnd":700000,"available_qty":12,"image_url":"https://..." } ],
      "subtotal_vnd": 700000,
      "shipping_fee_vnd": 20000,        // phí của option RẺ NHẤT (gợi ý); phí thật chốt theo carrier khách chọn
      "total_vnd": 720000,
      "shipping_options": [
        { "carrier":"Giao Hàng Nhanh (v3)", "carrier_name":"Giao Hàng Nhanh (v3)", "service":"standard", "amount_vnd":20000, "eta":"2-4 ngày" },
        { "carrier":"Vietnam Post", "carrier_name":"Vietnam Post", "service":"standard", "amount_vnd":15000, "eta":"3-5 ngày" }
      ]
    }
  ],
  "subtotal_vnd": 700000, "shipping_total_vnd": 20000, "grand_total_vnd": 720000,
  "min_order_value_vnd": 50000, "meets_min_order": true,
  "warnings": []
}
```
- `address_incomplete=true` → địa chỉ chưa có mã (địa chỉ cũ trước Goship). FE phải yêu cầu khách mở lại địa chỉ, chọn dropdown (§4.4.1) để bổ sung mã trước khi đặt.
- `shipping_options[].carrier` là **định danh** để gửi lại trong `shipping_selections` (hiện = tên hiển thị, vì Goship không trả mã ngắn).

**`POST /me/orders` body:**
```json
{
  "address_id": "uuid",
  "payment_method": "payos",            // "cod" | "payos"
  "notes": "giao giờ hành chính",       // optional, ≤500
  "shipping_selections": [              // BẮT BUỘC — 1 phần tử / brand trong giỏ
    { "brand_id": "uuid", "carrier": "Giao Hàng Nhanh (v3)" }
  ]
}
```
- `carrier` = giá trị `shipping_options[].carrier` khách đã chọn ở preview.
- Server re-quote theo carrier → nếu carrier không còn khả dụng → **409 `carrier_unavailable`** (FE preview lại). Thiếu selection cho 1 brand → lỗi. Địa chỉ thiếu mã → **422 `address_incomplete`** (Format A).

**`POST /me/orders` response 201:**
```json
{
  "order": {
    "id": "uuid", "order_no": "WW-20260527-AB12CD",
    "status": "pending_payment",          // hoặc "processing" cho COD
    "payment_method": "payos", "payment_status": "pending",
    "subtotal_vnd": 700000, "shipping_total_vnd": 20000, "grand_total_vnd": 720000,
    "shipping_address": {...},
    "sub_orders": [
      { "id":"uuid", "brand":{...}, "subtotal_vnd":700000, "shipping_fee_vnd":20000,
        "total_vnd":720000, "status":"pending",
        "tracking_no": null, "shipping_carrier": "Giao Hàng Nhanh (v3)",
        "tracking_url": null, "shipping_status_text": null,
        "items":[...] }
    ],
    "created_at":"..."
  },
  "payment": {
    "id": "uuid", "method":"payos", "status":"pending",
    "amount_vnd": 720000,
    "checkout_url": "https://pay.payos.vn/web/abc...",   // null cho COD
    "qr_code": "data:image/png;base64,...",              // null cho COD
    "expired_at": "..."                                  // null cho COD
  }
}
```

**Tracking trong `GET /me/orders/:order_no`:** mỗi `sub_orders[]` có `tracking_no`, `shipping_carrier`, `tracking_url`, `shipping_status_text` (cập nhật khi brand ship + webhook Goship báo trạng thái — xem §5.1). Khách dùng `tracking_url` để mở trang tra cứu của hãng, `shipping_status_text` để hiển thị trạng thái live.

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

### 4.9 Brand fulfillment — `/api/v1/brand/me/orders/*` (auth + role=brand) — **MỚI (Goship)**

Brand xử lý các **sub-order** của riêng mình (mỗi đơn khách có thể tách thành nhiều sub-order theo brand). Chỉ thao tác được sub-order thuộc brand đang đăng nhập (khác brand → 403).

| Method | Path | Body | Notes |
|--------|------|------|-------|
| GET | `/brand/me/orders?status=&page=&page_size=` | — | List sub-order của brand (filter theo `status`) |
| GET | `/brand/me/orders/:sub_order_id` | — | Chi tiết sub-order (items + địa chỉ giao + tracking) |
| POST | `/brand/me/orders/:sub_order_id/confirm` | — | `pending → confirmed` |
| POST | `/brand/me/orders/:sub_order_id/ship` | `{carrier?}` | `confirmed → shipped`: tạo đơn Goship, lưu tracking. `carrier` optional (override carrier đã chốt lúc đặt) |

**Vòng đời sub-order:** `pending → confirmed → shipped → delivered`. `delivered` do **webhook Goship** (§5.1), không phải brand tự bấm. Khi tất cả sub-order của 1 đơn `delivered` → order `completed`; đơn **COD** được đánh dấu `paid` + trừ kho lúc giao.

**`GET /brand/me/orders` response:**
```json
{
  "data": [
    { "sub_order_id":"uuid", "order_no":"WW-20260527-AB12CD", "status":"pending",
      "recipient":"Nguyễn Văn A", "total_vnd":720000, "item_count":2,
      "tracking_no": null, "created_at":"..." }
  ],
  "page":1, "page_size":20, "total":1, "total_pages":1
}
```

**`GET /brand/me/orders/:sub_order_id` response:**
```json
{
  "sub_order_id":"uuid", "order_no":"WW-20260527-AB12CD", "status":"confirmed",
  "subtotal_vnd":700000, "shipping_fee_vnd":20000, "total_vnd":720000,
  "shipping_carrier":"Giao Hàng Nhanh (v3)", "tracking_no":null, "tracking_url":null, "shipping_status_text":null,
  "shipping_address": { "recipient":"...","phone":"...","line1":"...","ward":"...","district":"...","city":"..." },
  "items": [ { "id":"uuid","variant_id":"uuid","product_name":"...","variant_label":"Black / L","qty":2,"unit_price_vnd":350000,"line_total_vnd":700000,"image_url":"https://..." } ],
  "created_at":"..."
}
```

**Lỗi (Format A nested):** `SUB_ORDER_NOT_FOUND` (404), `NOT_OWNER` (403), `INVALID_TRANSITION` (409 — vd ship khi chưa confirm / đơn chưa thanh toán), `CARRIER_UNAVAILABLE` (409), `SHIPMENT_FAILED` (502). Chỉ ship được khi order ở trạng thái `processing` (COD, hoặc PayOS đã `paid`).

> Phí khách trả (`shipping_fee_vnd`) đã **khóa từ checkout** và không đổi khi brand ship; chi phí thật với hãng vận lưu riêng (server-side), platform chịu chênh lệch. Hủy đơn / refund / xử lý hoàn-thất lạc: **chưa hỗ trợ** (spec sau).

### 4.10 Reviews sản phẩm — `/api/v1/*` (đọc public, ghi cần auth) — **MỚI (Social)**

Đánh giá sản phẩm. Chỉ khách **đã nhận hàng** (verified purchase) mới review được, mỗi sản phẩm 1 review/khách.

| Method | Path | Body / Query | Auth |
|--------|------|--------------|------|
| GET | `/products/:id/reviews` | query: `rating` (1-5), `fit` (`small`/`true`/`large`), `sort` (`newest`/`rating_high`/`rating_low`), `page`, `limit` (≤50) | — |
| POST | `/products/:id/reviews` | `{rating, body, fit?}` | Bearer (customer) |
| PATCH | `/reviews/:id` | `{rating, body, fit?}` | Bearer (chủ review) |
| DELETE | `/reviews/:id` | — | Bearer (chủ review) |

- `rating`: int 1–5 (bắt buộc). `body`: 20–5000 ký tự (bắt buộc). `fit`: optional, một trong `small` | `true` | `large`.

**`GET /products/:id/reviews` response:**
```json
{
  "items": [
    { "id":"uuid", "rating":5, "body":"Chất vải đẹp...", "fit":"true",
      "verified": true, "reviewer_name":"An N.", "created_at":"2026-06-10T08:00:00Z" }
  ],
  "avg_rating": 4.6,
  "review_count": 23,
  "pagination": { "page":1, "limit":20, "total":23, "total_pages":2 }
}
```
**Errors (Format A):** `PRODUCT_NOT_FOUND` (404), `NOT_VERIFIED_PURCHASE` (403 — chưa nhận hàng), `REVIEW_EXISTS` (409 — đã review rồi → dùng PATCH), `REVIEW_NOT_FOUND` (404), `FORBIDDEN` (403 — sửa/xóa review người khác), `INVALID_REVIEW` (400).

### 4.11 OOTD — feed cộng đồng — `/api/v1/*` — **MỚI (Social)**

"Outfit of the day": khách đăng ảnh phối đồ, gắn tag sản phẩm, like + comment. Đọc public (feed); ghi cần auth.

| Method | Path | Body / Query | Auth |
|--------|------|--------------|------|
| GET | `/ootd` | `page`, `limit` (≤50) | — (feed công khai) |
| GET | `/ootd/:id` | — | — |
| GET | `/ootd/:id/comments` | `page`, `limit` | — |
| GET | `/users/:id/ootd` | `page`, `limit` | — (post của 1 user) |
| GET | `/ootd-following` | `page`, `limit` | Bearer (feed người mình theo dõi) |
| POST | `/ootd` | **multipart/form-data** | Bearer |
| PATCH | `/ootd/:id` | `{caption}` (≤2000) | Bearer (chủ post) |
| DELETE | `/ootd/:id` | — | Bearer (chủ post) |
| POST | `/ootd/:id/like` | — | Bearer |
| DELETE | `/ootd/:id/like` | — | Bearer |
| POST | `/ootd/:id/comments` | `{body}` (1–500) | Bearer |
| DELETE | `/ootd-comments/:id` | — | Bearer (chủ comment) |

**`POST /ootd` (multipart):**
- `photos`: 1–10 file ảnh (mỗi file ≤5MB, JPG/PNG/WebP). Field lặp lại cho nhiều ảnh.
- `caption`: text optional (≤2000).
- `product_ids`: field lặp lại, mỗi giá trị là UUID product để gắn tag (optional).
- Response `201`: `{ "id": "uuid" }`.

**`PostResponse` (feed/detail):**
```json
{
  "id":"uuid", "author_name":"An N.", "caption":"Mix đồ đi chơi",
  "photo_urls":["https://.../1.jpg","https://.../2.jpg"],
  "like_count":12, "comment_count":3,
  "liked_by_me": true,
  "tags":[ { "product_id":"uuid", "slug":"oversized-tee", "name":"Oversized Tee" } ],
  "created_at":"2026-06-12T10:00:00Z"
}
```
- `liked_by_me`: chỉ đúng khi gửi kèm Bearer token (mọi route đều qua optional-auth); guest luôn `false`.
- Feed/comments envelope: `{ "items":[...], "pagination":{...} }`. Like/unlike trả `{ "liked": true|false }`. Comment tạo mới trả `{ "id":"uuid" }`.

**Errors (Format A):** `NO_PHOTOS` / `TOO_MANY_PHOTOS` / `FILE_TOO_LARGE` / `INVALID_MIME` / `CAPTION_TOO_LONG` (400), `POST_NOT_FOUND` / `COMMENT_NOT_FOUND` (404), `FORBIDDEN` (403 — sửa/xóa nội dung người khác).

### 4.12 Follow — theo dõi user & brand — `/api/v1/*` (auth) — **MỚI (Social)**

| Method | Path | Response | Notes |
|--------|------|----------|-------|
| POST | `/users/:id/follow` | `FollowStatus` | Theo dõi 1 user |
| DELETE | `/users/:id/follow` | `FollowStatus` | Bỏ theo dõi user |
| POST | `/brand-follows/:id` | `FollowStatus` | Theo dõi brand (`:id` = brand id) |
| DELETE | `/brand-follows/:id` | `FollowStatus` | Bỏ theo dõi brand |
| GET | `/me/following/users` | `{items[], pagination}` | Danh sách user đang theo dõi (`page`,`limit`) |
| GET | `/me/following/brands` | `{items[], pagination}` | Danh sách brand đang theo dõi |

- `FollowStatus`: `{ "following": true, "follower_count": 134 }` (idempotent — gọi lại không lỗi).
- `items` của `/me/following/users`: `{ id, name, avatar_url?, follower_count }`.
- `items` của `/me/following/brands`: `{ id, slug, name, logo_url?, follower_count }`.

**Errors (Format A):** `CANNOT_FOLLOW_SELF` (400), `USER_NOT_FOUND` (404), `BRAND_NOT_FOUND` (404).

### 4.13 Block — chặn user (UC40) — `/api/v1/*` (auth) — **MỚI (Moderation)**

Chặn 1 user: ẩn OOTD/bình luận **hai chiều** (mình không thấy họ và họ không thấy mình trong feed).

| Method | Path | Response | Notes |
|--------|------|----------|-------|
| POST | `/users/:id/block` | `{ "blocked": true }` | Chặn user (idempotent) |
| DELETE | `/users/:id/block` | `{ "blocked": false }` | Bỏ chặn |
| GET | `/me/blocks` | `{items[], pagination}` | Danh sách user đã chặn (`page`,`limit`) |

- `items`: `{ id, name, avatar_url? }`.
- **Effect:** chặn xong, OOTD của người bị chặn biến mất khỏi mọi feed của bạn (và ngược lại). FE không cần lọc thêm — backend đã filter.

**Errors (Format A):** `CANNOT_BLOCK_SELF` (400), `USER_NOT_FOUND` (404).

### 4.14 Stores — tìm cửa hàng / store locator — `/api/v1/stores/*` (public) — **MỚI**

Tìm cửa hàng brand theo vị trí (UC24 nearby, UC25 detail, UC26 directions). Dùng Goong cho khoảng cách + chỉ đường.

| Method | Path | Query | Notes |
|--------|------|-------|-------|
| GET | `/stores/nearby` | `lat`, `lng` (bắt buộc), `radius_km` (optional) | Cửa hàng gần 1 toạ độ, sắp theo khoảng cách |
| GET | `/stores` | `city_code`, `district_code`, `ward_code`, `q`, `lat`, `lng` (tất cả optional) | Tìm theo khu vực/từ khoá; nếu kèm `lat,lng` thì có thêm khoảng cách |
| GET | `/stores/:id` | — | Chi tiết 1 cửa hàng (giờ mở cửa, phone, banner) |
| GET | `/stores/:id/directions` | `from=lat,lng` (bắt buộc) | Chỉ đường từ vị trí khách tới cửa hàng |

**`GET /stores/nearby` & `/stores` response:** `{ "items": [ StoreSummary ] }`
```json
{
  "items": [{
    "id":"uuid", "brand_name":"REP VN", "brand_slug":"rep-vn", "logo_url":"https://...",
    "label":"Flagship Q1", "address_line":"123 Lê Lợi", "ward":"Bến Nghé", "district":"Quận 1", "city":"Hồ Chí Minh",
    "latitude":10.776, "longitude":106.700,
    "distance_m":1200, "duration_s":300, "distance_approx":false, "open":true
  }]
}
```
- `distance_m`/`duration_s`/`open` chỉ có khi query đủ điều kiện (nearby, hoặc search có `lat,lng`). `distance_approx=true` = khoảng cách đường chim bay (fallback khi Goong lỗi).

**`GET /stores/:id` response (`StoreDetail`)** = `StoreSummary` + `banner_url?`, `phone?`, `hours[]`:
```json
{
  "id":"uuid", "brand_name":"REP VN", "...": "...(các field StoreSummary)...",
  "banner_url":"https://...", "phone":"+842812345678",
  "hours":[ { "weekday":1, "open_time":"09:00", "close_time":"21:00" } ]
}
```
- `weekday`: 0=Chủ nhật … 6=Thứ Bảy.

**`GET /stores/:id/directions?from=10.78,106.70` response:**
```json
{ "distance_m": 3200, "duration_s": 720, "polyline": "encoded_polyline_string" }
```
- `polyline`: chuỗi encoded polyline (Google/Goong format) để vẽ route lên map.

**Errors (Format A):** `INVALID_QUERY` (400 — thiếu/sai `lat,lng` hoặc `from`), `INVALID_ID` (400), `STORE_NOT_FOUND` (404), `DIRECTIONS_UNAVAILABLE` (502 — Goong không trả route).

### 4.15 AI & Personalization — `/api/v1/me/*` (auth + role=customer) — **MỚI (AI)**

Ba tính năng cá nhân hoá cho customer: hồ sơ phong cách (UC31), gợi ý "For You" (UC29), tủ đồ thông minh (UC30). Tất cả scope theo user đăng nhập (IDOR-safe).

| Method | Path | Body / Query | Notes |
|--------|------|--------------|-------|
| GET | `/me/style-profile` | — | Hồ sơ phong cách; trả object rỗng (`style_tags:[]`) nếu chưa set |
| PUT | `/me/style-profile` | `{style_tag_ids[] (≤10), budget_min?, budget_max?}` | Upsert; ghi đè toàn bộ (editable anytime) |
| GET | `/me/recommendations` | `limit` (default 20, max 50) | Feed "For You" cá nhân hoá / trending |
| GET | `/me/wardrobe` | — | Tủ đồ + outfit gợi ý (cache theo signature) |
| POST | `/me/wardrobe/regenerate` | — | Buộc tạo lại outfit (nút "làm mới") |

#### UC31 — Style Profile

`style_tag_ids` lấy từ endpoint công khai `/style-tags` (§4.3). `budget_*` đơn vị VND.

**`GET` / `PUT /me/style-profile` response (`StyleProfileResponse`):**
```json
{
  "style_tags": [ { "id": "uuid", "slug": "minimal", "name": "Minimalist" } ],
  "budget_min": 100000,
  "budget_max": 800000,
  "onboarded_at": "2026-06-16T10:00:00Z"
}
```
- Chưa set hồ sơ → `{ "style_tags": [] }` (các field budget/onboarded_at vắng mặt).
- `onboarded_at` set lần đầu PUT, giữ nguyên các lần sau.

**Errors:** `VALIDATION_FAILED` (400) khi: có `style_tag_id` không tồn tại (kèm `details.unknown_style_tag_ids`), `budget_max < budget_min`, hoặc quá 10 tag.

#### UC29 — Recommendations ("For You")

Heuristic deterministic dựa trên style profile + brand đang follow + lịch sử mua + wishlist. Cache theo ngày, tự làm mới khi đổi profile.

**`GET /me/recommendations?limit=20` response:**
```json
{
  "items": [
    { "id":"uuid", "slug":"midi-dress", "name":"Midi Dress",
      "brand_slug":"rep-vn", "brand_name":"REP VN",
      "currency":"VND", "min_price":650000, "primary_image":"https://..." }
  ],
  "source": "personalized",
  "onboarding_prompt": false
}
```
- `source`: `"personalized"` (có profile/lịch sử) hoặc `"trending"` (người dùng mới).
- `onboarding_prompt: true` → FE nên nhắc user làm quiz style (PUT `/me/style-profile`).

#### UC30 — Smart Wardrobe

Phối outfit bằng AI (Gemini) từ đồ đã mua + gợi ý mua thêm. Luôn dùng được kể cả khi chưa mua gì (gợi ý nguyên outfit để mua, dựa trên style profile).

**`GET /me/wardrobe` & `POST /me/wardrobe/regenerate` response (`WardrobeResponse`):**
```json
{
  "closet": [ { "id":"uuid", "slug":"...", "name":"Áo thun trắng", "brand_slug":"...", "brand_name":"...", "currency":"VND", "min_price":250000, "primary_image":"https://..." } ],
  "outfits": [
    {
      "title": "Smart casual cuối tuần",
      "note": "Phối áo thun với quần tây sáng màu cho vẻ thanh lịch.",
      "owned":  [ { "id":"uuid", "name":"Áo thun trắng" } ],
      "to_buy": [ { "id":"uuid", "slug":"...", "name":"Quần tây be", "brand_slug":"...", "brand_name":"...", "currency":"VND", "min_price":420000, "primary_image":"https://..." } ]
    }
  ],
  "outfits_status": "ready",
  "onboarding_prompt": false
}
```
- `closet`: sản phẩm đã mua (đơn đã giao). Rỗng nếu chưa mua gì.
- Mỗi outfit gồm `owned` (đồ đang có — có thể rỗng) + `to_buy` (gợi ý mua, còn hàng).
- Closet rỗng → `owned` rỗng, `to_buy` là cả outfit (grounded theo style profile).
- `outfits_status`: `"ready"` bình thường; **`"unavailable"`** khi AI provider lỗi/timeout — lúc đó `outfits: []` nhưng `closet` vẫn trả về (degrade mềm, FE nên cho retry).
- `onboarding_prompt: true` khi chưa có profile lẫn lịch sử mua.
- Outfit được cache server-side; chỉ tạo lại khi tủ đồ hoặc style profile đổi. `POST /regenerate` để buộc tạo lại.

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

### 5.1 Goship shipping status webhook (server→server) — **MỚI**

Trạng thái giao hàng do **Goship** đẩy về backend, **không liên quan FE trực tiếp**:

```
Brand bấm Ship (POST /brand/me/orders/:id/ship)
  ↓ BE tạo đơn Goship → lưu tracking_no + tracking_url, sub_order = shipped
  ↓ Goship gọi POST /shipping/goship/webhook khi trạng thái đổi (ký x-goship-hmac-sha256)
  ↓ BE map status → sub_order (shipped → delivered); khi delivered: COD → paid + trừ kho; tất cả sub_order delivered → order = completed
  ↓
FE chỉ cần POLL GET /me/orders/:order_no để thấy sub_orders[].status / tracking_no / tracking_url / shipping_status_text cập nhật
```

- FE **không** gọi webhook này. Để hiển thị tracking cho khách: poll `GET /me/orders/:order_no` (hoặc fetch khi mở màn order detail) và đọc `sub_orders[].{status, tracking_no, tracking_url, shipping_status_text}`.
- **Dev:** `POST /dev/goship/simulate` (chỉ mock mode) body `{tracking_no, status, is_return?, is_lost?}` để mô phỏng webhook — ví dụ `status:"Đã giao hàng"` để chuyển sub-order sang delivered. Dùng test luồng giao hàng không cần Goship thật.
- Endpoint webhook: `POST /api/v1/shipping/goship/webhook` (public, server xác thực HMAC).

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
| Shipping provider | `SHIPPING_PROVIDER=goship` (hoặc `flat`) | `goship` → phí ship + carrier options thật; `flat` → 1 option phí phẳng (`carrier:"flat"`) |
| Goship mode | `GOSHIP_MODE=mock\|sandbox\|production` | `mock` → location/rates/shipment giả lập + bật `/dev/goship/simulate`; production cần `GOSHIP_TOKEN`/`GOSHIP_CLIENT_SECRET` |

> Khi `SHIPPING_PROVIDER=flat` hoặc `GOSHIP_MODE=mock`, `shipping_options[].carrier` sẽ là giá trị giả lập (`"flat"` hoặc tên hiển thị mock như `"Giao Hàng Nhanh (v3)"`) — FE vẫn xử lý đồng nhất: hiển thị options, gửi lại `carrier` đã chọn.

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
- [ ] **Address form: dropdown phân cấp** dùng `/locations/cities → /districts → /wards`; gửi kèm `city_code/district_code/ward_code` (+ `label`) khi tạo/sửa địa chỉ.
- [ ] Checkout flow: preview → **khách chọn carrier mỗi brand** từ `sub_orders[].shipping_options` → place_order với `shipping_selections` → (nếu PayOS) WebView → poll detail.
- [ ] Xử lý `address_incomplete=true` ở preview → ép khách cập nhật địa chỉ (bổ sung mã) trước khi đặt.
- [ ] Order detail: hiển thị **tracking** mỗi sub-order (`status`, `shipping_carrier`, `tracking_url`, `shipping_status_text`); poll lại để thấy cập nhật từ webhook Goship.
- [ ] (Brand app) màn fulfillment: list `/brand/me/orders`, confirm, ship (chọn carrier) — §4.9.
- [ ] Reviews: list trên product detail (§4.10); form viết review chỉ mở khi khách đã nhận hàng (bắt `NOT_VERIFIED_PURCHASE`/`REVIEW_EXISTS`).
- [ ] OOTD feed + detail (§4.11): post dùng **multipart** `photos` (1–10) + `product_ids`; like/comment authed; `liked_by_me` cần gửi token.
- [ ] Follow/Unfollow user & brand (§4.12) + màn "đang theo dõi"; feed người theo dõi qua `/ootd-following`.
- [ ] Block/Unblock user (§4.13) + màn "đã chặn"; backend đã filter feed hai chiều, FE không lọc thêm.
- [ ] Store locator (§4.14): xin quyền vị trí → `/stores/nearby?lat&lng`; map detail + `/stores/:id/directions` vẽ `polyline`.
- [ ] Handle 401 silently (interceptor đã làm); 403 → "không có quyền"; 409 (`carrier_unavailable`/cancel subcode) → preview lại / báo lý do.
- [ ] Hiển thị warnings từ `/checkout/preview.warnings` (low stock / unavailable).
- [ ] Test full flow trên Android emulator (10.0.2.2) + iOS sim (localhost).
- [ ] Khi đẩy build prod, đảm bảo `baseUrl` đổi sang HTTPS và `PAYOS_MODE=production` ở server.

---

**Liên hệ backend:** đọc spec để hiểu lifecycle:
- Order/payment: `docs/superpowers/specs/2026-05-24-sprint-3-orders-checkout-payos-design.md`
- Goship rates + địa chỉ + carrier checkout: `docs/superpowers/specs/2026-06-03-goship-shipping-rates-design.md`
- Goship fulfillment (confirm/ship + tracking webhook): `docs/superpowers/specs/2026-06-04-goship-fulfillment-design.md`

Nếu gặp lỗi không có trong tài liệu này, kiểm tra `internal/<module>/handler/` để xem code-string chính xác.
