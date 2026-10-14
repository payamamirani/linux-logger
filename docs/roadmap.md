# نقشه راه Payam Logger — Parity کامل با ManicTime

> این سند مرجع رسمی پروژه برای رسیدن به پوشش کامل قابلیت‌های ManicTime است.
> منبع تحقیق: مستندات رسمی ManicTime (Day view, Tracking, Tagging, Autotagging,
> Away window, Screenshots, Selecting time, Timesheet, Statistics, Shortcuts).

## وضعیت فعلی (پیش از فازهای M)

| قابلیت | وضعیت |
|---|---|
| کپچر بی‌صدا (اکستنشن GNOME Shell + fallback پورتال) | ✅ فاز ۰ |
| دیمن Go: زمان‌بند، قفل صفحه، retention، سرویس systemd | ✅ فاز ۱ و ۳ |
| ایندکس SQLite با migration | ✅ فاز ۲ |
| UI GTK4: تایم‌لاین ۲۴ ساعته، زوم ۱h–۲۴h (پیش‌فرض ۸–۱۸)، درگ-سلکشن با چسبندگی، hover→عکس، تنظیمات (interval/retention) | ✅ فاز ۴ |
| بسته‌بندی deb | ⏳ M8 |

---

## فهرست کامل قابلیت‌های ManicTime و معادل لینوکسی

| حوزه | قابلیت در ManicTime | راهکار ما |
|---|---|---|
| ثبت داده | Active/Away (idle) با کیبورد/ماوس | اکستنشن → Mutter IdleMonitor |
| ثبت داده | اپلیکیشن فعال + عنوان پنجره | اکستنشن → `global.display.focus_window` |
| ثبت داده | URL/سند (پلاگین per-app) | اکستنشن مرورگر (فاز M2.5 — اختیاری) |
| ثبت داده | دسکتاپ مجازی | اکستنشن → workspace index |
| ثبت داده | Git repo/branch از مسیر فایلها | فاز M7 (اختیاری) |
| ثبت داده | Off the record (توقف موقت ثبت) | M7 + indicator اکستنشن |
| ثبت داده | Tracking schedule (ساعات ثبت) | M7 |
| ثبت داده | لیست استثنا (اپ/سایت/عنوان) | M7 |
| ثبت داده | Alertها (استفاده بیش از حد) | M7 |
| Day View | چند تایم‌لاین: Tag/Autotag/Usage/Apps/Docs | M1 (داده) + M2 (UI) |
| Day View | Auto-zoom به بازه‌ی دارای داده | M2 |
| Day View | Snap انتخاب به دقیقه (۱/۲/۵/۱۰) | M2 |
| Day View | دابل‌کلیک = انتخاب کل بلاک | M2 |
| Day View | Ctrl+درگ = مولتی‌سلکت | M2 |
| Day View | پنل Details (چک‌باکس/فیلتر/ستون‌ها) + Summary (جمع گروه‌ها) | M2 |
| Day View | رنگ گروه‌ها + Folderها | M2 |
| Tagging | انتخاب→تگ، Tag picker درختی، Tag editor (rename/replace/رنگ) | M3 |
| Tagging | Billable ($)، Note، Shortcutهای تگ، Tag last untagged | M3 |
| Tagging | Stopwatch / Timer / Pomodoro | M3 |
| Tagging | Away window (تگ‌زدن بازگشت از idle + split + pause) | M3 |
| Autotag | قوانین app/url/title→تگ، Append/Prepend/Absorb | M4 |
| Autotag | Dynamic tags (`{{title}}`, regex)، copy-to-tags، diagnostics | M4 |
| عکس | tooltip preview، viewer داک/پنجره جدا، freeze + زوم/قبلی/بعدی/کپی/حذف | M5 |
| گزارش | Timesheet (group by سطوح، rounding، ذخیره، CSV) | M6 |
| گزارش | Statistics (Top/Day-duration/Attendance/Untagged/Custom) | M6 |
| گزارش | Advanced Search با زبان فیلتر و regex | M6 |
| سیستم | Backup خودکار، Mini window، Global shortcuts، Settings کامل | M7 |
| تیم | سرور sync، گزارش تیمی، attendance/vacation، API، MCP | M9 (اختیاری) |

**تطبیق‌های لینوکسی:** QuickBooks → خروجی CSV عمومی؛ tray ویندوز → indicator در پنل گنوم (اکستنشن)؛ ریموت-IP → خارج از دامنه.

---

## فازهای اجرا

هر فاز شippable است و بین فازها تایید کاربر گرفته می‌شود.

### فاز M1 — موتور داده‌ی فعالیت 🔥 (فاز جاری)

**هدف:** ثبت «پنجره فعال + عنوان + دسکتاپ» و «active/away» به‌صورت رویدادی.

**طراحی:**

1. **اکستنشن v2** — اینترفیس D-Bus `org.payam.Logger` گسترش می‌یابد:
   - سیگنال `FocusChanged(s app, s title, i workspace)` — روی تغییر فوکوس و تغییر title پنجره‌ی متمرک (dedup/throttle داخلی)
   - سیگنال `IdleChanged(b idle)` — عبور از آستانه‌ی idle (پیش‌فرض ۵ دقیقه، `add_user_active_watch` برای بازگشت)
   - متد `GetStatus() → (s app, s title, i workspace, b idle)` — وضعیت لحظه‌ای برای استارت دیمن
   - متد `Capture(s) → (bs)` — بدون تغییر
2. **دیمن** — کلکتور جدید (`internal/activity`):
   - استارت: `GetStatus()` → باز کردن سگمنت‌های اولیه (اگر اکستنشن نبود، فقط هشدار؛ کپچر ادامه می‌یابد)
   - subscribe روی سیگنال‌ها؛ سگمنت فوکوس در حافظه، روی تغییر → ثبت ردیف؛ idle سگمنت‌ها را gate می‌کند
3. **DB v2** — migration پله‌ای موجود:

```sql
CREATE TABLE app_usage (
  id INTEGER PRIMARY KEY,
  start TEXT NOT NULL, end TEXT NOT NULL, day TEXT NOT NULL,
  app TEXT NOT NULL, title TEXT NOT NULL, workspace INTEGER
);
CREATE INDEX idx_app_usage_day ON app_usage(day);

CREATE TABLE computer_usage (
  id INTEGER PRIMARY KEY,
  start TEXT NOT NULL, end TEXT NOT NULL, day TEXT NOT NULL,
  state TEXT NOT NULL  -- 'active' | 'away'
);
CREATE INDEX idx_computer_usage_day ON computer_usage(day);

CREATE TABLE tags (      -- از M3 استفاده می‌شود؛ از الان ساخته می‌شود
  id INTEGER PRIMARY KEY,
  start TEXT NOT NULL, end TEXT NOT NULL, day TEXT NOT NULL,
  name TEXT NOT NULL,    -- سلسله‌مراتب با کاما
  note TEXT,
  billable INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_tags_day ON tags(day);
```

**تست‌ها:** اسکریپت تست سیگنال‌ها (`scripts/extension-activity-test.py`)، تست‌های واحد storage برای migration و درج سگمنت‌ها، اجرای زنده بعد از یک logout/login.

### فاز M2 — Day View کامل
چند تایم‌لاین (Tag/Autotag/Computer usage/Applications + نشانگر عکس‌ها)، auto-zoom، snap دقیقه (کانفیگ)، دابل‌کلیک بلاک، مولتی‌سلکت، پنل Details/Summary با فیلتر، رنگ‌ها + Folder، میانبرهای کامل کیبورد.

### فاز M2.5 — URL tracking مرورگر (اختیاری)
اکستنشن Firefox/Chrome با native messaging → جدول `doc_usage`.

### فاز M3 — Tagging کامل
Add tag از انتخاب، Tag picker درختی، Tag editor (ترکیب‌ها/تگ‌ها/rename/replace/رنگ با منطق skip)، Billable + Note + shortcutها، Tag last untagged، Stopwatch/Timer/Pomodoro (با autostop روی idle و reminder)، Away window (split چندتکه، چند away جاافتاده، pause اعلان‌ها)، نمای Untagged.

### فاز M4 — Autotagging کامل
قوانین (اکستنشن راست‌کلیک روی فعالیت → Add to autotag)، editor قوانین با تست/دiagnostics، انواع Append/Prepend/Absorb، «اولین تطابق بس است»، auto-fill خلأهای ریز + highlight حدس‌ها، Dynamic tags با `{{title}}/{{group}}` و regex، copy autotags→tags با ۳ حالت تداخل، import/export قوانین.

### فاز M5 — Viewer اسکرین‌شات حرفه‌ای
tooltip روی تایم‌لاین، viewer داک‌شده (چپ/وسط/راست) یا پنجره‌ی جدا، Freeze با toolbar (زوم/پن/1:1/قبلی/بعدی با ←→/Save as/Copy/Open/Delete)، انتخاب بازه از روی عکس‌ها (Shift+←→)، حذف batch عکس‌ها، استثنای کپچر per-app.

### فاز M6 — جستجو و گزارش
Advanced search با زبان فیلتر (`app:`, `title:`, `tag:`, regex با `#"..."`)، Timesheet (group by سطوح تگ/روز/ماه، Total by، Include notes، Rounding با ۴ حالت + never-round-to-zero، ذخیره‌ی گزارش‌ها، خروجی CSV/کلیپ‌بورد)، Statistics (Top N / Day-duration / Attendance / Untagged / Custom با group by روز/هفته/ماه/سال، line/bar، میانگین، خروجی تصویر + CSV).

### فاز M7 — سیستم و حریم خصوصی
Off the record (تا فردا/۱ ساعت/۱۵ دقیقه/تا اطلاع) + indicator پنل در اکستنشن، Tracking schedule، لیست‌های استثنای ثبت، Alertها، Backup خودکار + restore، Mini window، Global shortcuts، Settings کامل، Git tracking (اختیاری).

### فاز M8 — بسته‌بندی `.deb`
دیمن + UI + اکستنشن + systemd unit + desktop entry با وابستگی‌های runtime خودکار (apt resolves). اسکریپت setup برای فعال‌سازی اکستنشن/سرویس.

### فاز M9 — سرور (اختیاری، آخر)
sync چند دستگاه (tags/autotags/عکس‌ها)، پنل گزارش تیمی، attendance/vacation، REST API، MCP server برای AI.

---

## اصول فنی ثابت

- داده فقط محلی؛ فایل‌ها `0600` و دایرکتوری‌ها `0700`
- دیمن تنها نویسنده‌ی DB است؛ UI فقط می‌خواند (WAL)
- اکستنشن منبع همه‌ی تعاملات با gnome-shell است (کپچر، فوکوس، idle، indicator آینده)
- هر تغییر اسکیمای DB از طریق migration پله‌ای `PRAGMA user_version`
- تست واحد برای منطق خالص Go؛ تست زنده برای مسیرهای D-Bus/اکستنشن
- به‌روزرسانی کد اکستنشن نیازمند یک logout/login است (محدودیت gnome-shell)
