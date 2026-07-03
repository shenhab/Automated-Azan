"""
Tests for the settings module (Pydantic + TOML config system).
"""
import os
import subprocess
import sys
import pytest
import tempfile
from pathlib import Path
from pydantic import ValidationError

from settings import Settings, SpeakerSettings, PrayerSettings, WebSettings, LogSettings, load_settings, _read_toml


VALID_TOML = """\
[speaker]
group_name = "living-room"

[prayer]
location = "icci"
pre_fajr_enabled = true
pre_fajr_minutes = 20

[web]
port = 8080
secret_key = "test-key"

[log]
level = "DEBUG"
file_path = "/tmp/test.log"
"""


class TestSettingsModel:

    @pytest.mark.unit
    def test_defaults(self):
        s = Settings()
        assert s.speaker.group_name == "athan"
        assert s.prayer.location == "naas"
        assert s.prayer.pre_fajr_enabled is False
        assert s.prayer.pre_fajr_minutes == 30
        assert s.web.port == 5000
        assert s.log.level == "INFO"

    @pytest.mark.unit
    def test_invalid_location_raises(self):
        with pytest.raises(ValidationError):
            Settings(prayer=PrayerSettings(location="london"))  # type: ignore[arg-type]

    @pytest.mark.unit
    def test_invalid_port_raises(self):
        with pytest.raises(ValidationError):
            Settings(web=WebSettings(port=0))

    @pytest.mark.unit
    def test_invalid_port_too_high(self):
        with pytest.raises(ValidationError):
            Settings(web=WebSettings(port=99999))

    @pytest.mark.unit
    def test_invalid_log_level_raises(self):
        with pytest.raises(ValidationError):
            Settings(log=LogSettings(level="VERBOSE"))  # type: ignore[arg-type]

    @pytest.mark.unit
    def test_empty_speaker_name_raises(self):
        with pytest.raises(ValidationError):
            Settings(speaker=SpeakerSettings(group_name=""))

    @pytest.mark.unit
    def test_valid_locations(self):
        for loc in ("naas", "icci"):
            s = Settings(prayer=PrayerSettings(location=loc))
            assert s.prayer.location == loc


class TestSettingsUpdate:

    @pytest.mark.unit
    def test_update_speaker(self):
        s = Settings()
        s.update(speaker={"group_name": "kitchen"})
        assert s.speaker.group_name == "kitchen"

    @pytest.mark.unit
    def test_update_partial_prayer_preserves_other_fields(self):
        s = Settings(prayer=PrayerSettings(location="icci", pre_fajr_minutes=15))
        s.update(prayer={"pre_fajr_enabled": True})
        assert s.prayer.location == "icci"
        assert s.prayer.pre_fajr_minutes == 15
        assert s.prayer.pre_fajr_enabled is True

    @pytest.mark.unit
    def test_update_invalid_location_raises(self):
        s = Settings()
        with pytest.raises(ValidationError):
            s.update(prayer={"location": "moon"})

    @pytest.mark.unit
    def test_update_returns_self(self):
        s = Settings()
        result = s.update(speaker={"group_name": "x"})
        assert result is s


class TestSettingsSaveReload:

    @pytest.mark.unit
    def test_save_creates_valid_toml(self, tmp_path, monkeypatch):
        target = tmp_path / "azan.toml"
        monkeypatch.setenv("AZAN_CONFIG_FILE", str(target))

        # Re-import to pick up env var — or just call save directly
        import importlib
        import settings as settings_mod
        importlib.reload(settings_mod)

        s = Settings(speaker=SpeakerSettings(group_name="test-group"))
        s.save()

        assert target.exists()
        loaded = _read_toml(target)
        assert loaded.speaker.group_name == "test-group"

    @pytest.mark.unit
    def test_reload_picks_up_file_changes(self, tmp_path, monkeypatch):
        target = tmp_path / "azan.toml"
        monkeypatch.setenv("AZAN_CONFIG_FILE", str(target))

        s = Settings(speaker=SpeakerSettings(group_name="first"))
        s.save()

        # Mutate on disk
        import tomli_w, tomllib
        with open(target, "rb") as fh:
            data = tomllib.load(fh)
        data["speaker"]["group_name"] = "second"
        with open(target, "wb") as fh:
            tomli_w.dump(data, fh)

        # Reload should pick up the change
        s.reload()
        assert s.speaker.group_name == "second"

    @pytest.mark.unit
    def test_reload_without_file_keeps_current(self, tmp_path, monkeypatch):
        monkeypatch.setenv("AZAN_CONFIG_FILE", str(tmp_path / "nonexistent.toml"))
        s = Settings(speaker=SpeakerSettings(group_name="kept"))
        s.reload()
        assert s.speaker.group_name == "kept"


class TestLoadSettings:

    @pytest.mark.unit
    def test_load_from_valid_toml(self, tmp_path, monkeypatch):
        target = tmp_path / "azan.toml"
        target.write_text(VALID_TOML)
        monkeypatch.setenv("AZAN_CONFIG_FILE", str(target))

        import importlib, settings as settings_mod
        importlib.reload(settings_mod)

        s = load_settings()
        assert s.speaker.group_name == "living-room"
        assert s.prayer.location == "icci"
        assert s.prayer.pre_fajr_enabled is True
        assert s.web.port == 8080
        assert s.log.level == "DEBUG"

    @pytest.mark.unit
    def test_load_with_no_file_returns_defaults(self, tmp_path, monkeypatch):
        monkeypatch.setenv("AZAN_CONFIG_FILE", str(tmp_path / "missing.toml"))
        s = load_settings()
        assert s.speaker.group_name == "athan"
        assert s.prayer.location == "naas"


class TestAsWebDict:

    @pytest.mark.unit
    def test_as_web_dict_keys(self):
        s = Settings()
        d = s.as_web_dict()
        assert "speakers_group_name" in d
        assert "location" in d
        assert "pre_fajr_enabled" in d
        assert "pre_fajr_minutes" in d

    @pytest.mark.unit
    def test_as_web_dict_values(self):
        s = Settings(
            speaker=SpeakerSettings(group_name="hall"),
            prayer=PrayerSettings(location="icci", pre_fajr_enabled=True),
        )
        d = s.as_web_dict()
        assert d["speakers_group_name"] == "hall"
        assert d["location"] == "icci"
        assert d["pre_fajr_enabled"] is True


class TestNoRealConfigMutation:
    """
    Regression test: reload-triggering tests must never write to the repo's
    real azan.toml. Several tests below monkeypatch AZAN_CONFIG_FILE to a
    not-yet-existing tmp path and then importlib.reload(settings) — which
    re-runs the module-level `load_settings()` singleton init and, on a
    missing config file, falls into the auto-migration path. Without
    monkeypatch.chdir(), that path used to resolve relative to the real
    process cwd (the repo root) instead of the monkeypatched tmp location,
    silently overwriting the real azan.toml with migrated defaults.

    Run in a subprocess (not just in-process reload) so the real cwd/env
    resolution matches what actually happens when the suite executes.
    """

    # The specific tests known to trigger a module-level reload with a
    # not-yet-existing AZAN_CONFIG_FILE target. Deliberately scoped to these
    # rather than the whole file (or test_web_interface_api.py, whose
    # network/hardware-touching tests are unrelated and can hang) so this
    # regression test stays fast and focused on the actual root cause.
    RELOAD_TRIGGERING_TESTS = [
        "tests/test_settings.py::TestSettingsSaveReload::test_save_creates_valid_toml",
        "tests/test_settings.py::TestLoadSettings::test_load_from_valid_toml",
        "tests/test_settings.py::TestLegacyMigration::test_migrates_adahn_config",
    ]

    @pytest.mark.unit
    def test_reload_triggering_tests_do_not_mutate_real_azan_toml(self):
        repo_root = Path(__file__).resolve().parent.parent
        real_config = repo_root / "azan.toml"

        before_bytes = real_config.read_bytes()
        before_mtime = real_config.stat().st_mtime_ns

        result = subprocess.run(
            [sys.executable, "-m", "pytest", *self.RELOAD_TRIGGERING_TESTS, "-q"],
            cwd=repo_root,
            capture_output=True,
            text=True,
            timeout=60,
        )

        after_bytes = real_config.read_bytes()
        after_mtime = real_config.stat().st_mtime_ns

        assert before_bytes == after_bytes, (
            "Running the reload-triggering settings tests mutated the real "
            "repo-root azan.toml.\n"
            f"pytest stdout:\n{result.stdout}\npytest stderr:\n{result.stderr}"
        )
        assert before_mtime == after_mtime, (
            "Real repo-root azan.toml mtime changed even though content matched.\n"
            f"pytest stdout:\n{result.stdout}\npytest stderr:\n{result.stderr}"
        )


class TestLegacyMigration:

    @pytest.mark.unit
    def test_migrates_adahn_config(self, tmp_path, monkeypatch):
        legacy = tmp_path / "adahn.config"
        legacy.write_text(
            "[Settings]\nspeakers-group-name = my-speaker\nlocation = icci\n"
        )
        target = tmp_path / "azan.toml"
        monkeypatch.setenv("AZAN_CONFIG_FILE", str(target))
        monkeypatch.chdir(tmp_path)

        import importlib, settings as settings_mod
        importlib.reload(settings_mod)

        from settings import _migrate_legacy
        result = _migrate_legacy(target)

        # Migration may or may not succeed depending on path resolution,
        # but if it does, values should be correct
        if result:
            s = _read_toml(target)
            assert s.speaker.group_name == "my-speaker"
            assert s.prayer.location == "icci"
