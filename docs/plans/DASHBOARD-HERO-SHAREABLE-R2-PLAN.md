# Dashboard Hero Card: Shareable WebP & Cloudflare R2 Implementation Plan

## 1. Goal & Two-Tier Architecture
Provide seamless, viral sharing for the Dashboard MTM Hero Card with **0% backend compute and storage overhead**:

1. **Tier 1 — Social Preview Image (`og:image`)**:
   - Shows **strictly the Hero Card** (1200×630 px WebP, 16:9 aspect ratio, clean, bold, zero clutter).
   - Displayed inside Twitter/X card, WhatsApp, Telegram, Discord, and iMessage previews.
2. **Tier 2 — Public Web Landing Page (`share.envoytrade.com/s/:id`)**:
   - Opened when a user clicks the shared link.
   - **Top Section**: The Hero Card (same visual styling as the Dashboard).
   - **Bottom Section**: **Detailed Account Breakdown** of the Master and Followers matching the exact scope selected in the right card dropdown at the time of sharing!

```
[ User Shares from Dashboard ]
              |
              | 1. Generates 1200x630 Hero Card WebP (Tier 1 Image)
              | 2. Compiles Scoped Breakdown Snapshot (Tier 2 Page)
              | 3. Requests R2 Presigned PUT URLs from Go Backend
              v
[ Go Backend (Presigner) ] ---> Signs 2 S3 URLs (~0.1ms, zero image/HTML bandwidth)
              |
              | 4. Returns { uploadImageUrl, uploadPageUrl, publicUrl }
              v
[ User Browser ]
              |
              | 5. Direct PUT uploadImageUrl (Hero WebP ~35 KB)
              | 6. Direct PUT uploadPageUrl (Static HTML Page ~15 KB)
              v
[ Cloudflare R2 Bucket: envoytrade-shares ]
              |
      +-------+-------+
      |               |
      v               v
[ Social Scrapers ] [ Web Visitors ]
Reads <id>.webp     Opens share.envoytrade.com/s/:id
(Hero Card only)    (Hero Card TOP + Breakdown Table BOTTOM)
```

---

## 2. Tier 1: Social Preview Image Spec (1200 × 630 px WebP)

Universal Safe-Zone design prevents social platforms from cropping off crucial data (X 1.91:1 banner vs WhatsApp 1:1 center-crop).

```
+-------------------------------------------------------------------+
|  Margin (285px)        |   SAFE ZONE (630 × 630)   | Margin (285px) |
|                        |                           |                |
|  [User Avatar]         |      [EnvoyTrade Logo]    |   [Verified]   |
|  [Trader Name]         |                           |                |
|                        |      ● COMBINED MTM       |                |
|                        |       +₹4,500.50          |                |
|                        |                           |                |
|                        |     [3 Active Accounts]   |   [Timestamp]  |
+-------------------------------------------------------------------+
|<-------------------------- 1200 px ------------------------------>|
```

- **Canvas dimensions**: 1200 × 630 px.
- **Center 630 × 630 px (Safe Zone)**:
  - EnvoyTrade Brand Logo.
  - Active MTM Scope Pill (e.g., `● COMBINED MTM`).
  - Hero MTM value in tabular bold font (`+₹4,500.50`).
  - Active accounts / follower count subtitle.
- **Left / Right Margins (285px each)**:
  - Left: User avatar circle + full name / handle.
  - Right: Shield Verified badge + IST Timestamp.
- **Format**: `image/webp` (quality 0.85, **~30 KB – 40 KB**).
- **Behavior**: Shown exclusively in social link unfurls (`og:image`, `twitter:image`).

---

## 3. Tier 2: Public Landing Page & Scoped Breakdown Section

When a recipient clicks `share.envoytrade.com/s/:id`, the browser loads a standalone, themed static HTML page with two distinct vertical sections:

```
+-------------------------------------------------------------------+
|                         [ NAVBAR / BRAND ]                        |
|                     EnvoyTrade Verified Share                     |
+-------------------------------------------------------------------+
|                                                                   |
|                       SECTION 1: HERO CARD                        |
|    +---------------------------------------------------------+    |
|    |  [Logo] EnvoyTrade                       [User Avatar]  |    |
|    |                     ● COMBINED MTM                      |    |
|    |                      +₹4,500.50                         |    |
|    |               Combined across 3 accounts                |    |
|    |  [Verified Badge]                  [Taken @ 26 Oct IST] |    |
|    +---------------------------------------------------------+    |
|                                                                   |
|               SECTION 2: ACCOUNT BREAKDOWN DETAIL                 |
|   (Populated dynamically based on scope selected during share)    |
|                                                                   |
|   SCOPE: "All Masters"                                            |
|   +---------------------------------------------------------+     |
|   | Account / Master        Broker   Followers   Status   MTM |   |
|   | Rajesh Sharma (ZX1234)  Kite     4           Active  +3000|   |
|   | Priya Patel (PR5678)    Dhan     2           Active  +1500|   |
|   +---------------------------------------------------------+     |
|                                                                   |
|   OR SCOPE: Specific Group (e.g. "Alpha Traders")                 |
|   +---------------------------------------------------------+     |
|   | Role     Account Name       Broker   Clone   MaxQty   MTM |   |
|   | Master   Rajesh (ZX1234)    Kite     —       —       +3000|   |
|   | Follower Rohan (FO100)      Kite     1.0     500      +800|   |
|   | Follower Amit (FO200)       Dhan     0.5     200      +700|   |
|   +---------------------------------------------------------+     |
|                                                                   |
|               [ Call-to-Action: Trade on EnvoyTrade ]             |
+-------------------------------------------------------------------+
```

### Breakdown Data Scopes
The snapshot captures the exact state of the right card dropdown at the moment the user clicks Share:

1. **Scope = `all_masters`**:
   - Master account name & broker account ID.
   - Broker logo badge (Zerodha / Dhan / TestBroker).
   - Follower count.
   - Connection & copy status (Active / Inactive).
   - Master individual MTM.
2. **Scope = `all_groups`**:
   - Group Name & status.
   - Master account summary & MTM.
   - Attached follower accounts, clone factors, and follower MTM contributions.
   - Aggregate group total MTM.
3. **Scope = Specific Group (`groupId`)**:
   - Group name header.
   - Master row with individual MTM.
   - Follower rows: Account name, Broker ID, Broker logo, Clone factor, Max quantity cap, Status, and follower individual MTM.

---

## 4. Zero-Backend Direct-to-R2 Workflow

Both Tier 1 (Image) and Tier 2 (Page) are generated client-side and saved directly to Cloudflare R2:

1. **User Clicks "Share"**:
   - Frontend captures current `HeroCardData` + active `ScopeBreakdownData`.
2. **Presign Request** (`POST /api/v1/share/presign`):
   - Backend generates a UUID `id`.
   - Backend signs two S3 PUT URLs with 5-minute TTL:
     - `uploadImageUrl`: `envoytrade-shares/images/:id.webp`
     - `uploadPageUrl`: `envoytrade-shares/s/:id.html`
3. **Frontend Direct Uploads**:
   - Uploads WebP image via `fetch(uploadImageUrl, { method: 'PUT', body: webpBlob, headers: { 'Content-Type': 'image/webp' } })`.
   - Compiles self-contained static HTML page (injecting the Hero Card HTML, breakdown table, inline minified CSS, and OpenGraph tags pointing to `images/:id.webp`).
   - Uploads HTML page via `fetch(uploadPageUrl, { method: 'PUT', body: htmlString, headers: { 'Content-Type': 'text/html' } })`.
4. **Instant Link Ready**:
   - User gets `https://share.envoytrade.com/s/:id`.
   - Copied to clipboard or opened in native share sheet.

---

## 5. Storage & Cost Model (50 Daily Active Users)

- **Trading days**: ~250 days/year.
- **Daily volume**: 50 shares/day.
- **Per Share Payload**:
  - Image (`.webp`): ~35 KB
  - HTML Page (`.html`): ~15 KB
  - **Total per share**: **~50 KB**.
- **Annual Storage**:
  - `50 KB × 50 users × 250 days` = **625 MB / year (0.62 GB / year)**.
- **Cloudflare R2 Free Allowance**:
  - **10 GB storage / month free forever**.
  - **1,000,000 upload operations (Class A) / month free** (Usage: ~2,100 / month).
  - **$0 egress fees** (bandwidth 100% free).
- **Runway without deletion**: **~16 Years**.
- **Runway with 90-day R2 Lifecycle Auto-Delete Rule**: **Infinite 100% Free** (bucket never exceeds ~160 MB).

---

## 6. Implementation Components

### Component 1: Go Backend Presigned URL Endpoint
**File**: `internal/httpapi/share.go`
- Endpoint: `POST /api/v1/share/presign` (Session authenticated).
- Generates 2 presigned S3 URLs using AWS S3 Go SDK configured for Cloudflare R2:
  - `uploadImageUrl` (`images/<id>.webp`)
  - `uploadPageUrl` (`s/<id>.html`)
  - `publicShareUrl` (`https://share.envoytrade.com/s/<id>`)

### Component 2: Frontend Canvas Exporter (Hero Card WebP)
**File**: `frontend/src/utils/shareCanvas.ts`
- `renderHeroCardToWebP(data: HeroCardData): Promise<Blob>`:
  - Draws 1200×630 Canvas with 630×630 Center Safe Zone.
  - Outputs `image/webp` blob (quality 0.85).

### Component 3: Frontend Snapshot HTML Builder (Page with Breakdown)
**File**: `frontend/src/utils/shareHtmlBuilder.ts`
- `buildShareHtml(data: HeroCardData, breakdown: ScopedBreakdownData, imageUrl: string): string`:
  - Builds clean, responsive HTML document.
  - `<head>`: Sets `og:image`, `og:title`, `og:description`, `twitter:card="summary_large_image"`.
  - `<body>`:
    - Top: Hero Card matching dashboard visual theme.
    - Bottom: Scoped breakdown table with broker badges, clone factors, and individual account MTM values.
    - Footer: "Verified by EnvoyTrade | Copy-Trade Platform".

### Component 4: Dashboard Share UI & Modal
**File**: `frontend/src/components/DashboardHeroCard.tsx`
- Add **"Share"** button in Hero Card controls panel.
- Share Modal with 3 actions:
  1. **Copy Image**: Copies WebP/PNG directly to clipboard.
  2. **Download Image**: Downloads `envoy-mtm-<date>.webp`.
  3. **Share Link**: Triggers presign + direct R2 upload + copies `https://share.envoytrade.com/s/:id`.
  4. Native `navigator.share` on mobile devices.

---

## 7. Cloudflare R2 & Domain Configuration

1. **Bucket**: `envoytrade-shares` in Cloudflare Dashboard.
2. **Custom Domain**: Connect `share.envoytrade.com` to `envoytrade-shares` bucket.
3. **CORS Configuration**:
   ```json
   [
     {
       "AllowedOrigins": ["https://mime.envoytrade.com", "http://localhost:5173"],
       "AllowedMethods": ["PUT", "GET"],
       "AllowedHeaders": ["Content-Type"]
     }
   ]
   ```
4. **Lifecycle Policy**: Auto-delete objects older than **90 days**.

---

## 8. Verification & TDD Plan

### Automated Unit Tests
1. `shareCanvas.test.ts`:
   - Verify 1200×630 dimensions.
   - Verify WebP MIME type and size under 45 KB.
2. `shareHtmlBuilder.test.ts`:
   - Verify generated HTML contains required OpenGraph and Twitter meta tags.
   - Verify scoped breakdown tables render master and follower rows accurately for `all_masters`, `all_groups`, and specific group IDs.
3. `internal/httpapi/share_test.go`:
   - Verify presign endpoint generates valid signed URLs with short 5-minute TTL.

### Manual Verification
1. Click "Share" with "All Masters" selected -> Verify generated link opens page with Master breakdown table.
2. Click "Share" with specific group selected -> Verify generated link opens page with that group's follower breakdown table.
3. Paste link in Twitter Card Validator and WhatsApp -> Confirm preview displays strictly the clean 1200×630 Hero Card without breakdown clutter.
4. Click link on phone/desktop -> Confirm landing page loads both Hero Card and Breakdown cleanly in dark mode.
