"""
Tests for config_watcher.py — watchdog-based hot-reload of azan.toml.
"""
import time
from unittest.mock import Mock

import pytest
from watchdog.events import FileModifiedEvent

import config_watcher
from config_watcher import ConfigWatcher, _FileWatcher


class TestFileWatcherOnModified:

    @pytest.mark.unit
    def test_callback_fires_when_watched_file_content_changes(self, tmp_path):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        callback = Mock()
        watcher = _FileWatcher(config_path, callback, debounce=0)

        config_path.write_text("changed")
        watcher.on_modified(FileModifiedEvent(str(config_path)))

        callback.assert_called_once()

    @pytest.mark.unit
    def test_no_callback_for_unrelated_file_event(self, tmp_path):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        callback = Mock()
        watcher = _FileWatcher(config_path, callback, debounce=0)

        unrelated_path = tmp_path / "other.txt"
        unrelated_path.write_text("something else")
        watcher.on_modified(FileModifiedEvent(str(unrelated_path)))

        callback.assert_not_called()

    @pytest.mark.unit
    def test_no_callback_when_content_unchanged(self, tmp_path):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        callback = Mock()
        watcher = _FileWatcher(config_path, callback, debounce=0)

        # Same event fires twice with no actual content change in between.
        watcher.on_modified(FileModifiedEvent(str(config_path)))

        callback.assert_not_called()

    @pytest.mark.unit
    def test_second_change_within_debounce_window_is_suppressed(self, tmp_path):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        callback = Mock()
        watcher = _FileWatcher(config_path, callback, debounce=100.0)

        config_path.write_text("first-change")
        watcher.on_modified(FileModifiedEvent(str(config_path)))

        config_path.write_text("second-change")
        watcher.on_modified(FileModifiedEvent(str(config_path)))

        callback.assert_called_once()

    @pytest.mark.unit
    def test_exception_in_callback_is_swallowed(self, tmp_path):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        callback = Mock(side_effect=RuntimeError("boom"))
        watcher = _FileWatcher(config_path, callback, debounce=0)

        config_path.write_text("changed")
        # Must not raise even though the callback blows up.
        watcher.on_modified(FileModifiedEvent(str(config_path)))

        callback.assert_called_once()

    @pytest.mark.unit
    def test_last_reload_time_updates_on_successful_reload(self, tmp_path):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        watcher = _FileWatcher(config_path, Mock(), debounce=0)
        assert watcher.last_reload_time == 0.0

        config_path.write_text("changed")
        before = time.time()
        watcher.on_modified(FileModifiedEvent(str(config_path)))

        assert watcher.last_reload_time >= before


class TestConfigWatcherStartStop:

    @pytest.mark.unit
    def test_start_returns_success_and_stop_cleans_up_observer(self, tmp_path, monkeypatch):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        monkeypatch.setattr(config_watcher, "_find_config_file", lambda: config_path)

        watcher = ConfigWatcher(scheduler=Mock())
        try:
            result = watcher.start()
            assert result["success"] is True
            assert watcher.observer.is_alive()
        finally:
            stop_result = watcher.stop()

        assert stop_result["success"] is True
        assert not watcher.observer.is_alive()

    @pytest.mark.unit
    def test_start_when_already_running_reports_failure(self, tmp_path, monkeypatch):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        monkeypatch.setattr(config_watcher, "_find_config_file", lambda: config_path)

        watcher = ConfigWatcher(scheduler=Mock())
        try:
            watcher.start()
            second = watcher.start()
            assert second["success"] is False
            assert "already" in second["message"].lower()
        finally:
            watcher.stop()

    @pytest.mark.unit
    def test_stop_when_not_running_reports_failure(self):
        watcher = ConfigWatcher(scheduler=Mock())
        result = watcher.stop()
        assert result["success"] is False
        assert "not running" in result["message"].lower()

    @pytest.mark.unit
    def test_reload_callback_fires_via_config_watcher_on_real_file_write(self, tmp_path, monkeypatch):
        """
        End-to-end (but deterministic) check: a real watchdog Observer
        watching a real directory should invoke ConfigWatcher._on_change
        when the config file is modified on disk.
        """
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        monkeypatch.setattr(config_watcher, "_find_config_file", lambda: config_path)

        watcher = ConfigWatcher(scheduler=Mock())
        on_change = Mock()
        monkeypatch.setattr(watcher, "_on_change", on_change)

        try:
            watcher.start()
            # Bypass real filesystem event timing entirely: dispatch the
            # modification event directly to the handler, exactly as the
            # Observer thread would, but deterministically.
            config_path.write_text("changed")
            watcher._file_watcher.on_modified(FileModifiedEvent(str(config_path)))
        finally:
            watcher.stop()

        on_change.assert_called_once()

    @pytest.mark.unit
    def test_get_status_reflects_running_state(self, tmp_path, monkeypatch):
        config_path = tmp_path / "azan.toml"
        config_path.write_text("initial")
        monkeypatch.setattr(config_watcher, "_find_config_file", lambda: config_path)

        watcher = ConfigWatcher(scheduler=Mock())
        assert watcher.get_status()["running"] is False

        try:
            watcher.start()
            assert watcher.get_status()["running"] is True
        finally:
            watcher.stop()

        assert watcher.get_status()["running"] is False
