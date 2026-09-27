# Authentication API (`/api/v1/auth`)

EnvoyTrade provides database-backed session authentication with multi-tenant data scoping.

## Security Model

- **Session Tokens**: 32 cryptographically secure random bytes generated via `crypto/rand`.
- **Database Storage**: Tokens are stored as SHA-256 hashes in `sessions.token_hash`. Plain tokens are never stored.
- **Client Transport**: Plain token transmitted via HTTP-only cookie (`envoytrade_session`, `SameSite=Lax`, `Secure` in production). Fallback support via `Authorization: Bearer <token>` for API clients/scripts.
- **Session Lifetime**: 24 hours. Rolling refresh updates `last_seen_at` and `expires_at` on active requests. Background daemon purges expired sessions hourly.
- **Passwords**: Hashed with bcrypt (cost 12).
- **Public Routes (No Session Required)**:
  - `POST /broker-callback` (verified independently via Kite HMAC-SHA256 signature)
  - `GET /api/v1/health`
  - `GET /api/v1/ready`
  - `POST /api/v1/auth/register`
  - `POST /api/v1/auth/login`
- **Protected Routes**: All other `/api/v1/*` routes require a valid session. Unauthenticated calls return `401 Unauthorized` with `{"error": "unauthenticated"}`.

---

## 1. Register New User

Creates a new platform user, initializes an active session, and sets the `envoytrade_session` cookie.

- **Method**: `POST`
- **Path**: `/api/v1/auth/register`
- **Auth**: Public

### Request Body

```json
{
  "email": "trader@example.com",
  "username": "trader1",
  "name": "Jane Doe",
  "password": "securepassword123"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `email` | string | Yes | Valid email address, unique across users. |
| `username` | string | Yes | Unique alphanumeric username. |
| `name` | string | No | Optional human display name. |
| `password` | string | Yes | Minimum 8 characters. |

### Responses

#### `201 Created`
Sets HTTP-only session cookie.

```json
{
  "user": {
    "id": "a0000000-0000-0000-0000-000000000001",
    "email": "trader@example.com",
    "username": "trader1",
    "name": "Jane Doe",
    "role": "user"
  },
  "token": "4f9d..."
}
```

#### `400 Bad Request`
```json
{
  "error": "password must be at least 8 characters"
}
```

#### `409 Conflict`
```json
{
  "error": "user with this email or username already exists"
}
```

---

## 2. Login

Authenticates user via email and password, creates a new session, and sets the `envoytrade_session` cookie.

- **Method**: `POST`
- **Path**: `/api/v1/auth/login`
- **Auth**: Public

### Request Body

```json
{
  "email": "trader@example.com",
  "password": "securepassword123"
}
```

### Responses

#### `200 OK`
Sets HTTP-only session cookie.

```json
{
  "user": {
    "id": "a0000000-0000-0000-0000-000000000001",
    "email": "trader@example.com",
    "username": "trader1",
    "name": "Jane Doe",
    "role": "user"
  },
  "token": "4f9d..."
}
```

#### `401 Unauthorized`
```json
{
  "error": "invalid email or password"
}
```

---

## 3. Get Current User

Returns current authenticated user identity from active session.

- **Method**: `GET`
- **Path**: `/api/v1/auth/me`
- **Auth**: Required (`envoytrade_session` cookie or `Authorization: Bearer <token>`)

### Responses

#### `200 OK`
```json
{
  "user": {
    "id": "a0000000-0000-0000-0000-000000000001",
    "email": "trader@example.com",
    "username": "trader1",
    "name": "Jane Doe",
    "role": "user"
  }
}
```

#### `401 Unauthorized`
```json
{
  "error": "unauthenticated"
}
```

---

## 4. Logout

Deletes session record from PostgreSQL and clears the `envoytrade_session` client cookie (`MaxAge: -1`).

- **Method**: `POST`
- **Path**: `/api/v1/auth/logout`
- **Auth**: Public / Best-effort

### Responses

#### `200 OK`
```json
{
  "status": "ok"
}
```

---

## 5. Update User Profile

Updates the authenticated user's email, username, name, phone number, address, and GSTIN.

- **Method**: `PUT`
- **Path**: `/api/v1/user/profile`
- **Auth**: Required (`envoytrade_session` cookie or `Authorization: Bearer <token>`)

### Request Body

```json
{
  "email": "trader@envoytrade.com",
  "username": "trader",
  "name": "Demo Trader",
  "phone": "+919876543210",
  "address": "123 Dalal Street, Fort, Mumbai 400001",
  "gstNumber": "27AABCU9603R1ZM"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `email` | string | Yes | Valid email address, unique across users. |
| `username` | string | Yes | Unique alphanumeric username. |
| `name` | string | No | Optional full name. |
| `phone` | string | No | Optional 10-digit Indian mobile number (normalized to `+91<10 digits>`). |
| `address` | string | No | Optional physical address. |
| `gstNumber` | string | No | Optional 15-character Indian GSTIN format. |

### Responses

#### `200 OK`
```json
{
  "user": {
    "id": "a0000000-0000-0000-0000-000000000001",
    "email": "trader@envoytrade.com",
    "username": "trader",
    "name": "Demo Trader",
    "phone": "+919876543210",
    "address": "123 Dalal Street, Fort, Mumbai 400001",
    "gstNumber": "27AABCU9603R1ZM",
    "role": "admin"
  }
}
```

#### `400 Bad Request`
```json
{
  "error": "invalid phone number: must be a 10-digit Indian mobile number"
}
```

#### `409 Conflict`
```json
{
  "error": "user with this email or username already exists"
}
```

---

## 6. Change Password

Updates the authenticated user's password. Verifies the current password, enforces minimum length of 8 characters, hashes new password with bcrypt (cost 12), and invalidates all other active sessions for that user across devices while preserving the current session.

- **Method**: `POST`
- **Path**: `/api/v1/user/password`
- **Auth**: Required (`envoytrade_session` cookie or `Authorization: Bearer <token>`)

### Request Body

```json
{
  "oldPassword": "password123",
  "newPassword": "NewSecurePassword456!"
}
```

### Responses

#### `200 OK`
```json
{
  "status": "ok"
}
```

#### `400 Bad Request`
```json
{
  "error": "new password must be at least 8 characters"
}
```

#### `401 Unauthorized`
```json
{
  "error": "incorrect current password"
}
```

