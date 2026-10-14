# Payam Logger

ابزار ردیابی فعالیت دسکتاپ برای اوبونتو (شبیه ManicTime) — یک سرویس بک‌گراند که هر ۱ دقیقه از صفحه اسکرین‌شات می‌گیرد و به‌صورت محلی ذخیره می‌کند، به‌همراه یک فرانتند GTK4 برای مشاهده تایم‌لاین.

> **وضعیت:** اسکلت پروژه — سرویس و فرانتند به‌تدریج پیاده‌سازی می‌شوند.

## معماری

```
┌─────────────────────────┐        ┌──────────────────────────┐
│  payam-loggerd (Go)     │        │  payam-logger-ui (GTK4)  │
│  systemd user service   │        │  فرانتند نمایش           │
│                         │        │                          │
│  scheduler ─► capture ──┼──►disk─┼── reads ─► SQLite index  │
│  (هر ۶۰ ثانیه)  backends│  JPEG  │        + فایل‌های تصویر   │
└─────────────────────────┘        └──────────────────────────┘
          │                                    │
          ▼                                    ▼
  ~/.local/share/payam-logger/        ~/.local/share/payam-logger/
    screenshots/YYYY/MM/DD/*.jpg        index.db (SQLite)
```

- **بک‌اند (Go):** دیمنی که با interval مشخص اسکرین‌شات می‌گیرد و روی دیسک + ایندکس SQLite ذخیره می‌کند. لایه‌ی capture به‌صورت pluggable است چون در Wayland (سشن پیش‌فرض اوبونتو) روش گرفتن اسکرین‌شات با X11 فرق دارد.
- **فرانتند (GTK4):** اپلیکیشن native گنوم برای مرور تایم‌لاین اسکرین‌شات‌ها — مستقیماً از SQLite و فایل‌ها می‌خواند.

## ساختار پروژه

```
payam-logger/
├── backend/                 # سرویس بک‌گراند (Go)
│   ├── cmd/payam-loggerd/   # نقطه ورود دیمن
│   └── internal/
│       ├── capture/         # بک‌اندهای اسکرین‌شات (portal / gnome-shell / x11)
│       ├── scheduler/       # حلقه‌ی زمان‌بندی (ticker)
│       ├── storage/         # ذخیره‌سازی فایل + ایندکس SQLite
│       ├── config/          # خواندن تنظیمات (JSON)
│       └── daemon/          # سیم‌کشی اجزا
├── frontend/                # فرانتند GTK4 (placeholder — بعداً)
├── extension/               # اکستنشن GNOME Shell (capture بی‌صدا)
│   └── payam-logger@payamam/
├── systemd/                 # یونیت user service
├── config/                  # نمونه فایل تنظیمات
├── scripts/                 # نصب و اجرای dev
└── docs/                    # معماری و یادداشت‌های فنی
```

## شروع سریع

```bash
# بیلد و اجرای آزمایشی در foreground
make run

# یا با فایل تنظیمات
backend/bin/payam-loggerd -config config/payam-logger.example.json

# نصب به‌عنوان systemd user service
make install-user
journalctl --user -u payam-logger -f   # دیدن لاگ‌ها
```

## پیش‌نیازها

- Go ≥ 1.24
- اوبونتو با GNOME (تست‌شده روی 26.04، Wayland و X11)
- برای فرانتند (بعداً): GTK4 development libraries

## مستندات

- [docs/architecture.md](docs/architecture.md) — معماری و نقشه راه
- [docs/capture.md](docs/capture.md) — چالش اسکرین‌شات در Wayland و گزینه‌ها
