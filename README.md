# HttpOverVercel — راهنمای کامل راه‌اندازی

پروکسی HTTP/HTTPS که تمام ترافیک را از طریق یک Vercel Edge Function تونل می‌کند.
مناسب برای مناطقی که تنها دسترسی به سرویس‌های Vercel وجود دارد.

> **نسخه Go** — بدون نیاز به Python یا pip. یک فایل اجرایی تکی، کارایی بالا، مصرف پهنای باند ۳۳٪ کمتر نسبت به نسخه قدیمی.

---

## ۰) دانلود فایل آماده (پیشنهادی)

نیازی به نصب Go نیست — فایل باینری آماده را از صفحه Releases دانلود کنید:

👉 **https://github.com/logicalangel/HttpOverVercel/releases/latest**

| سیستم‌عامل | فایل |
|---|---|
| Linux x64 | `proxy-linux-amd64` |
| Linux ARM64 | `proxy-linux-arm64` |
| macOS Intel | `proxy-darwin-amd64` |
| macOS Apple Silicon | `proxy-darwin-arm64` |
| Windows x64 | `proxy-windows-amd64.exe` |

بعد از دانلود روی Linux/macOS:

```bash
chmod +x proxy-*
./proxy-linux-amd64 -c config.json   # یا proxy-darwin-arm64 و غیره
```

---

## ۱) پیش‌نیازها (فقط برای build از سورس)

| ابزار | توضیح |
|---|---|
| اکانت Vercel | برای دیپلوی Edge Function |
| `git` | دریافت پروژه |
| `npm` | نصب Vercel CLI |
| Go 1.21+ | فقط اگر می‌خواهید از سورس build کنید |

نصب Node.js (برای npm): https://nodejs.org/en/download

---

## ۲) دریافت پروژه

```bash
git clone https://github.com/logicalangel/HttpOverVercel.git
cd HttpOverVercel
```

---

## ۳) ساخت کلاینت Go (اختیاری — اگر فایل آماده دانلود نکردید)

### از سورس (همه سیستم‌عامل‌ها)

```bash
cd go
go build -o ../proxy ./cmd/proxy
cd ..
```

### کامپایل برای ویندوز (از Linux/macOS)

```bash
cd go
GOOS=windows GOARCH=amd64 go build -o ../proxy.exe ./cmd/proxy
cd ..
```

### کامپایل برای Linux (از Windows/macOS)

```bash
cd go
GOOS=linux GOARCH=amd64 go build -o ../proxy ./cmd/proxy
cd ..
```

---

## ۴) نصب Vercel CLI و دیپلوی

```bash
npm i -g vercel
```

اگر به رجیستری اصلی دسترسی ندارید:

```bash
npm i -g vercel --registry="https://mirror-npm.runflare.com"
```

وارد پوشه `vercel` شوید و مراحل زیر را طی کنید:

```bash
cd vercel
vercel login
vercel        # Preview deploy (اولین بار)
vercel --prod # Production deploy
cd ..
```

> **مهم:** در داشبورد Vercel، وارد `Project → Settings → General → Root Directory` شوید و مقدار `vercel` را وارد کنید تا هر push به GitHub به صورت خودکار از پوشه درست دیپلوی شود.

---

## ۵) تنظیم متغیرهای محیطی در Vercel

در داشبورد Vercel وارد `Project → Settings → Environment Variables` شوید:

| Key | Value | توضیح |
|---|---|---|
| `AUTH_KEY` | یک کلید امن دلخواه | احراز هویت بین کلاینت و Edge Function |
| `STATS_USER` | نام کاربری (پیش‌فرض: `admin`) | ورود به صفحه آمار |
| `STATS_PASS` | رمز عبور (پیش‌فرض: `changeme`) | ورود به صفحه آمار |

بعد از ذخیره، حتماً Redeploy کنید:

```bash
vercel --prod --yes
```

---

## ۶) راه‌اندازی آمار (اختیاری — با Upstash Redis)

برای فعال‌سازی صفحه آمار به یک Redis نیاز دارید.

### روش اول: از طریق داشبورد Vercel
`Project → Storage → Add Store → Upstash Redis (رایگان)` → Create → Link to project

Vercel به صورت خودکار `UPSTASH_REDIS_REST_URL` و `UPSTASH_REDIS_REST_TOKEN` را inject می‌کند.

### روش دوم: دستی

```bash
printf 'https://YOUR_ENDPOINT.upstash.io' | vercel env add UPSTASH_REDIS_REST_URL production
printf 'YOUR_TOKEN' | vercel env add UPSTASH_REDIS_REST_TOKEN production
vercel --prod --yes
```

### دسترسی به صفحه آمار

```
https://YOUR_PROJECT.vercel.app/api/stats
```

با `STATS_USER` و `STATS_PASS` وارد شوید. شامل تعداد درخواست‌ها، بایت‌های ارسال‌شده، خطاها و جدول ۲۵ دامنه پراستفاده.

> اگر Redis راه‌اندازی نشده باشد، صفحه آمار باز می‌شود اما اعداد صفر نمایش می‌دهد. پروکسی اصلی بدون مشکل کار می‌کند.

---

## ۶) نکته مهم امنیتی Vercel

اگر **Deployment Protection** روشن باشد، پروکسی با خطا مواجه می‌شود.

- `Project → Settings → Deployment Protection`
- احراز هویت را برای دامنه مورد استفاده **خاموش** کنید.

---

## ۷) ساخت فایل تنظیمات

```bash
cp config.example.json config.json
```

فایل `config.json` را ویرایش کنید:

```json
{
  "worker_host": "YOUR_PROJECT.vercel.app",
  "relay_path": "/api/api",
  "auth_key": "changeme",
  "stats_user": "admin",
  "stats_pass": "changeme",
  "listen_host": "127.0.0.1",
  "listen_port": 8085,
  "log_level": "INFO",
  "verify_ssl": true
}
```

| فیلد | توضیح |
|---|---|
| `worker_host` | آدرس پروژه Vercel بدون `https://` (بخش Domains در داشبورد) |
| `auth_key` | باید دقیقاً برابر `AUTH_KEY` در Vercel باشد |
| `stats_user` / `stats_pass` | باید با `STATS_USER` / `STATS_PASS` در Vercel یکسان باشد |
| `relay_path` | مقدار ثابت `/api/api` |
| `listen_port` | پورت پروکسی محلی (پیش‌فرض: `8085`) |

همچنین می‌توانید از متغیرهای محیطی استفاده کنید:

```bash
export DFT_AUTH_KEY=my-secret
export DFT_HOST=YOUR_PROJECT.vercel.app
```

---

## ۸) اجرای پروکسی

```bash
./proxy -c config.json
```

گزینه‌های خط فرمان:

```
-c, --config <path>     مسیر فایل config (پیش‌فرض: config.json)
-p <port>               override پورت
--host <host>           override آدرس listen
--log-level DEBUG|INFO|WARNING|ERROR
--install-cert          نمایش دستورات نصب گواهی CA و خروج
--version               نمایش نسخه و خروج
```

در اولین اجرا، پوشه `ca/` ساخته می‌شود و گواهی CA در آن ذخیره می‌شود.

---

## ۹) نصب گواهی CA (برای HTTPS ضروری است)

برای مشاهده دستورات دقیق نصب:

```bash
./proxy --install-cert
```

### macOS

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain ca/ca.crt
```

### Windows (PowerShell به عنوان Administrator)

```powershell
Import-Certificate -FilePath "ca\ca.crt" -CertStoreLocation Cert:\LocalMachine\Root
```

### Linux (Ubuntu/Debian)

```bash
sudo cp ca/ca.crt /usr/local/share/ca-certificates/HttpOverVercel.crt
sudo update-ca-certificates
```

### Firefox (همه سیستم‌عامل‌ها)

Firefox از CA سیستم استفاده نمی‌کند؛ باید دستی import کنید:

1. `Settings → Privacy & Security → Certificates → View Certificates`
2. تب `Authorities` → `Import`
3. فایل `ca/ca.crt` را انتخاب کنید
4. گزینه `Trust this CA to identify websites` را فعال کنید

---

## ۱۰) استفاده از پروکسی

بعد از اجرا، پروکسی HTTP روی `127.0.0.1:8085` فعال می‌شود.

در تنظیمات مرورگر یا سیستم:

- **Proxy Host:** `127.0.0.1`
- **Proxy Port:** `8085`
- **Type:** HTTP

---

## ۱۱) تست سریع سلامت

```bash
curl -x http://127.0.0.1:8085 -sS https://httpbin.org/get
```

باید یک JSON با `"origin"` برابر IP سرور Vercel برگردد (نه IP شما).

---

## ۱۲) ساختار پروژه

```
.github/workflows/release.yml  — CI/CD: ساخت باینری برای همه پلتفرم‌ها
go/
  cmd/proxy/        — نقطه ورود CLI
  internal/config/  — بارگذاری تنظیمات
  internal/mitm/    — مدیریت گواهی CA و TLS
  internal/relay/   — کلاینت relay (پروتکل باینری)
  internal/proxy/   — سرور پروکسی HTTP/HTTPS
vercel/
  api/api.js        — Vercel Edge Function (پروکسی اصلی)
  api/stats.js      — صفحه آمار (محافظت‌شده با Basic Auth)
config.example.json
```

---

تمام.

---
DONATE

usdt (trc20)
TL9y6ejgFPgL8w1SyHuXCZbDrnUW4SXbEu

TON
UQC7PDo_Lw7a0R26KA9DMeTd5c1XY6NIDIpqzckfi326RROO
