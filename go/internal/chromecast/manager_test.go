package chromecast

import (
	"sort"
	"testing"
)

// TestEqualFold verifies case-insensitive ASCII matching used to resolve a
// configured device name against discovered/cached devices, including
// unicode and empty-string edge cases.
func TestEqualFold(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"exact match", "Kitchen Speaker", "Kitchen Speaker", true},
		{"different case", "kitchen speaker", "KITCHEN SPEAKER", true},
		{"mixed case", "Living Room", "living ROOM", true},
		{"different length", "Adahn", "Adahn Speaker", false},
		{"different content same length", "Kitchen", "Bedroom", false},
		{"both empty", "", "", true},
		{"one empty", "Adahn", "", false},
		{"unicode equal case", "Café", "Café", true},
		{"unicode different byte case unmatched", "CAFÉ", "café", false},
		{"digits and symbols", "Nest-Hub_2", "Nest-Hub_2", true},
		{"trailing whitespace differs", "Adahn", "Adahn ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := equalFold(tt.a, tt.b); got != tt.want {
				t.Errorf("equalFold(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			// equalFold should be symmetric.
			if got := equalFold(tt.b, tt.a); got != tt.want {
				t.Errorf("equalFold(%q, %q) = %v, want %v (symmetry)", tt.b, tt.a, got, tt.want)
			}
		})
	}
}

// TestFoldEqual verifies the byte-wise ASCII case folding helper directly,
// including its documented precondition that inputs are the same length.
func TestFoldEqual(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"exact match", "abc", "abc", true},
		{"upper vs lower", "ABC", "abc", true},
		{"mixed case", "AbC", "aBc", true},
		{"mismatch same length", "abc", "abd", false},
		{"both empty", "", "", true},
		{"non-letter bytes preserved", "a1B", "A1b", true},
		{"unicode multi-byte case unmatched", "É", "é", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := foldEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("foldEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestDevicesEmptyCache verifies Devices() returns an empty (non-nil) slice
// when no devices have been discovered or loaded yet.
func TestDevicesEmptyCache(t *testing.T) {
	m := NewManager("Kitchen Speaker")

	got := m.Devices()
	if got == nil {
		t.Fatalf("Devices() = nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("Devices() = %v, want empty", got)
	}
}

// TestDevicesReturnsAllCachedDevices verifies Devices() (backed by
// deviceList()) returns every device currently in the injected cache,
// regardless of map iteration order.
func TestDevicesReturnsAllCachedDevices(t *testing.T) {
	m := NewManager("Kitchen Speaker")
	m.devices["uuid-1"] = Device{UUID: "uuid-1", Name: "Kitchen Speaker", Host: "192.168.1.10", Port: 8009, ModelName: "Chromecast Audio"}
	m.devices["uuid-2"] = Device{UUID: "uuid-2", Name: "Living Room Speaker", Host: "192.168.1.20", Port: 8009, ModelName: "Chromecast"}
	m.devices["uuid-3"] = Device{UUID: "uuid-3", Name: "Bedroom Display", Host: "192.168.1.30", Port: 8009, ModelName: "Nest Hub"}

	got := m.Devices()
	if len(got) != 3 {
		t.Fatalf("Devices() returned %d devices, want 3: %+v", len(got), got)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].UUID < got[j].UUID })

	want := []Device{
		{UUID: "uuid-1", Name: "Kitchen Speaker", Host: "192.168.1.10", Port: 8009, ModelName: "Chromecast Audio"},
		{UUID: "uuid-2", Name: "Living Room Speaker", Host: "192.168.1.20", Port: 8009, ModelName: "Chromecast"},
		{UUID: "uuid-3", Name: "Bedroom Display", Host: "192.168.1.30", Port: 8009, ModelName: "Nest Hub"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Devices()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestDevicesFilteringByName verifies the equalFold-based name matching
// pattern used throughout manager.go (e.g. resolveDevice, PlayURLOnDevice)
// correctly finds a target device within a populated cache without
// requiring any real mDNS discovery or Chromecast connection.
func TestDevicesFilteringByName(t *testing.T) {
	m := NewManager("Kitchen Speaker")
	m.devices["uuid-1"] = Device{UUID: "uuid-1", Name: "Kitchen Speaker", Host: "192.168.1.10", Port: 8009}
	m.devices["uuid-2"] = Device{UUID: "uuid-2", Name: "Living Room Speaker", Host: "192.168.1.20", Port: 8009}

	tests := []struct {
		name       string
		target     string
		wantFound  bool
		wantDevice string
	}{
		{"exact case match", "Kitchen Speaker", true, "192.168.1.10"},
		{"case-insensitive match", "kitchen speaker", true, "192.168.1.10"},
		{"matches other device", "LIVING ROOM SPEAKER", true, "192.168.1.20"},
		{"no match", "Bedroom Display", false, ""},
		{"empty target no match", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devs := m.Devices()
			var found *Device
			for i := range devs {
				if equalFold(devs[i].Name, tt.target) {
					found = &devs[i]
					break
				}
			}
			if tt.wantFound && found == nil {
				t.Fatalf("expected to find device %q, got none", tt.target)
			}
			if !tt.wantFound && found != nil {
				t.Fatalf("expected no match for %q, got %+v", tt.target, *found)
			}
			if tt.wantFound && found.Host != tt.wantDevice {
				t.Errorf("found device host = %q, want %q", found.Host, tt.wantDevice)
			}
		})
	}
}
