// Package sysinfo reports device health (temperature, uptime, Wi-Fi) for
// the app's Device screen. Readers are best-effort: missing data is left
// empty rather than failing.
package sysinfo

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Info is a snapshot of device state.
type Info struct {
	Hostname   string   `json:"hostname"`
	Model      string   `json:"model,omitempty"`
	Addresses  []string `json:"addresses,omitempty"`
	CPUTempC   *float64 `json:"cpuTempC,omitempty"`
	UptimeSec  int64    `json:"uptimeSec,omitempty"`
	WiFiIface  string   `json:"wifiIface,omitempty"`
	WiFiSignal *int     `json:"wifiSignalDbm,omitempty"`
	Version    string   `json:"version"`
	Arch       string   `json:"arch"`
}

// Reader reads system files relative to Root ("/" on a real device).
type Reader struct {
	Root    string
	Version string
}

// Read gathers the current Info.
func (r Reader) Read() Info {
	root := r.Root
	if root == "" {
		root = "/"
	}
	host, _ := os.Hostname()
	info := Info{Hostname: host, Version: r.Version, Arch: runtime.GOARCH}
	info.Model = strings.TrimRight(readString(filepath.Join(root, "proc/device-tree/model")), "\x00")

	if s := readString(filepath.Join(root, "sys/class/thermal/thermal_zone0/temp")); s != "" {
		if milli, err := strconv.ParseFloat(s, 64); err == nil {
			c := milli / 1000
			info.CPUTempC = &c
		}
	}
	if s := readString(filepath.Join(root, "proc/uptime")); s != "" {
		if f := strings.Fields(s); len(f) > 0 {
			if v, err := strconv.ParseFloat(f[0], 64); err == nil {
				info.UptimeSec = int64(v)
			}
		}
	}
	info.WiFiIface, info.WiFiSignal = parseWireless(readString(filepath.Join(root, "proc/net/wireless")))
	info.Addresses = addresses()
	return info
}

// parseWireless extracts the first interface and its signal level (dBm)
// from /proc/net/wireless.
func parseWireless(s string) (string, *int) {
	lines := strings.Split(s, "\n")
	for _, line := range lines[min(2, len(lines)):] { // two header lines
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		iface := strings.TrimSuffix(f[0], ":")
		level, err := strconv.ParseFloat(strings.TrimSuffix(f[3], "."), 64)
		if err != nil {
			return iface, nil
		}
		dbm := int(level)
		return iface, &dbm
	}
	return "", nil
}

func addresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
