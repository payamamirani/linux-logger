# Payam Logger

ابزار ردیابی فعالیت دسکتاپ برای اوبونتو (شبیه ManicTime) — یک سرویس بک‌گراند که هر ۱ دقیقه از صفحه اسکرین‌شات می‌گیرد و به‌صورت محلی ذخیره می‌کند، به‌همراه یک رابط وب برای مشاهده تایم‌لاین (در حال بازطراحی).

## معماری

```
┌─────────────────────────┐        ┌──────────────────────────┐
│  payam-loggerd (Go)     │        │  payam-logger-ui (Go +   │
│  systemd user service   │        │  GTK4) — فرانتند نمایش    │
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
- **فرانتند (وب):** رابط وب که از خود دیمن سرو می‌شود — مستقیماً از SQLite و فایل‌ها.

## ساختار پروژه

```
payam-logger/
├── cmd/
│   └── payam-loggerd/       # سرویس بک‌گراند (دیمن + وب‌سرور UI)
├── internal/
│   ├── capture/             # بک‌اندهای اسکرین‌شات (extension / portal / x11)
│   ├── scheduler/           # حلقه‌ی زمان‌بندی (ticker)
│   ├── storage/             # ذخیره‌سازی فایل + ایندکس SQLite
│   ├── config/              # خواندن تنظیمات (JSON)
│   ├── daemon/              # سیم‌کشی اجزای دیمن
│   └── web/                 # وب‌سرور + API + فایل‌های UI
├── extension/               # اکستنشن GNOME Shell (capture بی‌صدا)
│   └── payam-logger@payamam/
├── systemd/                 # یونیت user service
├── config/                  # نمونه فایل تنظیمات
├── scripts/                 # نصب و تست
└── docs/                    # معماری و یادداشت‌های فنی
```

## شروع سریع

```bash
# بیلد و اجرای آزمایشی دیمن در foreground
make run

# یا با فایل تنظیمات
bin/payam-loggerd -config config/payam-logger.example.json

# نصب کامل دیمن به‌عنوان سرویس واقعی (باینری + systemd + اکستنشن + مجوزها)
make install-user
```

### کنترل سرویس بعد از نصب

```bash
journalctl --user -u payam-logger -f        # لاگ زنده
systemctl --user status payam-logger        # وضعیت
systemctl --user stop payam-logger          # توقف
systemctl --user start payam-logger         # شروع
systemctl --user disable payam-logger       # غیرفعال‌کردن استارت خودکار
```

## پیش‌نیازها

- Go ≥ 1.24
- اوبونتو با GNOME (تست‌شده روی 26.04، Wayland و X11)

## مستندات

- [docs/roadmap.md](docs/roadmap.md) — نقشه راه کامل parity با ManicTime (فازهای M1–M9)
- [docs/architecture.md](docs/architecture.md) — معماری و نقشه راه
- [docs/capture.md](docs/capture.md) — چالش اسکرین‌شات در Wayland و گزینه‌ها
