# اسکرین‌شات در Wayland — تحقیق و برنامه

## مسئله

سشن شما **Wayland** است (پیش‌فرض GNOME در اوبونتو). در Wayland برخلاف X11، اپلیکیشن‌ها نمی‌توانند مستقیماً پیکسل‌های صفحه‌ی هم را بخوانند — اسکرین‌شات باید از طریق یک API امتیازدار (compositor یا portal) انجام شود. به همین دلیل لایه‌ی capture در این پروژه pluggable است.

## گزینه‌ها

### 1. XDG Desktop Portal — `backend: "portal"` (مسیر اصلی)

`org.freedesktop.portal.Screenshot` روی D-Bus سشن — روش استاندارد و مستقل از compositor.

- ✅ روی همه‌ی Wayland desktopها کار می‌کند (GNOME/KDE/…)
- ✅ خروجی PNG از طریق URI برمی‌گرداند
- ⚠️ **سؤال باز:** آیا دیالوگ اجازه در GNOME فقط بار اول نشان داده می‌شود و تصمیم persist می‌شود؟ (portalهای جدید دیالوگ با گزینه «همیشه اجازه بده» دارند — باید روی ۲۶.۰۴ راستی‌آزمایی شود)

تست دستی روی سیستم:

```bash
gdbus call --session \
  --dest org.freedesktop.portal.Desktop \
  --object-path /org/freedesktop/portal/desktop \
  --method org.freedesktop.portal.Screenshot.Screenshot \
  '' '{}'
```

### 2. GNOME Shell D-Bus API — `backend: "gnome-shell"` (fallback)

`org.gnome.Shell.Screenshot.Screenshot(includeCursor, flash, filename)` — همان APIای که `gnome-screenshot` استفاده می‌کند.

- ✅ دیالوگ ندارد، کاملاً بی‌صدا
- ⚠️ مخصوص GNOME؛ از نسخه‌ی ۴۱ به بعد بخش‌هایی از این API محدود شده — باید روی GNOME اوبونتو ۲۶.۰۴ تست شود

تست دستی:

```bash
gdbus call --session \
  --dest org.gnome.Shell.Screenshot \
  --object-path /org/gnome/Shell/Screenshot \
  --method org.gnome.Shell.Screenshot.Screenshot \
  true false '/tmp/payam-test.png'
```

### 3. X11 — `backend: "x11"` (fallback تضمینی)

اگر سشن «Ubuntu on Xorg» باشد، گرفتن اسکرین‌شات بدیهی است: `maim` / `scrot` یا XGetImage.

- ✅ بدون هیچ محدودیتی و بی‌صدا
- ⚠️ کاربر باید از صفحه‌ی login سشن Xorg را انتخاب کند ( Wayland پیش‌فرض است)

تست سشن فعلی:

```bash
echo $XDG_SESSION_TYPE   # wayland | x11
```

## ترتیب پیاده‌سازی پیشنهادی

1. **X11** (ساده‌ترین؛ برای سشن Xorg و آزمایش pipeline کامل ذخیره‌سازی)
2. **Portal** (مسیر اصلی Wayland) + راستی‌آزمایی رفتار permission dialog
3. **gnome-shell** (اگر portal برای captures دوره‌ای ساکت نشود، این عملاً تنها راه Wayland بی‌صداست)
4. `auto`: بر اساس `XDG_SESSION_TYPE` + probe موجودی portal روی D-Bus

## نکته‌ی حریم خصوصی

اسکرین‌شات دوره‌ای داده‌ی حساسی است:

- فایل‌ها با `0600` و دایرکتوری‌ها با `0700` نوشته می‌شوند
- هیچ چیزی از ماشین کاربر خارج نمی‌شود
- `retention_days` برای پاک‌سازی خودکار در نظر گرفته شده (فعلاً `0` = نگه‌داشتن همیشه)
