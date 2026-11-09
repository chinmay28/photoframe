package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFromFakeRoot(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"proc/device-tree/model":               "Raspberry Pi Zero 2 W Rev 1.0\x00",
		"sys/class/thermal/thermal_zone0/temp": "41234\n",
		"proc/uptime":                          "518400.12 1000000.00\n",
		"proc/net/wireless": "Inter-| sta-|   Quality        |   Discarded packets\n" +
			" face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22\n" +
			"wlan0: 0000   52.  -58.  -256        0      0      0      0     12        0\n",
	}
	for p, c := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(c), 0o644)
	}
	info := Reader{Root: root, Version: "test"}.Read()
	if info.Model != "Raspberry Pi Zero 2 W Rev 1.0" {
		t.Errorf("model = %q", info.Model)
	}
	if info.CPUTempC == nil || *info.CPUTempC < 41.2 || *info.CPUTempC > 41.3 {
		t.Errorf("temp = %v", info.CPUTempC)
	}
	if info.UptimeSec != 518400 {
		t.Errorf("uptime = %d", info.UptimeSec)
	}
	if info.WiFiIface != "wlan0" || info.WiFiSignal == nil || *info.WiFiSignal != -58 {
		t.Errorf("wifi = %q %v", info.WiFiIface, info.WiFiSignal)
	}
}

func TestReadMissingFilesIsEmpty(t *testing.T) {
	info := Reader{Root: t.TempDir()}.Read()
	if info.CPUTempC != nil || info.WiFiSignal != nil || info.UptimeSec != 0 {
		t.Fatalf("expected empty info, got %+v", info)
	}
}
