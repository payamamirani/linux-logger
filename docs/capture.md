# اسکرین‌شات در Wayland — تحقیق و برنامه

## ✅ نتایج فاز ۰ (تست‌شده روی این سیستم — Ubuntu 26.04 / GNOME Shell 50.1 / Wayland)

| تست | نتیجه |
|---|---|
| `org.gnome.Shell.Screenshot` D-Bus | ❌ **بلاک شده** — `AccessDenied: Screenshot is not allowed` (GNOME ۵۰ دیگر به فراخوان‌های خارجی اجازه نمی‌دهد) |
| Portal بدون مجوز ذخیره‌شده | ❌ کد ۲؛ دیالوگ هم باز نمی‌شود چون گنوم فقط به اپِ **متمرک** اجازه‌ی نمایش دیالوگ اجازه می‌دهد |
| Portal با `interactive=true` از فرایند بی‌پنجره | ❌ همان مشکل focus — `InteractiveScreenshot didn't return a file` |
| **نوشتن مجوز مستقیم در permission store** | ✅ `SetPermission("screenshot", true, "screenshot", "", ["yes"])` بدون خطا انجام شد |
| **Portal بعد از مجوز** | ✅ **کاملاً بی‌صدا موفق** — حتی از فرایند بک‌گراند بدون پنجره؛ ~۰.۶ ثانیه برای هر capture |

### دستور grant (باید در نصاب اجرا شود)

```bash
gdbus call --session \
  --dest org.freedesktop.impl.portal.PermissionStore \
  --object-path /org/freedesktop/impl/portal/PermissionStore \
  --method org.freedesktop.impl.portal.PermissionStore.SetPermission \
  screenshot true screenshot '' "['yes']"
```

> `''` یعنی app_id خالی — برای فراخوان‌هایی که app-id قابل تشخیص ندارند (دیمن ما همین‌طور است).
> اگر در فاز ۱ معلوم شد دیمن Go با app-id دیگری صدا می‌زند، همان id هم باید grant شود.

### نکات طراحی که از تست درآمد

1. **پورتال PNG را در `~/Pictures/Screenshot-N.png` می‌نویسد با شماره‌ی افزایشی** — دیمن باید هر فایل را بخواند، به JPEG بازانکود کند و PNG موقت را حذف کند (وگرنه روزی ۱۴۴۰ فایل در Pictures انبار می‌شود!)
2. هر capture حدود ۰.۶ ثانیه طول می‌کشد — برای interval ۶۰ ثانیه کاملاً کافی است
3. خروجی PNG است (RGBA, 1920×1080) — بازانکود JPEG با stdlib انجام می‌شود
4. مجوز در permission store باقی می‌ماند؛ فقط یک بار در نصب grant می‌شود

### تصمیم فاز ۰

- **بک‌اند اصلی: `extension`** ✅ — اکستنشن GNOME Shell با `Shell.Screenshot` داخلی (بی‌صدا: بدون فلاش، بدون صدا، بدون دیالوگ)
- بک‌اند `portal`: fallback (کار می‌کند ولی **فلاش و صدای شاتر دارد** — هاردکد در xdg-desktop-portal-gnome)
- بک‌اند `gnome-shell`: مرده روی GNOME ۵۰+
- بک‌اند `x11`: fallback برای سشن‌های Xorg

## ریشه‌ی فلاش و صدای شاتر (کشف فاز ۰)

مسیر پورتال چنین است:

```
ما → org.freedesktop.portal.Screenshot
   → xdg-desktop-portal-gnome (screenshotdialog.c)
   → org.gnome.Shell.Screenshot D-Bus با flash=TRUE هاردکد!
   → js/ui/screenshot.js: _flashAsync() → Flashspot + play_from_theme('screen-capture')
```

- در `screenshotdialog.c` متد `do_take_screenshot` مقدار flash را **TRUE هاردکد** کرده — هیچ option پورتالی برای غیرفعال‌کردنش وجود ندارد.
- در `js/ui/screenshot.js` متد `_flashAsync` فلاش **و** صدای شاتر را با هم پخش می‌کند.
- ولی خودِ `Shell.Screenshot` (شیء C داخلی) نه فلاش دارد نه صدا — فقط صفحه را paint می‌کند و PNG در stream می‌نویسد (حتی پارامتر flash هم ندارد).

### راه‌حل: اکستنشن `payam-logger@payamam`

اکستنشن درون فرایند gnome-shell اجرا می‌شود و می‌تواند `new Shell.Screenshot()` را مستقیم صدا بزند — دقیقاً مثل کد خود گنوم — و نتیجه را روی D-Bus (`org.payam.Logger.Capture(filepath)`) در اختیار دیمن بگذارد.

ایمنی اکستنشن:

- فقط اجازه‌ی نوشتن داخل `~/.local/share/payam-logger/` را دارد (با canonicalize مسیر)
- اگر administrator ذخیره روی دیسک را قفل کرده باشد (`org.gnome.desktop.lockdown`) خودداری می‌کند
- وقتی session قفل است عکس نمی‌گیرد
- فراخوانی‌ها سریالایز می‌شوند (Shell.Screenshot همزمان فقط یک عملیات می‌پذیرد)

نصب: `make install-extension` — بار اول نیاز به یک logout/login دارد (گنوم‌شل اکستنشن‌ها را فقط موقع استارت اسکن می‌کند)؛ بعدش `scripts/extension-capture-test.sh` برای تست.

> مجوز permission store ای که در فاز ۰ گرفتیم برای بک‌اند `portal` (fallback) همان‌جا می‌ماند.

## مسئله

سشن **Wayland** است (پیش‌فرض GNOME در اوبونتو). در Wayland برخلاف X11، اپلیکیشن‌ها نمی‌توانند مستقیماً پیکسل‌های صفحه‌ی هم را بخوانند — اسکرین‌شات باید از طریق یک API امتیازدار (compositor یا portal) انجام شود. به همین دلیل لایه‌ی capture در این پروژه pluggable است.

## گزینه‌ها

### 1. XDG Desktop Portal — `backend: "portal"` (مسیر اصلی — ✅ تأییدشده در فاز ۰)

`org.freedesktop.portal.Screenshot` روی D-Bus سشن — روش استاندارد و مستقل از compositor.

- ✅ روی همه‌ی Wayland desktopها کار می‌کند (GNOME/KDE/…)
- ✅ خروجی PNG از طریق URI برمی‌گرداند
- ✅ **آزموده شد:** بعد از grant یک‌باره‌ی permission store، کاملاً بی‌صدا حتی از فرایند بک‌گراند کار می‌کند

تست دستی روی سیستم:

```bash
gdbus call --session \
  --dest org.freedesktop.portal.Desktop \
  --object-path /org/freedesktop/portal/desktop \
  --method org.freedesktop.portal.Screenshot.Screenshot \
  '' '{}'
```

### 2. GNOME Shell D-Bus API — `backend: "gnome-shell"` (❌ مرده روی GNOME ۵۰+)

`org.gnome.Shell.Screenshot.Screenshot(includeCursor, flash, filename)` — همان APIای که `gnome-screenshot` استفاده می‌کند.

- ❌ **روی GNOME 50.1 تست شد و AccessDenied برگرداند** — این API دیگر به فراخوان‌های خارجی مجاز نیست
- فقط اگر روزی روی نسخه‌ی قدیمی‌تر GNOME نیاز شد ارزش دارد؛ عملاً از نقشه حذف می‌شود

### 3. X11 — `backend: "x11"` (fallback تضمینی)

اگر سشن «Ubuntu on Xorg» باشد، گرفتن اسکرین‌شات بدیهی است: `maim` / `scrot` یا XGetImage.

- ✅ بدون هیچ محدودیتی و بی‌صدا
- ⚠️ کاربر باید از صفحه‌ی login سشن Xorg را انتخاب کند ( Wayland پیش‌فرض است)

تست سشن فعلی:

```bash
echo $XDG_SESSION_TYPE   # wayland | x11
```

## ترتیب پیاده‌سازی (به‌روزشده بعد از فاز ۰)

1. **Portal** (مسیر اصلی — تأییدشده) پیاده‌سازی می‌شود
2. **X11** به‌عنوان fallback برای سشن‌های Xorg
3. `gnome-shell` حذف می‌شود (روی GNOME ۵۰+ مرده است)
4. `auto`: بر اساس `XDG_SESSION_TYPE` + probe موجودی portal روی D-Bus

## نکته‌ی حریم خصوصی

اسکرین‌شات دوره‌ای داده‌ی حساسی است:

- فایل‌ها با `0600` و دایرکتوری‌ها با `0700` نوشته می‌شوند
- هیچ چیزی از ماشین کاربر خارج نمی‌شود
- `retention_days` برای پاک‌سازی خودکار در نظر گرفته شده (فعلاً `0` = نگه‌داشتن همیشه)
