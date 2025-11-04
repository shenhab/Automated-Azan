# Daylight Saving Time (DST) Issue Investigation Report
## Athan Prayer Times Application - Ireland/Dublin Timezone

**Date of Investigation:** November 4, 2025
**Repository:** /root/dev/athan
**Focus:** DST handling in Docker container with Europe/Dublin timezone

---

## Executive Summary

The investigation has identified a **critical timezone configuration mismatch** that causes incorrect prayer time scheduling after DST transitions in Ireland. The main issue is that **the Dockerfile hardcodes `TZ=UTC` while the application code expects `Europe/Dublin`**, creating a disconnect between system time and application timezone handling.

---

## Critical Issues Found

### 1. **CRITICAL: Hardcoded UTC in Dockerfile (Line 89)**

**File:** `/root/dev/athan/Dockerfile`
**Line:** 89
**Code:**
```dockerfile
ENV TZ=UTC
```

**Impact:**
- Docker container runs in UTC regardless of intended timezone
- Prayer times are scheduled relative to UTC time, not Dublin time
- After DST transitions, the mismatch becomes 1 hour off
- This is the **root cause** of the reported issue

**Expected Behavior:**
- Should respect the `TZ` environment variable from docker-compose or allow override
- Should default to Dublin time if it's specifically for Ireland deployment

---

### 2. **Hardcoded Dublin Timezone in Application Code**

**File:** `/root/dev/athan/athan_scheduler.py`
**Line:** 28
**Code:**
```python
self.tz = tz.gettz('Europe/Dublin')
```

**File:** `/root/dev/athan/prayer_times_config.py`
**Line:** 36
**Code:**
```python
timezone_str: str = "Europe/Dublin"
```

**Impact:**
- Application assumes Dublin timezone hardcoded
- If Docker container is set to UTC, there's a timezone mismatch:
  - System clock reads UTC time
  - Application interprets times as Dublin time
  - Results in 1-hour offset during standard time, 0-hour offset during DST (causing incorrect transitions)

**Timeline Example (Post-DST Transition Oct 27, 2024):**
- System UTC time: 14:00 (2:00 PM UTC)
- Application reads: 14:00 but interprets as Dublin time (2:00 PM)
- Actual Dublin time: 14:00 BST (British Summer Time, UTC+1 ended)
- Expected: 15:00 IST (Irish Standard Time, UTC+1)
- **Result:** Prayers scheduled 1 hour early!

---

### 3. **Schedule Library Uses System Timezone for next_run() Calculations**

**File:** `/root/dev/athan/athan_scheduler.py`
**Lines:** 163, 170, 418
**Code:**
```python
schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn)
schedule.every().day.at(formatted_time).do(chromecast_manager.start_adahn_alfajr)
schedule.every().day.at(pre_fajr_time).do(self.play_pre_fajr_quran)
```

**Issue:**
- `schedule.every().day.at(time_string)` uses the **system's local timezone** to calculate next_run()
- With UTC TZ in Dockerfile but Dublin tz in code, the library:
  - Uses UTC system time for scheduling calculations
  - But the application formats times as Dublin times
  - Creates ambiguity in when jobs actually execute

**Line:** 305-312 (Scheduler Status)
```python
next_run = schedule.next_run()
# job.next_run returns datetime in system timezone (UTC)
# but application treats times as Dublin time
```

**Impact:** Jobs execute at UTC times instead of Dublin times

---

### 4. **DST Transition Not Handled**

**File:** `/root/dev/athan/athan_scheduler.py`
**Lines:** 539-543, 576-579 (Daily refresh logic)
```python
current_date = datetime.now(self.tz).date()
if last_update_date != current_date:
    logging.info(f"Updating prayer times for new day: {current_date}")
    # Refresh happens at 1:00 AM Dublin time
```

**Issue:**
- While the application does refresh daily, it happens at fixed wall-clock times
- During DST transitions, wall-clock times are ambiguous
- Example: October 27, 2024 (1:00 AM occurs twice when clocks go back):
  - First 1:00 AM (BST, UTC+1)
  - Clocks go back to 0:00 AM (IST, UTC+0)
  - Second 1:00 AM (IST, UTC+0)
  - Code doesn't handle which instance is meant

---

### 5. **Docker Compose Configuration Issue**

**File:** `/root/dev/athan/docker-compose.yml`
**Line:** 18
**Code:**
```yaml
environment:
  - TZ=UTC  # Change to your timezone (e.g., Europe/Dublin, America/New_York)
```

**Issue:**
- Comment says "Change to your timezone" but default is UTC
- Users may miss this, or changes to docker-compose.yml aren't applied if using Dockerfile ENV
- Multiple Dockerfile variants all hardcode UTC (Dockerfile, Dockerfile.minimal, Dockerfile.optimized)

**Files affected:**
- `/root/dev/athan/Dockerfile` (Line 89): `ENV TZ=UTC`
- `/root/dev/athan/Dockerfile.minimal` (Line 69): `ENV TZ=UTC`
- `/root/dev/athan/Dockerfile.optimized` (Line 84): `ENV TZ=UTC`

---

### 6. **Portainer Stack Configuration Has Correct Timezone (But Overridden)**

**File:** `/root/dev/athan/portainer-stack.yml`
**Line:** 14
**Code:**
```yaml
environment:
  - TZ=Europe/Dublin
```

**Issue:**
- Portainer stack has correct timezone, BUT
- The Dockerfile's ENV TZ=UTC still applies unless explicitly overridden in docker-compose
- Layer order: Dockerfile ENV is applied first, docker-compose environment variables override it
- **But** if users build with Dockerfile directly (not compose), the UTC setting persists

---

## Code Analysis

### Timezone Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│ System Time (Docker)                                        │
│ TZ=UTC (Dockerfile hardcode) ──> OS reports UTC time        │
└────────────┬────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────┐
│ Application Code                                            │
│ datetime.now(tz.gettz('Europe/Dublin')) ──> Interprets     │
│ system UTC time AS Dublin time                             │
└────────────┬────────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────────┐
│ Schedule Library                                            │
│ schedule.every().day.at(time).do(job)                      │
│ Uses system timezone (UTC) for next_run calculations       │
└─────────────────────────────────────────────────────────────┘
```

### Mismatched Components

| Component | Timezone | Impact |
|-----------|----------|--------|
| System (OS) | UTC | Returns UTC times |
| Prayer times config | Europe/Dublin | Formats times as Dublin |
| Schedule library | System TZ (UTC) | Calculates jobs in UTC |
| Prayer times data | UTC timestamps | Interpreted as Dublin |

---

## Specific DST Problem Scenario

**Date: October 27, 2024 (DST Transition in Ireland)**

Before transition: UTC+1 (IST)
After transition: UTC+0 (GMT)

**Current Setup Problem:**

| Time | Actual Dublin Time | System UTC (Container) | App Reads UTC As | Error |
|------|-------------------|----------------------|------------------|-------|
| 13:00 UTC | 14:00 IST | 13:00 | 13:00 (DST mismatch) | +1 hour off |
| After midnight | Next prayer | Still on old TZ logic | Misaligned | Variable |

**Prayer Time Example:**

Fajr prayer scheduled for Dublin: 06:15
- Correct: System should wake at 06:15 Dublin time
- With UTC TZ: System reads 06:15 as UTC, but actually 05:15 UTC = 06:15 Dublin
- Actual execution: Runs at 06:15 UTC = 07:15 Dublin time = **1 hour LATE**

---

## Files with Timezone Configuration

### Primary Timezone Configuration Files

| File | Line | Setting | Type | Issue |
|------|------|---------|------|-------|
| Dockerfile | 89 | `ENV TZ=UTC` | Hardcoded | Critical - Contradicts app logic |
| Dockerfile.minimal | 69 | `ENV TZ=UTC` | Hardcoded | Critical - Same issue |
| Dockerfile.optimized | 84 | `ENV TZ=UTC` | Hardcoded | Critical - Same issue |
| docker-compose.yml | 18 | `TZ=UTC` | Default | User must change manually |
| portainer-stack.yml | 14 | `TZ=Europe/Dublin` | Correct | Conflicts with Dockerfile |
| athan_scheduler.py | 28 | `self.tz = tz.gettz('Europe/Dublin')` | Hardcoded | Conflicts with System |
| prayer_times_config.py | 36 | `timezone_str: str = "Europe/Dublin"` | Hardcoded | Correct but not configurable |

### Prayer Time Scheduling Code

| File | Lines | Function | Issue |
|------|-------|----------|-------|
| athan_scheduler.py | 155-175 | `schedule_prayers()` | Uses system TZ for schedule.every().day.at() |
| athan_scheduler.py | 539-543 | `run_scheduler()` | Refresh at fixed wall-clock time (ambiguous during DST) |
| athan_scheduler.py | 593-628 | `sleep_until_next_1am()` | Assumes next 1:00 AM exists (fails on spring forward) |

---

## Testing Evidence

**Relevant Test File:** `/root/dev/athan/tests/test_athan_scheduler.py`

The tests mock datetime and don't actually test DST transitions:
- Line 125: `mock_dt.now.return_value = datetime(2023, 6, 15, 10, 0, 0)` (No DST logic)
- Tests use fixed dates without testing transition dates

---

## Recommended Fixes

### Fix 1: Remove UTC Hardcode from All Dockerfiles (CRITICAL)

**Files to change:**
- `/root/dev/athan/Dockerfile` - Line 89
- `/root/dev/athan/Dockerfile.minimal` - Line 69
- `/root/dev/athan/Dockerfile.optimized` - Line 84

**Change from:**
```dockerfile
ENV TZ=UTC
```

**Change to:**
```dockerfile
ENV TZ=Europe/Dublin
```

**Or alternatively (more flexible):**
```dockerfile
ENV TZ=${TZ:-Europe/Dublin}
```

---

### Fix 2: Make Timezone Configurable via Environment Variable

**File:** `/root/dev/athan/athan_scheduler.py`

**Change from (Line 28):**
```python
self.tz = tz.gettz('Europe/Dublin')
```

**Change to:**
```python
timezone_str = os.environ.get('AZAN_TIMEZONE', 'Europe/Dublin')
self.tz = tz.gettz(timezone_str)
```

**File:** `/root/dev/athan/prayer_times_config.py`

**Change from (Line 36):**
```python
timezone_str: str = "Europe/Dublin"
```

**Change to:**
```python
timezone_str: str = os.environ.get('AZAN_TIMEZONE', 'Europe/Dublin')
```

---

### Fix 3: Handle DST Transitions in Scheduler Loop

**File:** `/root/dev/athan/athan_scheduler.py`
**Function:** `sleep_until_next_1am()` (Lines 593-628)

Add DST-aware logic:

```python
def sleep_until_next_1am(self):
    """
    Sleep until 1:00 AM the next day, handling DST transitions.
    
    Returns:
        dict: JSON response with sleep information
    """
    try:
        now = datetime.now(self.tz)
        next_1am = now.replace(hour=1, minute=0, second=0, microsecond=0)
        
        if now >= next_1am:
            # Add a day, but use timedelta to handle DST properly
            next_1am = now + timedelta(days=1)
            next_1am = next_1am.replace(hour=1, minute=0, second=0, microsecond=0)
            
            # Normalize timezone after arithmetic to handle DST transitions
            # The timezone offset might change
            next_1am = next_1am.astimezone(self.tz)
        
        sleep_duration = (next_1am - now).total_seconds()
        # ... rest of implementation
```

---

### Fix 4: Update docker-compose.yml Default

**File:** `/root/dev/athan/docker-compose.yml`

**Change from (Line 18):**
```yaml
- TZ=UTC  # Change to your timezone (e.g., Europe/Dublin, America/New_York)
```

**Change to:**
```yaml
- TZ=Europe/Dublin  # Change to your timezone (e.g., UTC, America/New_York)
```

---

### Fix 5: Add DST Testing

**File:** New test file or `/root/dev/athan/tests/test_athan_scheduler.py`

Add tests for DST transitions:

```python
def test_scheduler_handles_dst_transition():
    """Test that prayer times are correct before and after DST transition"""
    # Ireland DST transition: October 27, 2024 (1:00 AM -> 0:00 AM)
    # Test prayer times scheduled for Oct 26 and Oct 28
    pass

def test_sleep_until_1am_during_dst_transition():
    """Test sleep_until_next_1am handles spring-forward DST"""
    # March 31, 2024: 1:00 AM is skipped (clocks go forward to 2:00 AM)
    pass
```

---

## Summary of Issues by Severity

| Severity | Issue | Location | Fix Priority |
|----------|-------|----------|--------------|
| CRITICAL | Dockerfile hardcodes UTC | Dockerfile:89 | P0 - Fix immediately |
| CRITICAL | Dockerfile.minimal hardcodes UTC | Dockerfile.minimal:69 | P0 - Fix immediately |
| CRITICAL | Dockerfile.optimized hardcodes UTC | Dockerfile.optimized:84 | P0 - Fix immediately |
| HIGH | App timezone hardcoded | athan_scheduler.py:28 | P1 - Make configurable |
| HIGH | Docker-compose defaults to UTC | docker-compose.yml:18 | P1 - Change default |
| MEDIUM | DST transition handling | athan_scheduler.py:593-628 | P2 - Add proper DST logic |
| MEDIUM | Schedule uses system TZ | athan_scheduler.py:163-170 | P2 - Verify system TZ matches |
| LOW | Missing DST tests | tests/ | P3 - Add comprehensive tests |

---

## Why This Causes the Reported Issue

**User Report:** "Docker is not casting the athan at correct times after the recent time change"

**Root Cause Chain:**

1. Docker container system timezone = UTC (from Dockerfile ENV)
2. Application expects Dublin timezone hardcoded in code
3. After October 27, 2024 DST transition:
   - System recognizes UTC-0 (GMT, no offset)
   - App still expects UTC+1 (IST, +1 hour offset)
   - Mismatch creates 1-hour discrepancy
4. Prayer times scheduled for Dublin time are executed at UTC time
5. User experiences prayers 1 hour early/late depending on time of day

**Example:**
- Maghrib prayer scheduled: 16:30 Dublin time
- System time (UTC): 16:30
- Application interprets: "16:30" and schedules for 16:30 UTC
- Actual time: 17:30 Dublin time
- Prayer plays 1 hour late!

---

## Verification Steps

After applying fixes:

1. Build Docker image with corrected Dockerfile
2. Check container environment: `docker exec athan env | grep TZ`
3. Verify system time: `docker exec athan date`
4. Verify application timezone:
   ```bash
   docker exec athan curl http://localhost:5000/api/status | jq '.prayer_times.timezone'
   ```
5. Check next prayer times are correct for current date
6. Wait for actual DST transition to verify correct behavior

---

## Files to Review

- /root/dev/athan/Dockerfile
- /root/dev/athan/Dockerfile.minimal
- /root/dev/athan/Dockerfile.optimized
- /root/dev/athan/docker-compose.yml
- /root/dev/athan/portainer-stack.yml
- /root/dev/athan/athan_scheduler.py
- /root/dev/athan/prayer_times_config.py
- /root/dev/athan/prayer_times_fetcher.py
- /root/dev/athan/time_sync.py

---

**Report Generated:** November 4, 2025
**Investigator:** Code Analysis System
**Status:** Investigation Complete - Ready for Remediation
