"""
Regression tests proving test isolation for ChromecastManager's class-level
discovery cache (_shared_chromecasts / _shared_last_discovery).

These tests are order-sensitive by design: test_a runs before test_b in normal
(file-declaration-order) execution, but the assertions in test_b must hold no
matter what ran before it. Without the autouse `reset_chromecast_shared_state`
fixture in conftest.py, test_b would inherit the stale cache seeded by test_a
and fail.
"""
import pytest
from unittest.mock import patch

from chromecast_manager import ChromecastManager


class TestSharedChromecastStateIsolation:
    @pytest.mark.unit
    def test_a_dirty_shared_chromecast_cache(self):
        """Simulate a prior test leaving stale discovery state on the class."""
        ChromecastManager._shared_chromecasts = {"stale-uuid": {"name": "Stale Device"}}
        ChromecastManager._shared_last_discovery = 9999999999.0

        # Sanity check that this test really did dirty the class-level cache.
        assert ChromecastManager._shared_chromecasts
        assert ChromecastManager._shared_last_discovery != 0

    @pytest.mark.unit
    def test_b_shared_cache_starts_clean(self):
        """A fresh test must never see cache state left behind by another test."""
        assert ChromecastManager._shared_chromecasts == {}
        assert ChromecastManager._shared_last_discovery == 0

    @pytest.mark.unit
    @patch('chromecast_manager.ChromecastManager.discover_devices')
    def test_c_new_manager_instance_does_not_inherit_stale_cache(self, mock_discover):
        """A freshly constructed ChromecastManager must not inherit the stale
        shared-cache entries that test_a (a stand-in for "a prior test") left
        on the class."""
        mock_discover.return_value = {
            "success": True,
            "devices_found": 0,
            "devices": {},
            "timestamp": "2023-01-01T00:00:00",
        }

        # retry_delay=0 keeps this fast regardless of the mock not populating
        # self.chromecasts (discover_devices is mocked out entirely).
        manager = ChromecastManager(max_retries=1, retry_delay=0)

        assert manager.chromecasts == {}
        assert manager.last_discovery_time == 0
