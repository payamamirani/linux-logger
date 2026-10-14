# معماری Payam Logger

## اجزا

```
┌───────────────────────────────────────────────┐
│ payam-loggerd (Go) — systemd user service     │
│                                               │
│  main ─► daemon ─► scheduler (ticker)         │
│                    │                          │
│                    ▼                          │
│                 capture.Backend (pluggable)   │
│                    │                          │
│                    ▼                          │
│                 storage.Store                 │
│                    │                          │
│         ┌─────────┴─────────┐                │
│         ▼                   ▼                │
│   screenshots/YYYY/MM/DD  index.db (SQLite)  │
└───────────────────────────────────────────────┘

┌───────────────────────────────────────────────┐
│ payam-logger-ui (GTK4) — فرانتند نمایش        │
│   خواندن مستقیم از index.db + فایل‌های تصویر  │
└───────────────────────────────────────────────┘
```

### چرا این جدا‌سازی؟

- **سرویس و فرانتند هیچ وابستگی ران‌تایمی به هم ندارند** — تنها نقطه‌ی مشترک، فرمت داده روی دیسک است (SQLite + layout فایل‌ها). فرانتند می‌تواند بسته باشد ولی ثبت فعالیت ادامه پیدا کند، و برعکس.
- **لایه‌ی capture عمداً pluggable است** چون روش گرفتن اسکرین‌شات به display server وابسته است (تفصیل در [capture.md](capture.md)).
- هر دو از **XDG base directories** پیروی می‌کنند:
  - داده: `~/.local/share/payam-logger`
  - تنظیمات: `~/.config/payam-logger/config.json`
  - یونیت systemd: `~/.config/systemd/user/payam-logger.service`

## جریان داده

1. `scheduler` هر `interval_sec` (پیش‌فرض ۶۰) یک‌بار `daemon.captureOnce` را صدا می‌زند.
2. `capture.Backend.Capture` تصویر را به‌صورت JPEG انکد شده برمی‌گرداند.
3. `storage.Store.SaveScreenshot`:
   - فایل را در `screenshots/YYYY/MM/DD/HHMMSS.jpg` با پرمیشن `0600` می‌نویسد (حریم خصوصی).
   - رکوردی در `index.db` درج می‌کند (متادیتا برای تایم‌لاین فرانتند).

## اسکیمای دیتابیس (برنامه‌ریزی‌شده)

```sql
CREATE TABLE screenshots (
  id           INTEGER PRIMARY KEY,
  taken_at     TEXT NOT NULL,   -- UTC, RFC3339
  path         TEXT NOT NULL,   -- نسبت به DataDir
  size_bytes   INTEGER NOT NULL,
  width        INTEGER,
  height       INTEGER,
  app          TEXT,            -- آینده: اپلیکیشن فعال
  window_title TEXT             -- آینده: عنوان پنجره فعال
);
CREATE INDEX idx_screenshots_taken_at ON screenshots(taken_at);
```

ستون‌های `app` و `window_title` برای قابلیت‌های ManicTime-مانند (ردیابی اپ فعال) رزرو شده‌اند.

## تصمیمات ثبت‌شده

| موضوع | تصمیم | دلیل |
|---|---|---|
| زبان سرویس | Go | باینری تکی، مصرف کم، مناسب دیمن |
| فرانتند | GTK4 native | حس اپ‌های اوبونتو، یکپارچگی با گنوم |
| اجرا | systemd **user** service | دسترسی به session D-Bus برای اسکرین‌شات؛ بدون نیاز به root |
| ایندکس | SQLite (`modernc.org/sqlite`) | pure-Go بدون cgo؛ کوئری زمانی برای تایم‌لاین |
| فرمت تصویر | JPEG q80 | حجم معقول (~300-500MB در روز با هر ۱ دقیقه) |
| تنظیمات | JSON (فعلاً) | بدون وابستگی؛ در صورت نیاز به TOML مهاجرت می‌کنیم |

## نقشه راه

- [x] ~~پیاده‌سازی بک‌اند capture~~ — فاز ۱: `extension` (اصلی، بی‌صدا) + `portal` (fallback) + انتخاب خودکار `auto`
- [ ] ایندکس SQLite + migration
- [ ] retention/cleanup خودکار (`retention_days`)
- [ ] فرانتند GTK4 (تایم‌لاین روزانه، پرش بین روزها)
- [ ] تشخیص قفل صفحه/suspend و توقف capture (اکستنشن قفل را خودش تشخیص می‌دهد؛ suspend سمت دیمن)
- [ ] جمع‌آوری اپ/پنجره فعال (اختیاری، مثل ManicTime)
- [ ] بسته‌بندی (`.deb` شامل دیمن + اکستنشن + systemd unit)
