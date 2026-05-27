"""
Unified application settings — Pydantic + TOML.
Single source of truth for all runtime configuration.

Usage:
    from settings import settings          # always-current singleton

    # Read
    group = settings.speaker.group_name
    location = settings.prayer.location

    # Mutate, validate, and persist from the web UI
    settings.update(speaker={"group_name": "new-group"})
    settings.save()

    # Hot-reload after a file change (called by ConfigWatcher)
    settings.reload()
"""
from __future__ import annotations

import configparser
import logging
import os
import threading
import tomllib
from pathlib import Path
from typing import Literal, Optional

import tomli_w
from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)

_lock = threading.RLock()

# Config resolution order — first existing file wins.
# AZAN_CONFIG_FILE env var lets Docker/CI point at an arbitrary path.
def _search_paths() -> list[Path]:
    paths: list[Path] = []
    if env := os.environ.get("AZAN_CONFIG_FILE", ""):
        paths.append(Path(env))
    paths += [Path("/app/config/azan.toml"), Path("azan.toml")]
    return paths


def _find_config_file() -> Optional[Path]:
    # If the env var is explicitly set, only check that path — no fallback.
    if env := os.environ.get("AZAN_CONFIG_FILE", ""):
        p = Path(env)
        return p if p.exists() else None
    for p in [Path("/app/config/azan.toml"), Path("azan.toml")]:
        if p.exists():
            return p
    return None


def _find_writable_path() -> Path:
    """Return first path we can write to, creating parent dirs as needed."""
    for p in _search_paths():
        try:
            p.parent.mkdir(parents=True, exist_ok=True)
            # Probe writability without truncating
            with open(p, "ab"):
                pass
            return p
        except (OSError, PermissionError):
            continue
    # Last-resort fallback
    fallback = Path("azan.toml")
    fallback.touch()
    return fallback


# ---------------------------------------------------------------------------
# Model definitions
# ---------------------------------------------------------------------------

class SpeakerSettings(BaseModel):
    group_name: str = Field("athan", min_length=1, max_length=100)
    athan_speaker: str = ""
    pre_fajr_speaker: str = ""
    friday_kahf_speaker: str = ""
    quran_speaker: str = ""

    def resolve(self, type_: str) -> str:
        """Return the effective speaker name for this audio type (falls back to group_name)."""
        overrides = {
            "athan": self.athan_speaker,
            "pre_fajr": self.pre_fajr_speaker,
            "friday_kahf": self.friday_kahf_speaker,
            "quran": self.quran_speaker,
        }
        return overrides.get(type_, "") or self.group_name


class PrayerSettings(BaseModel):
    location: Literal["naas", "icci"] = "naas"
    pre_fajr_enabled: bool = False
    pre_fajr_minutes: int = Field(30, ge=1, le=60)
    friday_kahf_enabled: bool = False


class WebSettings(BaseModel):
    host: str = "0.0.0.0"
    port: int = Field(5000, ge=1, le=65535)
    secret_key: str = "automated-azan-secret-key"


class LogSettings(BaseModel):
    level: Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"] = "INFO"
    file_path: str = "logs/azan.log"


class Settings(BaseModel):
    speaker: SpeakerSettings = Field(default_factory=SpeakerSettings)
    prayer: PrayerSettings = Field(default_factory=PrayerSettings)
    web: WebSettings = Field(default_factory=WebSettings)
    log: LogSettings = Field(default_factory=LogSettings)

    def save(self) -> Path:
        """Write current state to TOML. Returns the path written."""
        with _lock:
            target = _find_writable_path()
            with open(target, "wb") as fh:
                tomli_w.dump(self.model_dump(), fh)
            logger.info("Settings saved to %s", target)
            return target

    def reload(self) -> "Settings":
        """Re-read from disk and update this instance in-place. Returns self."""
        with _lock:
            source = _find_config_file()
            if source is None:
                logger.warning("No config file found on reload — keeping current settings")
                return self
            fresh = _read_toml(source)
            for field_name in type(self).model_fields:
                setattr(self, field_name, getattr(fresh, field_name))
            logger.info("Settings reloaded from %s", source)
            return self

    def update(self, **section_dicts) -> "Settings":
        """
        Merge partial section dicts into current settings and validate.
        Does NOT auto-save — call settings.save() afterwards.

        Example:
            settings.update(speaker={"group_name": "living-room"})
            settings.update(prayer={"pre_fajr_enabled": True})
        """
        with _lock:
            current = self.model_dump()
            for section, updates in section_dicts.items():
                if section in current and isinstance(current[section], dict):
                    current[section].update(updates)
                else:
                    current[section] = updates
            fresh = Settings(**current)
            for field_name in type(self).model_fields:
                setattr(self, field_name, getattr(fresh, field_name))
        return self

    def as_web_dict(self) -> dict:
        """Flat dict for API responses and template context."""
        return {
            "speakers_group_name": self.speaker.group_name,
            "athan_speaker": self.speaker.athan_speaker,
            "pre_fajr_speaker": self.speaker.pre_fajr_speaker,
            "friday_kahf_speaker": self.speaker.friday_kahf_speaker,
            "quran_speaker": self.speaker.quran_speaker,
            "location": self.prayer.location,
            "pre_fajr_enabled": self.prayer.pre_fajr_enabled,
            "pre_fajr_minutes": self.prayer.pre_fajr_minutes,
            "friday_kahf_enabled": self.prayer.friday_kahf_enabled,
            "web_port": self.web.port,
            "log_level": self.log.level,
        }


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

def _read_toml(path: Path) -> Settings:
    with open(path, "rb") as fh:
        data = tomllib.load(fh)
    return Settings(**data)


def _migrate_legacy(target: Path) -> bool:
    """
    One-time migration: if adahn.config exists but azan.toml doesn't,
    convert and write azan.toml. Returns True if migration succeeded.
    """
    legacy_candidates = [
        Path("/app/config/adahn.config"),
        Path("config/adahn.config"),
        Path("adahn.config"),
    ]
    for legacy in legacy_candidates:
        if not legacy.exists():
            continue
        try:
            cp = configparser.ConfigParser()
            cp.read(legacy)
            s = cp["Settings"] if cp.has_section("Settings") else {}
            migrated = Settings(
                speaker=SpeakerSettings(
                    group_name=s.get("speakers-group-name", "athan"),
                ),
                prayer=PrayerSettings(
                    location=s.get("location", "naas"),  # type: ignore[arg-type]
                    pre_fajr_enabled=s.get("pre_fajr_enabled", "false").lower()
                    in ("true", "1", "yes"),
                ),
            )
            target.parent.mkdir(parents=True, exist_ok=True)
            with open(target, "wb") as fh:
                tomli_w.dump(migrated.model_dump(), fh)
            logger.info("Migrated legacy config %s → %s", legacy, target)
            return True
        except Exception as exc:
            logger.warning("Migration from %s failed: %s", legacy, exc)
    return False


# ---------------------------------------------------------------------------
# Public loader
# ---------------------------------------------------------------------------

def load_settings() -> Settings:
    """Load settings from disk, auto-migrating adahn.config if needed."""
    path = _find_config_file()
    if path is None:
        # Attempt one-time migration from legacy INI format
        target = _search_paths()[-1]  # azan.toml (local)
        if _migrate_legacy(target):
            path = target
        else:
            logger.warning("No config file found — using defaults")
            return Settings()
    try:
        s = _read_toml(path)
        logger.info("Settings loaded from %s", path)
        return s
    except Exception as exc:
        logger.error("Failed to load %s: %s — using defaults", path, exc)
        return Settings()


# ---------------------------------------------------------------------------
# Module-level singleton — import this everywhere
# ---------------------------------------------------------------------------
settings: Settings = load_settings()
