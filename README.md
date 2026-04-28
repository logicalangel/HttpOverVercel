# HttpOverVercel — راهنمای کامل راه‌اندازی

پروکسی HTTP/HTTPS که تمام ترافیک را از طریق یک Vercel Edge Function تونل می‌کند.  
مناسب برای مناطقی که تنها دسترسی به سرویس‌های Vercel وجود دارد.

> **نسخه Go** — بدون نیاز به Python یا pip. یک فایل اجرایی تکی، کارایی بالا، مصرف پهنای باند ۳۳٪ کمتر نسبت به نسخه قدیمی.

---

## ۱) پیش‌نیازها

| ابزار | توضیح |
|---|---|
| اکانت Vercel | برای دیپلوی Edge Function |
| `git` | دریافت پروژه |
| `npm` | نصب Vercel CLI |
| Go 1.21+ | **فقط برای build از سورس** — در غیر این صورت فایل باینری آماده را دانلود کنید |

نصب Node.js (برای npm): https://nodejs.org/en/download

---

## ۲) دریافت پروژه

```bash
git clone https://github.com/logicalangel/HttpOverVercel.git
cd HttpOverVercel
```

---

## ۳) ساخت کلاینت Go

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

---

## ۵) تنظیم متغیر محیطی `AUTH_KEY` در Vercel

در داشبورد Vercel:

- `Project → Settings → Environment Variables → Add`
- **Key:** `AUTH_KEY`
- **Value:** یک کلید امن دلخواه (این مقدار را در `config.json` هم وارد می‌کنید)

بعد از ذخیره، حتماً Redeploy کنید:

```bash
cd vercel && vercel --prod
```

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
  "auth_key": "SAME_VALUE_AS_VERCEL_AUTH_KEY",
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
curl -x http://127.0.0.1:8085 https://example.com
```

یا تست مستقیم endpoint روی Vercel:

```bash
curl -sS "https://YOUR_PROJECT.vercel.app/api/api" \
  -H "X-Auth-Key: YOUR_AUTH_KEY" \
  -H "X-Relay-Method: GET" \
  -H "X-Relay-URL: aHR0cHM6Ly9leGFtcGxlLmNvbQ==" \
  -H "X-Relay-Headers: e30="
```

---

## ۱۲) ساختار پروژه

```
go/
  cmd/proxy/        — نقطه ورود CLI
  internal/config/  — بارگذاری تنظیمات
  internal/mitm/    — مدیریت گواهی CA و TLS
  internal/relay/   — کلاینت relay (پروتکل باینری)
  internal/proxy/   — سرور پروکسی HTTP/HTTPS
vercel/
  api/api.js        — Vercel Edge Function
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
